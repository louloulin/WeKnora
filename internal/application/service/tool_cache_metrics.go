package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Build #30 — chat tool cache metrics.
//
// Five counters + one gauge back the ToolCache decision surface:
//   - hits_total / misses_total — outcome counter for cache Get,
//     labelled by tenant_id + tool_name. tenant_id cardinality is
//     bounded (<10k) and tool_name is the closed enum of registered
//     tools (today <50), so the resulting label series is acceptable
//     for a Prom scrape interval of 15s.
//   - evictions_total — increments every time Set has to drop the
//     LRU tail to make room. Labels: tenant_id + tool_name, so an
//     operator can spot a tenant or tool whose cache hit rate is
//     forcing heavy eviction churn.
//   - writes_total — every Set that successfully inserted an entry.
//   - invalidations_total — every InvalidateByTenant or
//     InvalidateByTool call, labelled by reason ("tenant",
//     "tool"). Drained in B31 by KB-write invalidation hooks.
//   - size_entries — gauge sampled at Get/Set time. Tracks the
//     current entry count for the addressed tenant; useful for
//     capacity planning dashboards.
//
// All counters use promauto so they register with
// prometheus.DefaultRegisterer on package init. The /metrics handler
// in cmd/server picks them up without further wiring.
var (
	metricToolCacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_tool_cache_hits_total",
		Help: "Total ToolCache hits, by kb_id (tenant) and tool_name.",
	}, []string{"kb_id", "tool_name"})

	metricToolCacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_tool_cache_misses_total",
		Help: "Total ToolCache misses, by kb_id (tenant) and tool_name.",
	}, []string{"kb_id", "tool_name"})

	metricToolCacheEvictionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_tool_cache_evictions_total",
		Help: "Total ToolCache LRU evictions, by kb_id (tenant) and tool_name.",
	}, []string{"kb_id", "tool_name"})

	metricToolCacheWritesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_tool_cache_writes_total",
		Help: "Total successful Set calls into ToolCache.",
	})

	metricToolCacheInvalidationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_tool_cache_invalidations_total",
		Help: "Total ToolCache Invalidate calls, by reason (tenant|tool).",
	}, []string{"reason"})

	metricToolCacheSizeEntries = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "chat_tool_cache_size_entries",
		Help: "Current number of entries in ToolCache for the addressed tenant.",
	}, []string{"kb_id"})
)