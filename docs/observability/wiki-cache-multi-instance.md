# Wiki Backlinks Cache — Multi-Instance Observability

> Build #29. Adds a `strategy` label to `wiki_cache_hits_total` and
> `wiki_cache_misses_total` so multi-instance deployments can answer
> "which invalidation strategy is hurting hit_ratio" without changing
> the cache row schema.

## What changed

| Metric | Before | After |
| --- | --- | --- |
| `wiki_cache_hits_total` | `{kb_id}` | `{kb_id, strategy}` |
| `wiki_cache_misses_total` | `{kb_id}` | `{kb_id, strategy}` |
| `wiki_cache_errors_total` | `{kb_id}` | **unchanged** (errors are strategy-agnostic) |
| `wiki_cache_invalidations_total` | `{op}` | unchanged (Build #23 already labels by op) |

`strategy` is one of 4 `SlugSetStrategy` values from `internal/types/wiki_page.go`:

| Value | Meaning |
| --- | --- |
| `self` | Only the slug's own row was wiped (most-local, default for reads) |
| `self_outgoing` | Slug + its out_links wiped |
| `self_incoming` | Slug + its in_links wiped |
| `kb_wide` | Whole-KB wipe (e.g. tenant-level reset) |

**Read-side convention**: the read path has no op, so it stamps `self`
by default. A future Build that promotes `strategy` to the row schema
can redistribute the `self` bucket without breaking this metric. See
"Roadmap" below.

## PromQL queries

### Q1 — Cluster-wide hit_ratio (across all instances + strategies)

```promql
sum(rate(wiki_cache_hits_total[5m]))
/
sum(rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

### Q2 — hit_ratio by KB

```promql
sum by (kb_id) (rate(wiki_cache_hits_total[5m]))
/
sum by (kb_id) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

### Q3 — hit_ratio by strategy (the new Build #29 view)

```promql
sum by (strategy) (rate(wiki_cache_hits_total[5m]))
/
sum by (strategy) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

Expected distribution under steady state: `self` should be the
majority (most reads see local-only invalidations). Spikes in
`kb_wide` or `self_outgoing` correlates with cache storms — those
are the buckets worth alerting on.

### Q4 — Per-instance hit_ratio (detects stuck pods)

```promql
sum by (instance) (rate(wiki_cache_hits_total[5m]))
/
sum by (instance) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

A pod that's stuck (e.g. corrupted in-memory cache) will show a
materially lower ratio than its peers. Alert when the delta exceeds
0.2 for 10+ minutes.

### Q5 — Per (kb, strategy) heat map (drilldown)

```promql
sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m]))
/
sum by (kb_id, strategy) (rate(wiki_cache_hits_total[5m]) + rate(wiki_cache_misses_total[5m]))
```

For dashboards: use a heatmap panel or table.

## Prometheus scrape config

Prometheus automatically adds `instance` from the scrape target.
For multi-pod deployments, ensure each pod has a unique scrape
target (e.g. via a headless Service in Kubernetes):

```yaml
# prometheus.yml
scrape_configs:
  - job_name: weknora
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      # Keep the pod IP + port as the instance label (default behavior)
      - source_labels: [__address__]
        action: keep
        regex: weknora-app:.*:8080
      - source_labels: [__address__]
        target_label: instance
      # Drop the noisy /metrics endpoint from non-app targets
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: weknora
```

For VM / bare-metal: list every instance as a target:

```yaml
scrape_configs:
  - job_name: weknora
    static_configs:
      - targets:
          - weknora-app-1.internal:8080
          - weknora-app-2.internal:8080
          - weknora-app-3.internal:8080
    metrics_path: /metrics
```

## Grafana panel

A ready-to-import panel lives at
`grafana/wiki-cache-hit-ratio-by-strategy.json`. It renders Q3
(hit_ratio by strategy) as a stacked time series.

## Backward compatibility

Old dashboards using `{kb_id=...}` directly **keep working** — the
extra `strategy` label is just an additional dimension. If a query
omits `strategy`, Prom aggregates across it by default (or use
`sum without (strategy) (...)` for explicit downward projection).

```promql
# Both forms are equivalent and return the same value:
sum(rate(wiki_cache_hits_total[5m]))
sum without (strategy) (rate(wiki_cache_hits_total[5m]))
```

## Alert recipes (suggested, not enabled by default)

```yaml
# alert: CacheStormKBWide
# Fires when kb-wide wipes drive the miss rate above 50% of all misses
- alert: WikiCacheKBWideStorm
  expr: |
    sum(rate(wiki_cache_misses_total{strategy="kb_wide"}[5m]))
    /
    sum(rate(wiki_cache_misses_total[5m])) > 0.5
  for: 10m
  annotations:
    summary: "Wiki backlinks cache seeing sustained kb_wide wipe pressure"
    runbook: "https://wiki.internal/runbooks/wiki-cache-kbwide-storm"

# alert: PodStuck
- alert: WikiCachePodStuck
  expr: |
    (
      sum by (instance) (rate(wiki_cache_hits_total[5m]))
      /
      (sum by (instance) (rate(wiki_cache_hits_total[5m])) + sum by (instance) (rate(wiki_cache_misses_total[5m])))
    )
    < 0.3
  for: 10m
  annotations:
    summary: "Pod {{ $labels.instance }} hit_ratio < 0.3 for 10+ minutes"
```

## Roadmap

| Future Build | Idea |
| --- | --- |
| B29+ | Promote `strategy` from "read-side self" to "row-level stamped at write time" — requires migration to add `invalidation_strategy VARCHAR(16)` to `wiki_backlinks_cache` |
| B29+ | Add a Histogram for hit/miss latency (currently only counters) |
| B29+ | Aggregate multi-region (requires recording rules in Prom) |
