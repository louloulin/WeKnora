package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiRealtimeSnapshotRepository persists Yjs document snapshots.
//
// The contract is intentionally narrow: the application layer owns the
// CRDT semantics (snapshot compaction triggers, GC of stale rows) and the
// repo just stores/loads bytes keyed by (tenant, kb, page).
type WikiRealtimeSnapshotRepository interface {
	// Upsert inserts or replaces the snapshot for the (tenant, kb, page) tuple.
	// On update, version auto-increments via DB trigger.
	Upsert(ctx context.Context, in types.WikiRealtimeSnapshotUpsert) (*types.WikiRealtimeSnapshot, error)

	// Get returns the latest snapshot or nil if none exists.
	Get(ctx context.Context, tenantID uint64, kbID, pageID string) (*types.WikiRealtimeSnapshot, error)

	// Delete removes the snapshot (used by admin purge / GC).
	Delete(ctx context.Context, tenantID uint64, kbID, pageID string) error
}

// WikiRealtimeSessionRepository tracks live presence.
//
// Sessions are short-lived (heartbeat-driven). The presence sweeper deletes
// rows whose last_heartbeat is older than the configured idle threshold.
type WikiRealtimeSessionRepository interface {
	// Upsert creates or refreshes a presence row keyed by
	// (tenant, page, client_id). color and display_name are overwritten
	// each call so reconnections pick up new style/name changes.
	Upsert(ctx context.Context, s *types.WikiRealtimeSession) error

	// ListByPage returns live sessions for a page, optionally filtered by
	// heartbeat (e.g. only last 30s).
	ListByPage(ctx context.Context, tenantID uint64, pageID string, since time.Time) ([]*types.WikiRealtimeSession, error)

	// DeleteByClient removes a single (user, client) presence row.
	DeleteByClient(ctx context.Context, tenantID uint64, pageID string, clientID uint64) error

	// SweepOlderThan removes rows whose last_heartbeat < cutoff. Returns the
	// number of rows deleted (for metrics).
	SweepOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
