# Build #23 — Wiki Backlinks Cache Observability + Invalidation Audit Trail — 完整目标 Spec

## A1-A12 验收矩阵

### 数据模型(A1-A2)

| ID | 验收 | 检查方式 |
|---|---|---|
| A1 | `migrations/versioned/000099_wiki_backlinks_cache_invalidation_log.up.sql` 存在,创建 `wiki_backlinks_cache_invalidation_log(id, kb_id, slug, op, actor_user_id, source_event_id, affected_count, details, created_at)` 表 + `(kb_id, created_at DESC)` 索引 | `cat` 文件 |
| A2 | `.down.sql` 删除表(可逆) | `cat` 文件 |

### 读路径埋点(A3-A4)

| ID | 验收 | 检查方式 |
|---|---|---|
| A3 | `ListBacklinkGraph` 在 `cacheRepo.Get` 后增加 3 个 outcome 判定,各自 inc 一次对应 counter:`hits_total` (返回非空 row) / `misses_total` (row==nil) / `errors_total` (err != nil 或 decode 失败)。所有 counter 带 `kb_id` label | harness |
| A4 | `ListBacklinkGraph` 行为完全不变 —— counter 是旁路 metric,不影响返回值/错误传递 | harness + smoke 零回归 |

### 写路径埋点(A5-A6)

| ID | 验收 | 检查方式 |
|---|---|---|
| A5 | `cacheRepo.Upsert` 在 service 层调用时 inc `wiki_cache_writes_total`(全局,无 label) | harness |
| A6 | 现有 `cacheRepo.Get` 第 2 处调用(在 `GetPageBacklinksCacheStatus` 内)不增加 hit/miss counter —— 这是 metadata 探测,不污染命中率统计 | harness |

### 失效路径(A7-A9)

| ID | 验收 | 检查方式 |
|---|---|---|
| A7 | `InvalidateBacklinksCache`(7 个写入口 + 手动调用)每次 inc `wiki_cache_invalidations_total{op=...}` 并 insert 一行 `wiki_backlinks_cache_invalidation_log` | harness |
| A8 | Build #22 sweeper 的 `DeleteStale` 在每批 DELETE 完成后,inc `wiki_cache_invalidations_total{op=cleanup_sweep}` 并 insert 一行 log(details 含 `{ttl_days, rows_deleted}`) | harness |
| A9 | `LogInvalidation` 失败仅 warn-log,不阻断 cache DELETE 语义 | harness |

### cache-status payload(A10-A11)

| ID | 验收 | 检查方式 |
|---|---|---|
| A10 | `GetPageBacklinksCacheStatus` 返回 payload 在现有 4 字段(`slug/computed_at/updated_at/source_event_id`)基础上,新增 4 字段:`kb_id`(从 path 注入)、`row_count`(全 KB 行数,cheap query)、`hit_ratio`(服务端 `hits_total/(hits_total+misses_total)`,全 KB 聚合)、`payload_size_bytes`(4 个 JSON 列字节和);JSON 序列化保持向后兼容(omitempty,旧字段不可空) | harness + smoke |
| A11 | 新端点 `GET /pages/:kb_id/backlinks/cache-statuses?limit=&offset=` 挂 `g.Admin()` 中间件,复用 `ListByKB`,返回 `{statuses:[...],total,limit,offset}` | smoke |

### source_event_id 关联(A12)

| ID | 验收 | 检查方式 |
|---|---|---|
| A12 | 写动作 handler 在调用 `InvalidateBacklinksCache` 前,从 `c.GetHeader("X-Request-ID")` 读出 ID(为空时新生成 UUID v4),随 `LogInvalidation` 一并存入 `source_event_id` | harness |

### i18n 零增量

本 Build 是纯后端 + 1 个 admin/debug 端点,前端不消费。**0 i18n keys**。

### Smoke + Verify 零回归承诺

- smoke-wiki-cache-observability.sh 7 步(创建→graph(冷 miss)→graph(命中)→看 metrics→改 page→看 invalidation_log 1 行→列表端点)
- Build #21/22 smoke 脚本(wiki-cache-cleanup + wiki-backlinks-cache + wiki-search-v2 + wiki-page-batch)零修改仍可跑
- harness 单测 ≥5 个(全部用 fake repo + cache counter)

## 检查清单(写完后自检)

- [ ] migration 000099 + rollback 验证
- [ ] hit/miss/error counter 在 3 处都有正确分支(命中 +1 hits,miss +1 misses,err +1 errors)
- [ ] `cache_writes_total` 只在 cache hit path 的 Upsert 调用处 +1(不在 status endpoint 的 Get 处 +1)
- [ ] invalidation counter 的 7 个 op label 都覆盖(手工触发一次每种 + grep `wiki_cache_invalidations_total{op="..."}`)
- [ ] Build #22 sweeper 的 `DeleteStale` 也走 `LogInvalidation`,verify log 表里有 `op="cleanup_sweep"` 的行
- [ ] cache-status 4 新字段在 empty cache 时返回合理零值(row_count=0 / hit_ratio=0 / payload_size_bytes=0)
- [ ] `/backlinks/cache-statuses` admin 中间件生效(非 admin 调用返回 403)
- [ ] X-Request-ID 透传:有值时存入 source_event_id,无值时新生成 UUID

## 已知限制

- `kb_id` label 在 metric 中基数受控,但极端 KB 数量 (>10k) 时 Prom 序列化开销不可忽略;目前不需要担心,但需在 spec 留一条「运维禁用 kb_id label」的逃生口(下次 Build 加 `--no-kb-label` flag)
- `invalidation_log` 表只 insert 不 delete(Build #22 cleanup 不动它);如果未来 log 表膨胀,需要 Build #X 加 retention(参考 `service/audit_log_retention.go` 已有 retention 模式)
- `hit_ratio` 是服务端聚合,前端不计算;如果未来需要分 KB / 分时间段图表,需扩 endpoint 或 Grafana 直接查 Prom
- 本地无 Go 工具链,`go build` / `go test ./...` 由 CI 验证