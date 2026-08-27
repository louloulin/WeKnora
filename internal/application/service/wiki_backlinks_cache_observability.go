package service

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Build #23 — observability helpers for the wiki backlinks cache layer.
//
// This file holds three things:
//   - per-process atomic counters (hit / miss / write) used by the
//     cache-status handler to surface a hit_ratio without scraping Prom
//   - ctx → source_event_id / actor_user_id extractors used by every
//     InvalidateBacklinksCache call site to stamp the audit log row
//   - public Snapshot() for tests + the cache-status handler to read
//     a consistent (hits, misses) tuple atomically
//
// The Prom counter (metricCacheHitsTotal) carries the durable view across
// pods; the atomic counters carry the cheap in-process view that the
// cache-status endpoint exposes without a Prom scrape round-trip. Both
// are incremented together in the service hot paths.

// wikiCacheObsState is the per-process atomic counter pair. Atomic loads
// give a consistent (hits, misses) snapshot via Snapshot().
type wikiCacheObsState struct {
	hits   atomic.Uint64
	misses atomic.Uint64
}

var wikiCacheObs wikiCacheObsState

// wikiCacheObsSnapshot returns a consistent (hits, misses) tuple. Safe
// to call from any goroutine; atomic loads give each value independently
// but they don't need to be a single read for this use case (the panel
// just displays a ratio).
type wikiCacheObsSnapshot struct {
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
}

func wikiCacheObsRead() wikiCacheObsSnapshot {
	return wikiCacheObsSnapshot{
		Hits:   wikiCacheObs.hits.Load(),
		Misses: wikiCacheObs.misses.Load(),
	}
}

// kbLabelFor returns the kb_id we should stamp on Prom counters. Empty
// kb_id (extremely rare — only when callers pass through an unknown
// tenant) collapses to "global" so the metric never loses a sample.
func kbLabelFor(kbID string) string {
	if kbID == "" {
		return "global"
	}
	return kbID
}

func wikiCacheObsIncHit(kbID string) {
	wikiCacheObs.hits.Add(1)
	metricCacheHitsTotal.WithLabelValues(kbLabelFor(kbID), readStrategyLabel()).Inc()
}

func wikiCacheObsIncMiss(kbID string) {
	wikiCacheObs.misses.Add(1)
	metricCacheMissesTotal.WithLabelValues(kbLabelFor(kbID), readStrategyLabel()).Inc()
}

// readStrategyLabel returns the strategy label value used on hit / miss
// counters. The read path has no op — it doesn't know what kind of
// invalidation last wiped the row. As a pragmatic, additive step
// (Build #29), we stamp "self" — the most-local invalidation strategy —
// so the metric label cardinality stays bounded and dashboard queries
// keep working. A future Build can promote this to the row-level
// invalidation_strategy stamped at write time without changing the
// metric schema; the "self" bucket will simply get redistributed.
func readStrategyLabel() string {
	return string(types.SlugSetStrategySelf)
}

func wikiCacheObsIncError(kbID string) {
	metricCacheErrorsTotal.WithLabelValues(kbLabelFor(kbID)).Inc()
}

func wikiCacheObsIncWrite() {
	metricCacheWritesTotal.Inc()
}

// wikiCacheObsReset is for tests only. Production code never calls it.
var wikiCacheObsResetMu sync.Mutex

func wikiCacheObsReset() {
	wikiCacheObsResetMu.Lock()
	defer wikiCacheObsResetMu.Unlock()
	wikiCacheObs.hits.Store(0)
	wikiCacheObs.misses.Store(0)
}

// ctx keys for the two Build #23 cross-layer signals.
type wikiObsCtxKey int

const (
	wikiObsCtxKeySourceEventID wikiObsCtxKey = iota + 1
	wikiObsCtxKeyActorUserID
)

// WithWikiObsSourceEventID stores the X-Request-ID (or generated UUID)
// on the context so InvalidateBacklinksCache can stamp the audit log.
// Pass-through pattern — gin handlers call this once per request, the
// service layer reads it. Empty string is preserved as "unknown" rather
// than dropping the audit row.
func WithWikiObsSourceEventID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, wikiObsCtxKeySourceEventID, id)
}

// wikiSourceEventIDFromContext returns the wiki-specific source_event_id
// if explicitly stamped, otherwise falls back to the canonical
// types.RequestIDFromContext — the latter is set by middleware.RequestID()
// on every request (X-Request-ID header or fresh UUID v4 fallback), so
// the cache_invalidation_log row always carries a usable correlation id
// without the handler having to do anything.
func wikiSourceEventIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(wikiObsCtxKeySourceEventID).(string); ok && v != "" {
		return v
	}
	if v, ok := types.RequestIDFromContext(ctx); ok && v != "" {
		return v
	}
	return ""
}

// WithWikiObsActorUserID stores the actor's user id. Zero values (no
// auth / batch jobs) are kept as nil so the audit log row shows
// actor_user_id IS NULL — operators can grep "where did this come from"
// without joining against users.
func WithWikiObsActorUserID(ctx context.Context, id uint64) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, wikiObsCtxKeyActorUserID, id)
}

// wikiActorUserIDFromContext returns the actor's user id, falling back
// to the canonical types.UserIDFromContext so handlers don't need an
// explicit wrap — auth middleware already stamps the user id on the
// context. Returns nil if neither key is set.
func wikiActorUserIDFromContext(ctx context.Context) *uint64 {
	if v, ok := ctx.Value(wikiObsCtxKeyActorUserID).(uint64); ok && v > 0 {
		return &v
	}
	if v := types.UserIDFromContext(ctx); v > 0 {
		return &v
	}
	return nil
}

// logCacheError is a tiny helper for the read path so the three error
// counters stay visually parallel at the call site.
func logCacheError(ctx context.Context, where string, err error) {
	logger.Warnf(ctx, "wiki backlinks cache %s: %v", where, err)
}