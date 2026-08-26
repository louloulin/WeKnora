# Build #21 — Wiki Backlinks Graph Cache (write-time invalidation) — 完整目标 Spec

## A1-A20 验收矩阵

### 数据模型(A1-A4)

| ID | 验收 | 检查方式 |
|---|---|---|
| A1 | `migrations/versioned/000097_wiki_backlinks_cache.up.sql` 存在,创建 `wiki_backlinks_cache` 表 | `cat` 文件 |
| A2 | 表主键为 `(kb_id, slug)`,包含 `direct_json / indirect_json / related_json / broken_json / stats_json / source_event_id / computed_at / updated_at` 8 字段 + 1 个 `(kb_id, updated_at)` 二级索引 | `grep` |
| A3 | `.down.sql` 删除表(可逆) | `cat` 文件 |
| A4 | 3 dialect(POSTGRES / MYSQL / SQLITE)GORM AutoMigrate 在 `cmd/server/main.go` 注册;不依赖方言特性,JSON 列用 `types.JSONMap` 或自实现 `JSONSlice` | `grep` + `go build` |

### Read path(A5-A8)

| ID | 验收 | 检查方式 |
|---|---|---|
| A5 | `ListBacklinkGraph` 改为 cache-first:第一行代码是 `cacheRepo.Get(kbID, slug)`,命中则 `return cache, nil` | harness |
| A6 | cache miss 时回退 Build #20 的 `computeGraph(...)` + `cacheRepo.Upsert(...)` + `return cache, nil`,cache 写失败 warning 但不阻断返回 | harness |
| A7 | GET `/pages/:slug/backlinks/graph` 行为不变:payload schema 一致、HTTP 状态码一致 | smoke + harness |
| A8 | GET `/pages/:slug/backlinks`(Build #11 端点)**完全不动** | smoke 零回归步骤 |

### Write path(A9-A14)

| ID | 验收 | 检查方式 |
|---|---|---|
| A9 | `CreatePage(A)` 在事务提交后调用 `InvalidateBacklinksCache(kbID, [A] + A.out_links)` | harness |
| A10 | `UpdatePage(A)` 同上 | harness |
| A11 | `DeletePage(A)` 调用 `InvalidateBacklinksCache(kbID, [A] + A.in_links)` | harness |
| A12 | `MovePage(A oldSlug → newSlug)` 调用 `InvalidateBacklinksCache(kbID, [oldSlug, newSlug] + newSlug.out_links)` | harness |
| A13 | `WikiBatchMove` / `WikiBatchDelete` / `WikiBatchStatus` 在 batch job `completed` 事件 hook `InvalidateBacklinksCache` with affected slugs | harness + batch 已存在的事件接口 |
| A14 | `Invalidate` 失败仅记 warning log,不抛 / 不回滚主事务 | harness |

### Invalidation 算法(A15-A16)

| ID | 验收 | 检查方式 |
|---|---|---|
| A15 | `ResolveAffectedSlugs(op)` 对 7 种 op 各返回正确 slug 集合(包括 out_links / in_links 的传递闭包) | 4 个 harness + 表格驱动 |
| A16 | `Invalidate(kbID, slugs)` 一次 DELETE `WHERE kb_id = ? AND slug IN (?, ?, ...)`;批量超过阈值(默认 500)时分批 | harness + 注释 |

### Cache-status UI(A17-A18)

| ID | 验收 | 检查方式 |
|---|---|---|
| A17 | 新增 `GET /pages/:slug/backlinks/cache-status` 端点,返回 `{computed_at, source_event_id}` 或 404(未计算过) | smoke |
| A18 | `WikiBacklinksPanel.vue` footer 旁加一行灰色小字「最近计算于 {{relativeTime(cacheStatus?.computed_at)}}」;`computed_at` 缺失时隐藏 | i18n 4 locale 新增 `backlinksGraph.lastComputed` 键 + test |

### 回归(A19-A20)

| ID | 验收 | 检查方式 |
|---|---|---|
| A19 | Build #11 `/backlinks` 端点行为不变;`WikiBacklinksPanel.vue` 4-section 渲染逻辑不变;store 的 byKey / graphByKey 缓存层字段不变 | smoke + test |
| A20 | `WikiPageService` 其他方法(`ListPageBacklinks` / `GetPageBySlug` / `GetGraph` / batch 一切)行为不变 | harness + smoke |

## 关键文件路径速查

| 路径 | 用途 |
|---|---|
| `migrations/versioned/000097_wiki_backlinks_cache.{up,down}.sql` | 建表 + 索引 |
| `internal/types/wiki_page.go` | `WikiBacklinksCacheRow` / `BacklinkCacheInvalidateRequest` |
| `internal/types/interfaces/wiki_page.go` | `WikiBacklinksCacheRepository` / `WikiBacklinksCacheInvalidator` |
| `internal/application/repository/wiki_backlinks_cache.go` | GORM repo 实现 |
| `internal/application/service/wiki_backlinks_cache.go` | `ResolveAffectedSlugs` / `Invalidate` 业务逻辑 |
| `internal/application/service/wiki_page.go` | `ListBacklinkGraph` cache-first 化 + 写路径 hook |
| `internal/application/service/wiki_batch.go` | batch completed 事件 hook |
| `internal/handler/wiki_backlinks_cache.go` | `GetPageBacklinksCacheStatus` handler |
| `internal/router/routes_knowledge.go` | `/backlinks/cache-status` 路由注册 |
| `internal/application/service/wiki_page_backlinks_v2_test.go` | +4 个 cache 用例 |
| `frontend/src/api/wiki/backlinksCacheTypes.ts` | `WikiBacklinksCacheStatus` |
| `frontend/src/api/wiki/index.ts` | `getWikiBacklinksCacheStatus` |
| `frontend/src/stores/wikiBacklinks.ts` | `cacheStatusByKey` + `loadCacheStatusFor` |
| `frontend/src/components/wiki/WikiBacklinksPanel.vue` | 「最近计算于」一行 |
| `frontend/src/components/wiki/wikiBacklinksPanelLogic.ts` | `relativeTime(iso)` 助手 |
| `frontend/src/i18n/locales/{zh-CN,en-US,ko-KR,ru-RU}.ts` | 新增 `backlinksGraph.lastComputed` 4 翻译 |
| `scripts/smoke-wiki-backlinks-cache.sh` | 7 步 DRY_RUN-safe |
| `docs/comet/changes/weknora-wiki-backlinks-v2-cache/{brief.md,specs/wiki-backlink-cache/spec.md}` | 本 Build 制品 |

## 已知不在范围内(留给后续 Build)

- 反向链接 push 通知 / WebSocket 实时推送 — 由 Build #8 collab 已经覆盖,本 Build 只补 write-time 失效
- 全局 cache 刷新后台 worker — write-time 触发 + cold read 重算已足够,后续若发现 N 万级 cold read 才考虑
- TTL / stale cache 检测 — cache 仅在 write-time 失效;若需要更激进的兜底,后续可加 `computed_at < NOW - 1h` 触发后台重算
- 「手动重算」按钮 — 暂不加,需要时 1 行代码(`POST /backlinks/recompute?slug=X`)即可