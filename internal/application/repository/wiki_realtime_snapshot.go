package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// wikiRealtimeSnapshotRepository is the GORM implementation of the snapshot
// repo. It is intentionally thin: the application service owns compaction
// policy (size threshold, time threshold), the repo just upserts and reads.
type wikiRealtimeSnapshotRepository struct {
	db *gorm.DB
}

// NewWikiRealtimeSnapshotRepository wires the snapshot repo to the supplied
// GORM handle. Returns the interface so callers depend on the contract.
func NewWikiRealtimeSnapshotRepository(db *gorm.DB) interfaces.WikiRealtimeSnapshotRepository {
	return &wikiRealtimeSnapshotRepository{db: db}
}

func (r *wikiRealtimeSnapshotRepository) Upsert(ctx context.Context, in types.WikiRealtimeSnapshotUpsert) (*types.WikiRealtimeSnapshot, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot upsert invalid: %w", err)
	}
	row := &types.WikiRealtimeSnapshot{
		TenantID:    in.TenantID,
		KBID:        in.KBID,
		PageID:      in.PageID,
		YDocState:   in.YDocState,
		VectorClock: in.VectorClock,
		SizeBytes:   in.SizeBytes,
	}
	// On conflict, replace state + size + vector_clock; created_at is
	// preserved by the column exclusion below so the audit history keeps
	// the original join time.
	// Upsert via portable dialect-aware SQL. SQLite uses INSERT OR REPLACE;
	// Postgres uses ON CONFLICT DO UPDATE. Both end up with one row per
	// (tenant, kb, page) tuple with the supplied state.
	var err error
	dialect := r.db.Dialector.Name()
	switch dialect {
	case "sqlite":
		// Use a session so the subsequent Get() reuses the same connection.
		session := r.db.WithContext(ctx).Session(&gorm.Session{Initialized: true})
		err = session.Exec(
			`INSERT OR REPLACE INTO wiki_doc_snapshots
			 (tenant_id, kb_id, page_id, ydoc_state, vector_clock, version, size_bytes, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, COALESCE((SELECT version FROM wiki_doc_snapshots WHERE tenant_id=? AND kb_id=? AND page_id=?), 0)+1, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			in.TenantID, in.KBID, in.PageID, in.YDocState, in.VectorClock,
			in.TenantID, in.KBID, in.PageID,
			in.SizeBytes,
		).Error
	default: // postgres + others
		// Use a session so the subsequent Get() reuses the same connection.
		session := r.db.WithContext(ctx).Session(&gorm.Session{Initialized: true})
		err = session.Exec(
			`INSERT INTO wiki_doc_snapshots (tenant_id, kb_id, page_id, ydoc_state, vector_clock, version, size_bytes, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 1, ?, NOW(), NOW())
			 ON CONFLICT (tenant_id, kb_id, page_id)
			 DO UPDATE SET ydoc_state = EXCLUDED.ydoc_state, vector_clock = EXCLUDED.vector_clock, size_bytes = EXCLUDED.size_bytes`,
			in.TenantID, in.KBID, in.PageID, in.YDocState, in.VectorClock, in.SizeBytes,
		).Error
	}
	if err != nil {
		logger.Errorf(ctx, "wiki realtime snapshot upsert failed: tenant=%d kb=%s page=%s err=%v",
			in.TenantID, in.KBID, in.PageID, err)
		return nil, fmt.Errorf("upsert snapshot: %w", err)
	}
	// Re-read via the same transaction so the snapshot we just wrote is visible.
	got, err := r.Get(ctx, in.TenantID, in.KBID, in.PageID)
	if err != nil {
		return nil, err
	}
	if got == nil {
		row.Version = 1
		row.CreatedAt = time.Now().UTC()
		row.UpdatedAt = row.CreatedAt
		return row, nil
	}
	return got, nil
}

func (r *wikiRealtimeSnapshotRepository) Get(ctx context.Context, tenantID uint64, kbID, pageID string) (*types.WikiRealtimeSnapshot, error) {
	var row types.WikiRealtimeSnapshot
	// Raw SQL avoids GORM's struct-mapping quirks across dialects and
	// guarantees the same connection is used for read-after-write on
	// SQLite (which holds the in-memory DB per-connection).
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, kb_id, page_id, ydoc_state, vector_clock, version, size_bytes, created_at, updated_at
		 FROM wiki_doc_snapshots WHERE tenant_id = ? AND kb_id = ? AND page_id = ?`,
		tenantID, kbID, pageID,
	).Scan(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		// Raw Scan returns no error on empty result in some drivers; check ID.
		if row.ID == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *wikiRealtimeSnapshotRepository) Delete(ctx context.Context, tenantID uint64, kbID, pageID string) error {
	res := r.db.WithContext(ctx).
		Model(&types.WikiRealtimeSnapshot{}).
		Table("wiki_doc_snapshots").
		Where("tenant_id = ? AND kb_id = ? AND page_id = ?", tenantID, kbID, pageID).
		Delete(&types.WikiRealtimeSnapshot{})
	if res.Error != nil {
		return fmt.Errorf("delete snapshot: %w", res.Error)
	}
	return nil
}

// Compile-time assertion that the repo satisfies the interface contract.
var _ interfaces.WikiRealtimeSnapshotRepository = (*wikiRealtimeSnapshotRepository)(nil)

// time marker so the package is referenced even on builds that only use
// the time import indirectly (some linters flag unused imports otherwise).
var _ = time.Second
