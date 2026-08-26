# Build #23 — Wiki Backlinks Cache Observability + Invalidation Audit Trail

## 来源(上一轮决策)
- **触发评论** `01a03e93-0014-7922-8672-c865a7f9cd51` (LUM-20 thread `01a03c17-09b9-7cfc-aced-866b74d7b836`)
- **用户意图** "直接进 Build #23"——承接 Build #22 reply 的提案「管理员 / debug 端点的 `/backlinks/cache-status` 缓存命中率埋点 + audit log 关联」
- **本 change** weknora-cache-observability

## 起点研判(Build #23 调查结论)

Build #21 给 `wiki_backlinks_cache` 加了持久层,Build #22 加了 stale cleanup cron + 4 个 sweeper 侧 Prom 指标。但**主链路(读 / 写 / 失效)完全没有埋点**——运维既看不到命中率,也看不到失效历史。

调研出的 10 个缺口(全文见 investigation report),按"可在 1 个 Build 内完成"的边界切到 Build #23 范围,剩余 4 个留到 Build #24+:

| Gap | Build #23? | 备注 |
|---|---|---|
| #1 `ListBacklinkGraph` 无 hit/miss/error counter | ✅ | 3 个 outcome 都已 inline,加 1 行 metric 包装 |
| #2 `Upsert` 无 cache_writes,`Delete` 无 invalidation counter | ✅ | 同 #1,3 个 wrap 点 |
| #3 `cache-status` payload 无 aggregate | ✅ | 扩展 struct,加 hit_ratio / kb_id / row_count |
| #4 `ListByKB` 已存在但无 HTTP 路由 | ✅ | 挂到 `/backlinks/cache-statuses` 列表端点 |
| #5 失效事件仅 log,无 durable audit,无 source_event_id 关联 | ✅ | 加 `wiki_backlinks_cache_invalidation_log` 表(参考 Build #21 D7) |
| #6 三套 audit 表不统一 | 留 Build #24 | 「audit log 统一层」是单独 Build |
| #7 `wiki.content_changed` 无 slug/op 细节 | 留 Build #24 | 跟 #6 一起做 |
| #8 ACL 变更未触发 backlinks cache 失效 | 留 Build #25 | ACL → cache 失效链路是单独 Build |
| #9 page move / batch 写无 `audit_logs` 行 | 留 Build #25 | 跟 #8 一起,统一 audit 覆盖 |
| #10 `DeleteStale` evicted keys 不可恢复 | 留 Build #22.x (Build #22 已 ship,但 dry-run 模式只输出 count,可以补一行 log)| 实际 Build #22 logger 已 log affected count,Build #23 顺便把 Build #22 的 eviction 也走 #5 的新表 |

> 备注:Build #23 顺手补 #10,成本几乎为 0(只是 `DeleteStale` 调用处多一行 `InsertInvalidationLog`),不另起 Build。

## 目标(一句话)

给 `wiki_backlinks_cache` 主链路补齐**可观测三件套**(Prom 命中率/写率/失效率)+ **运维 debug 端点**(`/backlinks/cache-statuses` 列表视图)+ **失效事件持久化**(`wiki_backlinks_cache_invalidation_log` 表,source_event_id 关联写动作)。不改读路径行为,不改 Build #21/22 已 ship 的代码语义。

## 决策表 D1–D7(等你拍板)

