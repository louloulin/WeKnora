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