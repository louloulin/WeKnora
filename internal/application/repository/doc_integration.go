package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// docIntegrationRepository is the gorm-backed implementation of
// interfaces.DocIntegrationRepository.
type docIntegrationRepository struct {
	db *gorm.DB
}

// NewDocIntegrationRepository constructs a gorm DocIntegrationRepository.
func NewDocIntegrationRepository(db *gorm.DB) interfaces.DocIntegrationRepository {
	return &docIntegrationRepository{db: db}
}

// --- Doc ↔ KG relations ---

func (r *docIntegrationRepository) UpsertDocKgRelation(ctx context.Context, rel *types.DocKgRelation) error {
	// Use INSERT ... ON CONFLICT semantics by relying on gorm Save with
	// a unique-key lookup. To keep the schema light we do a SELECT then
	// UpdateOrCreate here.
	var existing types.DocKgRelation
	tx := r.db.WithContext(ctx).
		Where("source_type = ? AND source_id = ? AND target_type = ? AND target_id = ? AND kind = ?",
			rel.SourceType, rel.SourceID, rel.TargetType, rel.TargetID, rel.Kind).
		First(&existing)
	if tx.Error == nil {
		existing.Confidence = rel.Confidence
		existing.Anchor = rel.Anchor
		return r.db.WithContext(ctx).Save(&existing).Error
	}
	if tx.Error != gorm.ErrRecordNotFound {
		return tx.Error
	}
	return r.db.WithContext(ctx).Create(rel).Error
}

func (r *docIntegrationRepository) ListDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) ([]*types.DocKgRelation, error) {
	var out []*types.DocKgRelation
	if err := r.db.WithContext(ctx).
		Where("source_type = ? AND source_id = ?", sourceType, sourceID).
		Order("created_at ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *docIntegrationRepository) ListDocKgRelationsByTarget(ctx context.Context, targetType, targetID string) ([]*types.DocKgRelation, error) {
	var out []*types.DocKgRelation
	if err := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Order("confidence DESC, created_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *docIntegrationRepository) DeleteDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) error {
	return r.db.WithContext(ctx).
		Where("source_type = ? AND source_id = ?", sourceType, sourceID).
		Delete(&types.DocKgRelation{}).Error
}

// --- KB → wiki reverse references ---

func (r *docIntegrationRepository) UpsertKbWikiReference(ctx context.Context, ref *types.KbWikiReference) error {
	var existing types.KbWikiReference
	tx := r.db.WithContext(ctx).
		Where("kb_chunk_id = ? AND wiki_page_id = ?", ref.KBChunkID, ref.WikiPageID).
		First(&existing)
	if tx.Error == nil {
		existing.Anchor = ref.Anchor
		existing.CitationCtx = ref.CitationCtx
		existing.UpdatedAt = ref.UpdatedAt
		return r.db.WithContext(ctx).Save(&existing).Error
	}
	if tx.Error != gorm.ErrRecordNotFound {
		return tx.Error
	}
	return r.db.WithContext(ctx).Create(ref).Error
}

func (r *docIntegrationRepository) ListKbWikiReferencesByChunk(ctx context.Context, kbChunkID string) ([]*types.KbWikiReference, error) {
	var out []*types.KbWikiReference
	if err := r.db.WithContext(ctx).
		Where("kb_chunk_id = ?", kbChunkID).
		Order("updated_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *docIntegrationRepository) ListKbWikiReferencesByPage(ctx context.Context, wikiPageID string) ([]*types.KbWikiReference, error) {
	var out []*types.KbWikiReference
	if err := r.db.WithContext(ctx).
		Where("wiki_page_id = ?", wikiPageID).
		Order("updated_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *docIntegrationRepository) DeleteKbWikiReferencesByPage(ctx context.Context, wikiPageID string) error {
	return r.db.WithContext(ctx).
		Where("wiki_page_id = ?", wikiPageID).
		Delete(&types.KbWikiReference{}).Error
}

// --- Inline KB citations ---

func (r *docIntegrationRepository) UpsertInlineKBRef(ctx context.Context, ref *types.InlineKBRef) error {
	var existing types.InlineKBRef
	tx := r.db.WithContext(ctx).
		Where("wiki_page_id = ? AND kb_chunk_id = ? AND kind = ?",
			ref.WikiPageID, ref.KBChunkID, ref.Kind).
		First(&existing)
	if tx.Error == nil {
		existing.Anchor = ref.Anchor
		existing.Position = ref.Position
		existing.UpdatedAt = ref.UpdatedAt
		return r.db.WithContext(ctx).Save(&existing).Error
	}
	if tx.Error != gorm.ErrRecordNotFound {
		return tx.Error
	}
	return r.db.WithContext(ctx).Create(ref).Error
}

func (r *docIntegrationRepository) ListInlineKBRefsByPage(ctx context.Context, wikiPageID string) ([]*types.InlineKBRef, error) {
	var out []*types.InlineKBRef
	if err := r.db.WithContext(ctx).
		Where("wiki_page_id = ?", wikiPageID).
		Order("position ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *docIntegrationRepository) DeleteInlineKBRefsByPage(ctx context.Context, wikiPageID string) error {
	return r.db.WithContext(ctx).
		Where("wiki_page_id = ?", wikiPageID).
		Delete(&types.InlineKBRef{}).Error
}
