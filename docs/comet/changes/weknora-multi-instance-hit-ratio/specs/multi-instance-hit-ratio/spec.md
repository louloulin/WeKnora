# B29 — multi-instance-hit-ratio spec

> 详见 `brief.md`,本 spec 仅列验收矩阵 + 强制 checklist + Prom 查询参考。

## 验收矩阵

| ID | 测试 | 命令 | 期望 |
| --- | --- | --- | --- |
| A1 | CounterVec label 完整性 | `grep -c 'strategy' wiki_backlinks_cache_metrics.go` | ≥ 2(hits + misses) |
| A2 | 观测调用点更新 | `grep -c 'WithLabelValues' wiki_backlinks_cache_observability.go` | ≥ 2(hits + misses 都传 2 个 label) |
| A3 | Harness 4 用例 | `go test -short -run 'TestCacheMetricsStrategy' ./internal/application/service/...` | exit 0,4 个 sub-test 全过 |
| A4 | Prom 文档存在 | `test -f docs/observability/wiki-cache-multi-instance.md` | exit 0 |
| A5 | Grafana JSON 合法 | `python3 -c "import json; json.load(open('docs/observability/grafana/wiki-cache-hit-ratio-by-strategy.json'))"` | exit 0 |
| A6 | smoke 跑通 | `bash scripts/smoke-wiki-cache-hit-ratio-labels.sh`(无 BASE_URL) | exit 0,grep 找到 `strategy="self"` 等 4 个 |

## 强制 checklist(commit 前自查)

- [ ] `metricCacheHitsTotal` 与 `metricCacheMissesTotal` 的 `NewCounterVec` 第二参数 `[]string{...}` **完全一致**(`"kb_id"` + `"strategy"` 顺序一致),否则启动 `panic: inconsistent label cardinality`
- [ ] `wiki_backlinks_cache_observability.go` 的 `WithLabelValues(...)` 调用从 1 个 label 升到 2 个 label,**没有遗漏任何调用点**
- [ ] harness test 用 `prometheus.NewRegistry()` 而非全局 `DefaultRegisterer`,避免污染其他测试
- [ ] `strategy` label 值必须从 `types.SlugSetStrategy` 来,**不要在 metrics 文件里硬编码字符串**
- [ ] 文档里给的 Prom 查询必须能在 Prom 2.40+ 通过(`sum by` + `rate` 语法,不需要 histogram_quantile)
- [ ] Grafana JSON 里 datasource 字段写 `"type": "prometheus"`,uid 留 `"${DS_PROMETHEUS}"`(import 时用户填)
- [ ] smoke 脚本**无 BASE_URL 时直接退 0**(dry-run safe),与既有 smoke 风格一致

## Prom 查询模板(写入 `docs/observability/wiki-cache-multi-instance.md`)

```promql
# Q1 — 全集群 hit_ratio(多实例加和)
sum(rate(wiki_cache_hits_total[5m]))
/
sum(rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))

# Q2 — 按 kb_id 切分
sum by (kb_id) (rate(wiki_cache_hits_total[5m]))
/
sum by (kb_id) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))

# Q3 — 按 strategy 切分(本 Build 的核心新增)
sum by (strategy) (rate(wiki_cache_hits_total[5m]))
/
sum by (strategy) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))

# Q4 — 按 instance 切分(健康度,看单实例 hit_ratio 是否掉队)
sum by (instance) (rate(wiki_cache_hits_total[5m]))
/
sum by (instance) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))

# Q5 — 综合:看某 KB 在某策略下的命中率
sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m]))
/
sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

## Prom relabel 提示(写到 docs,**不**自动 apply)

```yaml
# prometheus.yml — 在 scrape_configs 里加,保留 instance label
- job_name: 'weknora'
  static_configs:
    - targets: ['app-1:8080', 'app-2:8080']
  relabel_configs:
    # 保留 __address__ 作为 instance;Prom 默认行为就是如此,这里只是显式记录
    - source_labels: [__address__]
      target_label: instance
```

## 验证命令(本机 dry-run)

```bash
# 1. CounterVec 标签一致性
grep -A1 'NewCounterVec' internal/application/service/wiki_backlinks_cache_metrics.go

# 2. 观测点调用数
grep -c 'WithLabelValues' internal/application/service/wiki_backlinks_cache_observability.go

# 3. JSON 合法
python3 -c "import json; json.load(open('docs/observability/grafana/wiki-cache-hit-ratio-by-strategy.json'))"

# 4. smoke dry-run
bash -n scripts/smoke-wiki-cache-hit-ratio-labels.sh && \
  BASE_URL= bash scripts/smoke-wiki-cache-hit-ratio-labels.sh
```

## 不在 spec 内(留给后续 Build)

- Prom alert rule(B29 完成后再单独 small Build)
- Cardinality 治理(若 kb_id 真的 >10k,留给 ops 用 relabel drop)
- 多 region 聚合(目前假设单 region)
- `errors_total` 加 strategy label(已明确不做)
- 单 instance 的 hit_ratio histogram(同上)
