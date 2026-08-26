package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wikiBacklinksCacheRepository is the GORM implementation of
// WikiBacklinksCacheRepository (Build #21). The table layout is
// declared in migrations/versioned/000097_wiki_backlinks_cache.up.sql;
// the GORM struct mirrors it 1:1 with primary key (kb_id, slug).
//
// The four payload columns + stats column are TEXT and the service
// layer is responsible for serialising / deserialising JSON. The repo
// is dialect-agnostic — no jsonb / json_extract / SQL string funcs.
type wikiBacklinksCacheRepository struct {
	db *gorm.DB
}

// NewWikiBacklinksCacheRepository wires the GORM-backed cache repo
// into the DI container. Returns the interface so callers depend on
// the contract, not the struct.
func NewWikiBacklinksCacheRepository(db *gorm.DB) interfaces.WikiBacklinksCacheRepository {
	return &wikiBacklinksCacheRepository{db: db}
}

// Get returns the cached row for (kbID, slug). Missing rows produce
// (nil, nil) — the read path treats this as a cache miss and
// recomputes. Other errors bubble up so the service can log + decide.
func (r *wikiBacklinksCacheRepository) Get(
	ctx context.Context,
	kbID string,
	slug string,
) (*types.WikiBacklinksCacheRow, error) {
	if kbID == "" || slug == "" {
		return nil, nil
	}
	var row types.WikiBacklinksCacheRow
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND slug = ?", kbID, slug).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// Upsert writes a new row or replaces the existing one for the same
// (kb_id, slug). The service stamps computed_at / updated_at before
// calling, but we re-stamp here to keep the repo as the single source
// of truth for time fields — avoids drift if a caller forgets.
func (r *wikiBacklinksCacheRepository) Upsert(
	ctx context.Context,
	row *types.WikiBacklinksCacheRow,
) error {
	if row == nil {
		return errors.New("wikiBacklinksCacheRepository.Upsert: nil row")
	}
	if row.KbID == "" || row.Slug == "" {
		return errors.New("wikiBacklinksCacheRepository.Upsert: empty kb_id or slug")
	}
	now := time.Now().UTC()
	row.ComputedAt = now
	row.UpdatedAt = now
	// clause.OnConflict{DoUpdates: clause.AssignmentColumns([])} covers
	// PG (`ON CONFLICT DO UPDATE SET`), MySQL (`ON DUPLICATE KEY UPDATE`),
	// and SQLite (`ON CONFLICT DO UPDATE SET`) with one clause — GORM
	// translates the SQL per dialect.
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{
				"direct_json",
				"indirect_json",
				"related_json",
				"broken_json",
				"stats_json",
				"source_event_id",
				"computed_at",
				"updated_at",
			}),
		}).
		Create(row).Error
}

// Delete removes rows for (kbID, slug IN (?, ?, ...)). Returns the
// affected count for the caller's warning log. Empty slug list is a
// no-op (returns 0, nil) so callers can pass through the unfiltered
// Resolve output safely.
func (r *wikiBacklinksCacheRepository) Delete(
	ctx context.Context,
	kbID string,
	slugs []string,
) (int64, error) {
	if kbID == "" || len(slugs) == 0 {
		return 0, nil
	}
	// Dedup — DELETE IN with duplicates is harmless but wastes bind
	// slots. Cheap when the typical input is < 50 slugs.
	uniq := make([]string, 0, len(slugs))
	seen := make(map[string]struct{}, len(slugs))
	for _, s := range slugs {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("kb_id = ? AND slug IN ?", kbID, uniq).
		Delete(&types.WikiBacklinksCacheRow{})
	return res.RowsAffected, res.Error
}

// ListByKB returns slim cache statuses (computed_at + source_event_id)
// for one KB, paginated. Used by the admin / debug
// GET /backlinks/cache-status endpoint.
func (r *wikiBacklinksCacheRepository) ListByKB(
	ctx context.Context,
	kbID string,
	limit int,
	offset int,
) ([]*types.WikiBacklinksCacheStatus, int64, error) {
	if kbID == "" {
		return []*types.WikiBacklinksCacheStatus{}, 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&types.WikiBacklinksCacheRow{}).
		Where("kb_id = ?", kbID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*types.WikiBacklinksCacheStatus{}, 0, nil
	}
	var rows []types.WikiBacklinksCacheRow
	if err := r.db.WithContext(ctx).
		Where("kb_id = ?", kbID).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	statuses := make([]*types.WikiBacklinksCacheStatus, 0, len(rows))
	for i := range rows {
		statuses = append(statuses, &types.WikiBacklinksCacheStatus{
			Slug:          rows[i].Slug,
			ComputedAt:    rows[i].ComputedAt,
			UpdatedAt:     rows[i].UpdatedAt,
			SourceEventID: rows[i].SourceEventID,
		})
	}
	return statuses, total, nil
}

// DeleteStale removes cache rows whose updated_at is strictly older
// than `before`, up to `limit` rows. Used by the Build #22 sweeper.
//
// The GORM `Limit(...)` here applies to the DELETE itself: GORM emits
// `DELETE ... WHERE id IN (SELECT id FROM ... LIMIT n)`. PostgreSQL
// supports that pattern directly; MySQL needs the inner SELECT to be
// wrapped in another SELECT (GORM does this automatically). SQLite
// supports LIMIT on DELETE since 3.7+.
//
// Caller is responsible for looping (call → check RowsAffected < limit
// → call again) to drain the stale set without holding a giant
// transaction. CleanupService.Run does this.
func (r *wikiBacklinksCacheRepository) DeleteStale(
	ctx context.Context,
	before time.Time,
	limit int,
) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	if before.IsZero() {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Where("updated_at < ?", before).
		Limit(limit).
		Delete(&types.WikiBacklinksCacheRow{})
	return res.RowsAffected, res.Error
}

// CountRows returns the total number of cache rows across all KBs.
// Used by the sweeper's stale-monitoring gauge (cache_rows_remaining)
// so the alert fires when the table grows past the configured
// threshold regardless of TTL state — TTL cleanup may not keep up with
// churn in a hot KB, and operators need to see that.
func (r *wikiBacklinksCacheRepository) CountRows(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.WikiBacklinksCacheRow{}).
		Count(&count).Error
	return count, err
}

