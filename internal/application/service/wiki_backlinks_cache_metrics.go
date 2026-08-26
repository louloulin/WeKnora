package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Build #22 — wiki backlinks cache cleanup metrics.
//
// These four metrics back the D6 + D7 decisions from the Build #22
// brief:
//   - metricCleanupDeletedTotal (counter) — total rows actually deleted
//   - metricCleanupDryRunTotal (counter) — total dry-run sweep invocations
//   - metricCleanupDurationSeconds (histogram) — sweep wall-clock
//   - metricCacheRowsRemaining (gauge) — current table size for alert
//
// Naming follows the Prometheus convention: snake_case + _total
// suffix for counters. All are in the "wiki_cache_cleanup" subsystem
// namespace so Grafana dashboards can group them.
//
// We use promauto so the metrics auto-register with the default
// prometheus.DefaultRegisterer on package init — that is what
// cmd/server's existing /metrics handler scrapes (assuming one
// exists; if not, this is still safe because promauto registers
// against the default registry which the eventual handler will pick
// up).
var (
	metricCleanupDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wiki_cache_cleanup_deleted_total",
		Help: "Total number of wiki_backlinks_cache rows deleted by the sweeper.",
	})

	metricCleanupDryRunTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wiki_cache_cleanup_dry_run_total",
		Help: "Total number of dry-run wiki_backlinks_cache sweeps executed.",
	})

	metricCleanupDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "wiki_cache_cleanup_duration_seconds",
		Help:    "Wall-clock duration of a real (non-dry-run) wiki_backlinks_cache sweep.",
		Buckets: prometheus.ExponentialBuckets(0.05, 2, 12), // 50ms .. ~200s
	})

	metricCacheRowsRemaining = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wiki_cache_rows_remaining",
		Help: "Current number of rows in the wiki_backlinks_cache table (refreshed after every sweep).",
	})

	// Build #26 — wiki_cache_backref_rows_remaining moved to the
	// wikicachemetrics sub-package so the repository can update it
	// incrementally on Upsert / Delete / DeleteByKB / DeleteStale
	// without importing the service package (which would create a
	// dependency cycle: service → repository).

	// Build #23 — wiki backlinks cache observability (D1-D3).
	//
	// Five new counters + zero gauge additions:
	//   - hits_total / misses_total / errors_total: outcome counter
	//     for ListBacklinkGraph's cache Get. kb_id label is bounded
	//     (typically <10k) so cardinality is acceptable; if it ever
	//     spikes, operators can disable via Prom relabel rules (D1
	//     escape hatch documented in spec).
	//   - writes_total: every Upsert in the cache miss → recompute →
	//     writeback path. No label (single global write rate).
	//   - invalidations_total: every InvalidateBacklinksCache call,
	//     labeled by the BacklinkCacheInvalidateOp value (8 labels
	//     total: the 7 write ops + cleanup_sweep from Build #22).
	//
	// The cleanup_sweep label is the bridge to Build #22 — sweeper
	// invocations also bump this counter, which keeps the Prom
	// observability surface unified across the subsystem.
	metricCacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wiki_cache_hits_total",
		Help: "Total cache hits in ListBacklinkGraph, by kb_id.",
	}, []string{"kb_id"})

	metricCacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wiki_cache_misses_total",
		Help: "Total cache misses in ListBacklinkGraph, by kb_id.",
	}, []string{"kb_id"})

	metricCacheErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wiki_cache_errors_total",
		Help: "Total cache errors (Get failure or decode failure) in ListBacklinkGraph, by kb_id.",
	}, []string{"kb_id"})

	metricCacheWritesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "wiki_cache_writes_total",
		Help: "Total Upsert calls into wiki_backlinks_cache.",
	})

	metricCacheInvalidationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "wiki_cache_invalidations_total",
		Help: "Total InvalidateBacklinksCache invocations, by op.",
	}, []string{"op"})

	// Build #24 — ACL-change reverse-lookup wipe duration. Only the
	// large-KB path (>10k cache rows) records here — the small-KB
	// path is sub-millisecond and not worth a histogram sample. The
	// bucket range covers 5ms..~150s, matching the range of a real
	// reverse-lookup against a 100k-row table on PG/MySQL.
	metricCacheAclChangeWipeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "wiki_cache_acl_change_wipe_duration_seconds",
		Help:    "Wall-clock duration of the Build #24 ACL→cache reverse-lookup wipe (large-KB path only).",
		Buckets: prometheus.ExponentialBuckets(0.005, 2, 14), // 5ms .. ~80s
	})
)