| ID | 决定 | 推荐 |
|---|---|---|
| **D1** | hit/miss/error metric 维度 | **A 3 个独立 counter**:`wiki_cache_hits_total` / `wiki_cache_misses_total` / `wiki_cache_errors_total`,按 `kb_id` label(基数受控,KB 数量 < 10k) |
| **D2** | cache_writes metric | **A 1 个 counter** `wiki_cache_writes_total`,无 label(Build #21 `Upsert` 全局写入,不分维度) |
| **D3** | invalidation metric | **A 1 个 counter** `wiki_cache_invalidations_total`,按 `op` label(7 种 op:`create/update/delete/move/batch_move/batch_delete/batch_status`) |
| **D4** | cache-status payload 扩展 | **A 加 4 字段**:`kb_id`(必填,从 path 拿,目前缺)、`row_count`(全 KB 行数,cheap query)、`hit_ratio`(从 `hits_total/(hits+misses)` 算,服务端算,客户端不积分)、`payload_size_bytes`(4 个 JSON 列字节和,UI 可显示「5.2 KB」) |
| **D5** | 新端点 `/backlinks/cache-statuses` | **A 复用 `ListByKB`**,挂到 admin/debug 路径,加 `g.Admin()` 中间件(不是 Viewer),分页 limit=50 |
| **D6** | 失效事件持久化 | **A 新表 `wiki_backlinks_cache_invalidation_log(id, kb_id, slug, op, actor_user_id, source_event_id, affected_count, created_at)`**——Build #21 D7 的承诺(spec 里有但没实现),每次失效调用(包括 Build #22 sweeper 的 `DeleteStale`)插一行 |
| **D7** | source_event_id 来源 | **A handler 层在写动作提交后,通过 `recordWikiInvalidationEvent(ctx, kbID, slug, op, eventID)` 调用 `cacheRepo.LogInvalidation`,eventID 来自请求 header `X-Request-ID`(gin 中间件已生成)或新生成 UUID** |

## 不做(非目标)

- **不做** ACL → cache 失效联动(留 Build #25)
- **不做** `wiki.content_changed` audit 字段扩展(留 Build #24)
- **不做** 三套 audit 表的合并(留 Build #24)
- **不做** 任何缓存策略调整(hit/miss 决策逻辑不变,只观察)
- **不做** 新 admin UI(运维 grep Prom + curl 端点即可,前端不做)

## 数据模型

新增 1 张表(其它全部复用 Build #21/22 的):

```sql
CREATE TABLE wiki_backlinks_cache_invalidation_log (
    id              BIGSERIAL PRIMARY KEY,
    kb_id           VARCHAR(64) NOT NULL,
    slug            VARCHAR(512) NOT NULL,    -- 受影响的 slug,可能多个,见 details JSONB
    op              VARCHAR(32)  NOT NULL,    -- 7 种 BacklinkCacheInvalidateOp 之一
    actor_user_id   BIGINT,                   -- 可空:batch / sweeper 无 user
    source_event_id VARCHAR(64),              -- 关联 X-Request-ID
    affected_count  INT NOT NULL DEFAULT 0,   -- 本次失效的行数
    details         JSONB,                    -- {"slugs":["a","b","c"]} 或 Build #22 sweeper 的 {"ttl_days":30,"rows":N}
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wbc_invalidation_log_kb_created ON wiki_backlinks_cache_invalidation_log(kb_id, created_at DESC);
```

## 新增文件预清单

- backend:
  - `migrations/versioned/000099_wiki_backlinks_cache_invalidation_log.{up,down}.sql`
  - `internal/application/repository/wiki_backlinks_cache.go` — 加 `LogInvalidation(ctx, entry)`,`InvalidationLog` struct
  - `internal/application/service/wiki_backlinks_cache.go` — `ListBacklinkGraph` + `Upsert` 包装 hit/miss/write metric;`InvalidateBacklinksCache` + sweeper `DeleteStale` 包装 invalidation metric + 插 log
  - `internal/application/service/wiki_backlinks_cache_metrics.go` — 扩展 3 个新 counter(把现有 4 个 cleanup metric 留在原文件)
  - `internal/handler/wiki_page.go` — `GetPageBacklinksCacheStatus` 扩 payload(4 新字段);新 handler `ListBacklinksCacheStatuses`
  - `internal/router/routes_knowledge.go` — 挂新端点 `/backlinks/cache-statuses`(admin)
  - harness 测试:`wiki_backlinks_cache_observability_test.go`(5 用例:hit/miss/error 计数 / op-label 失效计数 / cache-status 4 新字段 / ListByKB admin 路由 / LogInvalidation 持久化)
- scripts:
  - `scripts/smoke-wiki-cache-observability.sh` — DRY_RUN-safe(创建 → graph → graph(命中) → 看 metrics → 改 page → 看 invalidation_log → 列表端点)
- docs:
  - `docs/comet/changes/weknora-cache-observability/{brief.md, specs/cache-observability/spec.md}`

## Build #21/22 零回归承诺

- `ListBacklinkGraph` 行为不变 —— hit/miss 计数器是「旁路 metric」,不影响返回值
- `Upsert` 失败仍然 warn-log + 返回原 cache row(语义不变)
- `DeleteStale` 行为不变 —— 仅多一行 `LogInvalidation` insert(失败也 warn-log 不阻断)
- 4 个 Build #22 cleanup metric 完全不动
- `GetPageBacklinksCacheStatus` 行为不变 —— 4 新字段都是「附加」,旧字段(`slug/computed_at/updated_at/source_event_id`)shape 不变

## 确认这个 Shape 后我直接进 Build

回我 **"按推荐走"** 或对 D1–D7 任一调整即可。如果想扩到 ACL 联动(Gap #8)或 audit 表合并(Gap #6/#7),可以并入,只是单 Build 会比 ~10 个验收项膨胀到 ~16 个,我建议先按本 Shape 走、留后续 Build。