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
)