// ListStaleForUpdate returns up to `limit` stale (kb_id, slug) pairs
// under SELECT ... FOR UPDATE SKIP LOCKED. The caller is expected to
// pass an already-open *gorm.DB transaction (`tx`) so the lock lives
// only as long as the surrounding tx.
//
// SKIP LOCKED semantics (PG 9.5+, MySQL 8.0+, SQLite 3.37+): another
// concurrent cleanup on a different instance / worker grabs a disjoint
// slice of the stale set without blocking. If the dialect is older,
// the row-level lock degrades to plain `FOR UPDATE` (blocks) — the
// sweeper still works, just slower under contention. We accept the
// degradation rather than refuse to ship on older deployments.
//
// On SQLite (single-writer by definition), SKIP LOCKED is a no-op but
// harmless — the global write lock already serialises sweeps.
func (r *wikiBacklinksCacheRepository) ListStaleForUpdate(
	ctx context.Context,
	tx *gorm.DB,
	before time.Time,
	limit int,
) ([]string, error) {
	if tx == nil {
		return nil, errors.New("wikiBacklinksCacheRepository.ListStaleForUpdate: nil tx")
	}
	if limit <= 0 {
		limit = 1000
	}
	if before.IsZero() {
		return []string{}, nil
	}
	// We return composite keys (kb_id + "\x00" + slug) because the
	// cleanup caller needs both to issue precise DELETE statements.
	// Using a string union keeps the cross-dialect SQL simple — GORM's
	// distinct dialect-specific Scan glue would otherwise need a small
	// anonymous struct.
	var rows []struct {
		KbID string
		Slug string
	}
	err := tx.WithContext(ctx).
		Table("wiki_backlinks_cache").
		Select("kb_id, slug").
		Where("updated_at < ?", before).
		Order("updated_at ASC").
		Limit(limit).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.KbID+"\x00"+row.Slug)
	}
	return out, nil
}

// LogInvalidation persists a row in wiki_backlinks_cache_invalidation_log.
// Build #23 — every call to InvalidateBacklinksCache (and the Build #22
// sweeper's DeleteStale) calls this. The entry's Details is a JSON string
// the caller has already marshalled; the repo just inserts it. Failures
// are warn-logged by the service layer but never bubble — losing one log
// row must not block a cache DELETE.
//
// We stamp CreatedAt here if the caller left it zero, so the
// service-layer caller doesn't have to know the column default. The
// caller may pre-set it for tests.
func (r *wikiBacklinksCacheRepository) LogInvalidation(
	ctx context.Context, entry *types.WikiBacklinksCacheInvalidationLogEntry,
) error {
	if entry == nil {
		return errors.New("wikiBacklinksCacheRepository.LogInvalidation: nil entry")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(entry).Error
}

// ListInvalidationLog returns invalidation log entries for one KB,
// newest first, paginated. The (kb_id, created_at DESC) index in
// migration 000099 keeps this cheap even when the table grows.
func (r *wikiBacklinksCacheRepository) ListInvalidationLog(
	ctx context.Context, kbID string, limit int, offset int,
) ([]*types.WikiBacklinksCacheInvalidationLogEntry, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&types.WikiBacklinksCacheInvalidationLogEntry{}).
		Where("kb_id = ?", kbID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []*types.WikiBacklinksCacheInvalidationLogEntry
	err := r.db.WithContext(ctx).
		Where("kb_id = ?", kbID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// CountByKB returns the number of cache rows for one KB. Cheap because
// the table's primary key is (kb_id, slug) — Postgres / MySQL / SQLite
// can satisfy this from the index without scanning the JSON payload
// columns.
func (r *wikiBacklinksCacheRepository) CountByKB(ctx context.Context, kbID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.WikiBacklinksCacheRow{}).
		Where("kb_id = ?", kbID).
		Count(&count).Error
	return count, err
}

// SumPayloadSizeByKB sums the byte length of the five payload JSON
// columns across every row for one KB. Uses LENGTH() which works in
// PG, MySQL, and SQLite identically for a TEXT column.
//
// Build #23 — this is a single full-table scan within the (kb_id)
// subset. For a KB with 10k cached pages the scan reads five TEXT
// columns × 10k rows = ~50k LENGTH() calls. At ~5µs each on a
// vanilla PG that's ~250ms — too slow for the per-request admin
// endpoint. The handler calls this only on the admin list endpoint
// (not the per-page endpoint), and we expect admins to call it
// infrequently. If a KB grows past ~50k cached pages we should add a
// per-KB counter table populated at write time, but that's a future
// Build — out of scope for Build #23.
func (r *wikiBacklinksCacheRepository) SumPayloadSizeByKB(
	ctx context.Context, kbID string,
) (int64, error) {
	if kbID == "" {
		return 0, nil
	}
	var total int64
	err := r.db.WithContext(ctx).
		Model(&types.WikiBacklinksCacheRow{}).
		Select("COALESCE(SUM(LENGTH(direct_json) + LENGTH(indirect_json) + LENGTH(related_json) + LENGTH(broken_json) + LENGTH(stats_json)), 0)").
		Where("kb_id = ?", kbID).
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}