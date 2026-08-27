# Build B29 — Prom 多实例 hit_ratio

## 一句话

给 `wiki_cache_hits_total` / `wiki_cache_misses_total` 加 `strategy` 标签(`self` / `self_outgoing` / `self_incoming` / `kb_wide`),落地一份 Prometheus 查询 + Grafana panel JSON,让多实例部署能直接看到"按失效策略分类"的命中率。后端代码增量小,文档/查询增量是大头。

## 现状(Why we need it)

| 现状 | 缺口 |
| --- | --- |
| `wiki_cache_hits_total{kb_id="..."}` 和 `misses_total{kb_id="..."}` 已经按 `kb_id` 切分 | 没按 `strategy` 切,看不到 `self_outgoing` 失效是不是带来"刷掉太多 hit" |
| 多实例部署(2+ pod 跑同一份 Prom) | Prom 自带 `instance` 标签,但只有"加和"查询模板(`sum by (kb_id) (rate(...))`),没文档化 |
| `metricCacheInvalidationsTotal{op="..."}` 已经按 op 切(Build #23) | `op` 是写侧视角(谁触发的失效),`strategy` 是查询侧视角(失效的扩散范围),两个正交,但后者缺失 |
| Build #28 把 `strategy` 已经传到 `InvalidateBacklinksCache` 签名 | **没有传递到 hit/miss counter** —— 错过一次完整闭环 |

## 验收(A1-A6)

| ID | 验收项 | 落点 |
| --- | --- | --- |
| **A1** | `wiki_cache_hits_total` / `wiki_cache_misses_total` 新增 `strategy` label(4 个取值) | `wiki_backlinks_cache_metrics.go:72-80` 改 `CounterOpts.Labels: []string{"kb_id", "strategy"}` |
| **A2** | 观测代码(`wiki_backlinks_cache_observability.go:64-69`)按 `strategy` 维度打标签 | 调用点加第 2 个 `WithLabelValues` 参数 |
| **A3** | 4 个 harness test 覆盖 `self` / `self_outgoing` / `self_incoming` / `kb_wide` 四种策略下的 hit + miss 计数 | `wiki_backlinks_cache_metrics_test.go` |
| **A4** | Prometheus 查询模板 ≥ 4 条 | `docs/observability/wiki-cache-multi-instance.md`(新文件),包含:多实例加和、按 kb 切分、按 strategy 切分、按 instance 切分 |
| **A5** | Grafana panel JSON(`wiki-cache-hit-ratio-by-strategy.json`)能直接 import | `docs/observability/grafana/` 下 |
| **A6** | smoke 脚本跑 4 种策略路径 + 解析 `/metrics` 端点确认 label 出现 | `scripts/smoke-wiki-cache-hit-ratio-labels.sh` |

## 关键决策(D1-D7)

| ID | 决策 | 推荐 |
| --- | --- | --- |
| **D1** | label 加在哪? | **`hits_total` + `misses_total` 两个 CounterVec 都加**(errors_total 不加,errors 与 strategy 无关,加只会膨胀 cardinality) |
| **D2** | label 取值边界? | **4 个**:`self` / `self_outgoing` / `self_incoming` / `kb_wide`(沿用 Build #28 `SlugSetStrategy`) |
| **D3** | 路径上 `strategy` 是从哪传过来的? | **`cacheInvalidator.Resolve(ctx, op, kb, slug)` 已经返回 `(slugs, strategy, err)`**,Build #28-B1b 已经完成;**只需在观测点把它取出** |
| **D4** | 是否引入新的 CounterVec 名? | **不改名**,只在原 CounterVec 上 append label(Prom client 允许,但需要同步注册)。注册顺序很重要 —— 必须保证 label key 一致,否则 `panic: inconsistent label cardinality` |
| **D5** | 多实例聚合查询模式 | **标准 Prom 模式**:`sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m])) / sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))` |
| **D6** | Grafana 是否走 Grafana.com 官方 dashboard? | **不**,我们没上游 dashboard,且 Build #28 之后命名空间定型,自己写 JSON 即可 |
| **D7** | 是否上 PR 文档? | **是**,在 `docs/observability/` 下新增 1 个 markdown + 1 个 JSON |

## 改动清单

| 文件 | 行数 | 用途 |
| --- | --- | --- |
| `internal/application/service/wiki_backlinks_cache_metrics.go` | +6 | hits/misses CounterVec 加 `strategy` label |
| `internal/application/service/wiki_backlinks_cache_observability.go` | +20/-10 | 调用 `WithLabelValues(kb, strategy)`,从 cacheInvalidator.Resolve 取 strategy |
| `internal/application/service/wiki_backlinks_cache_metrics_test.go`(NEW) | +90 | 4 个用例,每个覆盖一种 strategy |
| `internal/application/service/wiki_backlinks_cache_observability_test.go`(可能小幅调) | ±5 | 现有测试断言更新 |
| `docs/observability/wiki-cache-multi-instance.md`(NEW) | +80 | Prom 查询模板 + 故障排查 checklist |
| `docs/observability/grafana/wiki-cache-hit-ratio-by-strategy.json`(NEW) | +60 | Grafana 9.x panel JSON |
| `scripts/smoke-wiki-cache-hit-ratio-labels.sh`(NEW) | +30 | 拉 `/metrics`,grep `strategy="self"` 4 次 |

## 范围外(明确不做)

- ❌ 不改 `metricCacheErrorsTotal` label(errors 与 strategy 无关)
- ❌ 不做 instance-level histogram(粒度太细,会爆 cardinality)
- ❌ 不引入 Prom relabel 配置(留给运维)
- ❌ 不做实时 dashboard 截图(留给 ops)
- ❌ 不上 Prometheus alert rule(留给 ops)

## 风险点

| 风险 | 缓解 |
| --- | --- |
| CounterVec 改 label 顺序会导致启动 panic | `prometheus.MustRegister` 之前做 init check;首次跑 harness 验 |
| 多实例 scrape 后 `instance` 标签出现,但早期 Prom 配置可能没保留 | 文档里给运维一条 relabel 配置片段 |
| 旧 dashboard 可能 break(hits/misses 多了 strategy label) | Grafana panel 用 `sum without (strategy)`,自动向下兼容 |
| Harness test 用了全局 prometheus.DefaultRegisterer,改 label 名会冲突 | test 用 `prometheus.NewRegistry()`,不污染全局 |

## 排期

| 步骤 | 预计时间 |
| --- | --- |
| 写 brief + spec | 0.1 天(当前) |
| 等用户确认 Shape | 用户异步 |
| 改 metrics + observability 调用 + 4 个 harness test | 0.3 天 |
| 写 docs/observability/*.md + grafana JSON | 0.2 天 |
| smoke 脚本 + verify + commit + push + reply | 0.2 天 |
| **总计** | **0.8 天** |
