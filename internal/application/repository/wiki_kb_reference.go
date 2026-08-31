package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type wikiKBReferenceRepository struct {
	db *gorm.DB
}

// NewWikiKBReferenceRepository wires the GORM implementation into the
// DI container. Tests can pass an in-memory SQLite handle to exercise
// the full soft-delete + upsert lifecycle without a real DB.
func NewWikiKBReferenceRepository(db *gorm.DB) interfaces.WikiKBReferenceRepository {
	return &wikiKBReferenceRepository{db: db}
}

func (r *wikiKBReferenceRepository) Upsert(ctx context.Context, ref *types.WikiKBReference) error {
	now := time.Now().UTC()
	ref.UpdatedAt = now
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = now
	}
	// Atomic upsert on the unique (wiki_page_id, knowledge_id)
	// constraint. On conflict we refresh only the mutable columns:
	// reference_label (what the author typed) and updated_at (audit).
	// tenant_id and created_at are immutable to preserve the audit trail.
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "wiki_page_id"},
			{Name: "knowledge_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"reference_label": ref.ReferenceLabel,
			"updated_at":      now,
		}),
	}).Create(ref)
	if tx.Error != nil {
		return tx.Error
	}
	return nil
}

func (r *wikiKBReferenceRepository) GetByPair(ctx context.Context, tenantID, wikiPageID, knowledgeID string) (*types.WikiKBReference, error) {
	var row types.WikiKBReference
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND wiki_page_id = ? AND knowledge_id = ?", tenantID, wikiPageID, knowledgeID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, interfaces.ErrWikiKBReferenceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *wikiKBReferenceRepository) ListByWikiPage(ctx context.Context, tenantID, wikiPageID string, limit, offset int) ([]*types.WikiKBReference, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND wiki_page_id = ?", tenantID, wikiPageID).
		Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []*types.WikiKBReference
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *wikiKBReferenceRepository) ListByKnowledge(ctx context.Context, tenantID, knowledgeID string, limit, offset int) ([]*types.WikiKBReference, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_id = ?", tenantID, knowledgeID).
		Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []*types.WikiKBReference
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *wikiKBReferenceRepository) SoftDelete(ctx context.Context, tenantID, wikiPageID, knowledgeID string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&types.WikiKBReference{}).
		Where("tenant_id = ? AND wiki_page_id = ? AND knowledge_id = ? AND deleted_at IS NULL",
			tenantID, wikiPageID, knowledgeID).
		Updates(map[string]interface{}{"deleted_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Either the pair never existed or it was already soft-deleted.
		// Both are fine for the caller — we return ErrNotFound only when
		// the caller asked us to delete a row that was never live, so
		// audit code can distinguish "already gone" from "never was".
		return interfaces.ErrWikiKBReferenceNotFound
	}
	return nil
}
