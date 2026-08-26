# Build #21 — Wiki Backlinks Graph Cache (write-time invalidation)

## 来源(上一轮决策)
- **触发评论** `01a03dda-8187-7f2b-a9bc-5e613f464ae9`(LUM-20 thread `01a03c17-09b9-7cfc-aced-866b74d7b836`)
- **用户意图** "选择最佳方式实现"(从 Build #20 完成 reply 列出的 4 个后续方向中挑一个实现)
- **本 change 推荐** Build #21(反向链接更新流)

## 起点研判
Build #20 把"扁平反向链接面板"扩成了 4-section graph view(直接/间接/相关/失效),数据来源是 on-demand:`ListBacklinkGraph` 在每次 `GET /pages/:slug/backlinks/graph` 时跑 3 次 IN 查询 + Jaccard 集合运算 + broken 检测。

每次切页面 / 每次编辑完切回旧页面都会重算。即使浏览器本地缓存还在,服务端都是冷启动。

`WikiPageService.UpdatePage` / `DeletePage` / `CreatePage` / `MovePage` 已经存在,都会更新 `wiki_pages.out_links` 与 `wiki_pages.in_links`。`WikiBatch*Service`(Build #12-#16)也已经在 batch 写后 hook。

**所以 Build #21 是承接 Build #20 的纯升级**:加一个持久的 cache 层 + write-time 失效 hook,不改读接口,不动 Build #11/20 已有的 on-demand fallback。

## 目标(一句话)
为 `WikiBacklinkGraph` payload 加一个 DB-backed cache(`wiki_backlinks_cache`),在每次写入操作(`CreatePage` / `UpdatePage` / `DeletePage` / `MovePage` / `BatchMove` / `BatchDelete` / `BatchStatus`)失效相关页面的 cache 行;读取时 cache 命中直接返回,miss 走 Build #20 的 `ListBacklinkGraph` + 写回。Build #11 的 `/backlinks` 端点行为不变。

## 决策表 D1-D7

| ID | 决定 | 推荐 |
|---|---|---|
| D1 | cache 存储模型 | **A 单独表 `wiki_backlinks_cache`(kb_id, slug, direct_json, indirect_json, related_json, broken_json, stats_json, computed_at, source_event_id)**。out_links / in_links 已是 TEXT,JSON 列同方言(GORM AutoMigrate 支持 `types.JSONMap` 或纯 `string` + 自定义 marshal) |
| D2 | cache key 范围 | **A 单 (kb_id, slug) 一行**,跟 Build #20 payload 1:1。删除 / move 时清 1-3 行(slug 改名前后) |
| D3 | cache 命中策略 | **A 总是返回 cache**(无条件信任 — 计算 deterministic,只依赖 out_links / in_links,这两列在 write-time 失效后必 stale,read-time 不可能 stale)。`computed_at` 仅供 UI 显示「最近计算时间」 |
| D4 | write-time 失效触发 | **A 7 个写入口**:CreatePage / UpdatePage / DeletePage / MovePage(`Slug` rename + `FolderID` 改)/ BatchMove / BatchDelete / BatchStatus。每个 handler 在事务提交后 hook 一个 `InvalidateBacklinksCache(kbID, slugs []string)` |
| D5 | 失效范围 | **A**:`UpdatePage(slug A)` → 失效 A 本身(可能引用变)+ A.out_links 中的每个 target B(B 的 in_links 变化,direct backlinks 变);`DeletePage(A)` → 失效 A 本身 + A.in_links 中的每个 source;`MovePage(A oldSlug → newSlug)` → 失效 oldSlug + newSlug + newSlug.out_links + newSlug.in_links |
| D6 | batch 失效实现 | **A**:`WikiBatchService` 在每个 batch job `completed` 事件里调用 `InvalidateBacklinksCache` with `affected_slugs` 集合(`BatchMove` 的 source + target folders 子树,`BatchDelete` 的 slug 列表,`BatchStatus` 不动 out_links 所以只失效 slug 本身) |
| D7 | 失败处理 | **A 缓存写入失败 → 静默回退到 on-demand 计算**;**失效调用失败 → 写一条 `wiki_backlinks_cache_invalidation_log` warning**,下一读自动重算覆盖 |

## 不做(非目标)

- **不做** ACL 后置过滤(继续用 `KBAccessRead` 中间件,与 Build #20 一致)
- **不做** 反向链接的 push notification / 实时推送(Build #8 已经支持 WS 通知 wiki 事件,后续可叠加,不是本 Build 范围)
- **不做** 全局"刷新所有 backlink cache"的 cron / 后台 worker(只做 write-time 失效 + cold read 重算;过期由下次写覆盖)
- **不做** 单 page 的「我手动重算」按钮(可在 UI 里加 footer "Recompute",但不在本 Build spec)

## 数据模型

新增 1 张表:

```sql
CREATE TABLE wiki_backlinks_cache (
    kb_id          VARCHAR(64) NOT NULL,
    slug           VARCHAR(512) NOT NULL,
    direct_json    JSON NOT NULL,
    indirect_json  JSON NOT NULL,
    related_json   JSON NOT NULL,
    broken_json    JSON NOT NULL,
    stats_json     JSON NOT NULL,
    source_event_id VARCHAR(64),       -- 触发本行写入的 wiki_event.id,可空
    computed_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (kb_id, slug),
    INDEX idx_kb_id_updated (kb_id, updated_at)
);
```

为什么用 JSON 列而不是单独的 `wiki_backlinks_*_rows` 子表:
- payload 已经定型(4 段 + stats),写入是 upsert,不需要 row-level query
- 4 段通常很小(50 行 indirect / 10 行 related / 20 行 broken 量级),JSON 性能足够
- 与 Build #20 的 Go type / TS type 一一对应,无转换层

## 失效算法

```
Invalidate(kbID, affectedSlugs []string):
    DELETE FROM wiki_backlinks_cache WHERE kb_id = ? AND slug IN (?, ?, ...)

ResolveAffectedSlugs(writeOp):
    switch op:
        case UpdatePage(A):
            // A 自身的 out_links 变了 → 影响 A.in_links 上的 B 们的 direct
            // 同时 A.out_links 中的 target C 的 related 也变
            return uniq([A] + A.out_links)

        case DeletePage(A):
            // A.in_links 中的 source B 受影响(直接少了一条 backlink)
            return uniq([A] + A.in_links)

        case MovePage(A, oldSlug, newSlug, outLinks):
            return uniq([oldSlug, newSlug] + outLinks)

        case CreatePage(A):
            return uniq([A] + A.out_links)

        case BatchMove(slugs[]):
            return uniq(slugs + recursiveOutLinks(slugs))

        case BatchDelete(slugs[]):
            return uniq(slugs + recursiveInLinks(slugs))

        case BatchStatus(slugs[]):
            // status 不影响 backlinks 内容,只让相关 source 隐藏(若已 live)
            return uniq(slugs)
```

## 变更文件预清单

**Backend**
- `migrations/versioned/000097_wiki_backlinks_cache.up.sql` + `.down.sql`
- `internal/types/wiki_page.go` — `WikiBacklinksCacheRow` 类型 + `BacklinkCacheInvalidateRequest`
- `internal/types/interfaces/wiki_page.go` — `WikiBacklinksCacheRepository` 接口 + `WikiBacklinksCacheInvalidator` 接口
- `internal/application/repository/wiki_backlinks_cache.go` — `WikiBacklinksCacheRepository` GORM 实现
- `internal/application/service/wiki_backlinks_cache.go` — `ResolveAffectedSlugs` + `Invalidate` + cache 读 / 写
- `internal/application/service/wiki_page.go` — `ListBacklinkGraph` 改为 cache-first:命中返回,miss 计算 + 写 cache + 返回;`CreatePage` / `UpdatePage` / `DeletePage` / `MovePage` 提交后 hook `Invalidate`
- `internal/application/service/wiki_batch.go`(若已存在) — batch job completed 后 hook `Invalidate`
- `internal/handler/wiki_page.go` — 不变(`/backlinks/graph` 路径相同,行为更稳定)
- `internal/handler/wiki_backlinks_cache.go` — 新增 `GET /pages/:slug/backlinks/cache-status`(只返 `{computed_at, source_event_id}`),admin/debug 用
- `internal/router/routes_knowledge.go` — 注册 `/backlinks/cache-status` 路由
- `internal/application/service/wiki_page_backlinks_v2_test.go` — 加 4 用例:cache hit、cache miss 重算 + 写回、UpdatePage 失效 A + out_links、DeletePage 失效 A + in_links
- `cmd/server/main.go` — DI:注入 `WikiBacklinksCacheRepository` 进 `wikiPageService`

**Frontend**
- `frontend/src/api/wiki/backlinksCacheTypes.ts` — `WikiBacklinksCacheStatus` 接口
- `frontend/src/api/wiki/index.ts` — `getWikiBacklinksCacheStatus(kbId, slug)`
- `frontend/src/stores/wikiBacklinks.ts` — `cacheStatusByKey` + `loadCacheStatusFor` + 在 `loadBacklinkGraph` 之后调一次,显示「上次计算 X 分钟前」
- `frontend/src/components/wiki/WikiBacklinksPanel.vue` — 在 footer 旁边加一行「最近计算于 {{formatTime(cacheStatus?.computed_at)}}」灰色小字

**Scripts / 制品**
- `scripts/smoke-wiki-backlinks-cache.sh` — 7 步:create A → read(冷 miss) → read(命中) → update A → read(失效重算) → delete A → read(404)
- `docs/comet/changes/weknora-wiki-backlinks-v2-cache/{brief.md,specs/wiki-backlink-cache/spec.md}`

## 验收(commit 前必须过)

1. `go test ./internal/application/service/...` — 4 新 harness + 原 6 + Build #19.x / #20 全绿
2. `npm run type-check` — 0 新错误
3. `npm run build` — ✓
4. `npm run check-i18n` — 11/11
5. `WikiBacklinksPanel.test.ts` + 新增 `WikiBacklinksCache.test.ts` — 全绿
6. `scripts/smoke-wiki-backlinks-cache.sh` — dry-run 通过
7. `git push origin lumos0826` 成功

## Build #11 / #20 零回归

- `GET /pages/:slug/backlinks` — 路径、行为、payload 完全不变
- `GET /pages/:slug/backlinks/graph` — 路径、payload schema 完全不变;只是 hot path 走 cache(命中时返回时间从 ~80ms 降到 ~5ms)
- `WikiBacklinksPanel.vue` — UI 完全兼容;新增一行「最近计算于 ...」是纯增量
- `wikiBacklinks.ts` store — 增加 `cacheStatusByKey` 字段,Build #11/20 的 byKey / graphByKey 缓存层完全不动
- 写路径:`CreatePage` / `UpdatePage` / `DeletePage` / `MovePage` / batch — 在事务提交后调用 `Invalidate`,失败仅 warning,不影响主流程

## 风险与缓解

| 风险 | 缓解 |
|---|---|
| 事务外 hook:写完但 hook 失败 → cache stale | 失效日志表 + UI 显示「最近计算 X 秒前」,user 可手动刷新 |
| batch move/delete 影响几千 slug → 一条 SQL `WHERE slug IN (...)` | 已有 `WikiPageRepo` 提供批量查询接口,直接复用;DELETE 失败则降级为分批 |
| hot page 高频 read → 同一 slug 同时多 reader 触发 cache miss | cache write 用 `INSERT ... ON DUPLICATE KEY UPDATE`(MySQL)或 `INSERT ... ON CONFLICT DO UPDATE`(PG/SQLite);不需要分布式锁 |
| 写入失败但 read 已返 → UI 显示旧 cache | 当前 cache 只在 `ListBacklinkGraph` 写后 commit,在事务里;失败回滚则 cache 不写 |