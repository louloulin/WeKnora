package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// kgRepository is the gorm-backed implementation of interfaces.KGRepository.
type kgRepository struct {
	db *gorm.DB
}

// NewKGRepository constructs a gorm KGRepository.
func NewKGRepository(db *gorm.DB) interfaces.KGRepository {
	return &kgRepository{db: db}
}

func (r *kgRepository) CreateSupertag(ctx context.Context, st *types.KGSupertag) error {
	return r.db.WithContext(ctx).Create(st).Error
}

func (r *kgRepository) GetSupertag(ctx context.Context, tenantID uint64, id string) (*types.KGSupertag, error) {
	var st types.KGSupertag
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (r *kgRepository) ListSupertagsByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.KGSupertag, error) {
	var out []*types.KGSupertag
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("created_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *kgRepository) UpdateSupertag(ctx context.Context, st *types.KGSupertag) error {
	return r.db.WithContext(ctx).Save(st).Error
}

func (r *kgRepository) DeleteSupertag(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.KGSupertag{}).Error
}

func (r *kgRepository) CreateEntity(ctx context.Context, e *types.KGEntity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *kgRepository) GetEntity(ctx context.Context, tenantID uint64, id string) (*types.KGEntity, error) {
	var e types.KGEntity
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *kgRepository) FindEntitiesByName(ctx context.Context, tenantID uint64, kbID, name string) ([]*types.KGEntity, error) {
	var out []*types.KGEntity
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ? AND name = ?", tenantID, kbID, name).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *kgRepository) UpdateEntity(ctx context.Context, e *types.KGEntity) error {
	return r.db.WithContext(ctx).Save(e).Error
}

func (r *kgRepository) BumpEntityOccurrence(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&types. KGEntity{}).
		Where("id = ?", id).
		UpdateColumn("occurrence", gorm.Expr("occurrence + 1")).Error
}

func (r *kgRepository) ListEntitiesBySupertag(ctx context.Context, tenantID uint64, supertagID string, limit int) ([]*types.KGEntity, error) {
	var out []*types.KGEntity
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND supertag_id = ?", tenantID, supertagID).
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *kgRepository) CreateRelation(ctx context.Context, rel *types.KGEntityRelation) error {
	return r.db.WithContext(ctx).Create(rel).Error
}

func (r *kgRepository) ListRelationsByEntity(ctx context.Context, tenantID uint64, entityID string, limit int) ([]*types.KGEntityRelation, error) {
	var out []*types.KGEntityRelation
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND (src_entity_id = ? OR dst_entity_id = ?)", tenantID, entityID, entityID).
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *kgRepository) ListRelationsByKB(ctx context.Context, tenantID uint64, kbID string, limit int) ([]*types.KGEntityRelation, error) {
	var out []*types.KGEntityRelation
	if err := r.db.WithContext(ctx).
		Joins("JOIN entities src ON entity_relations.src_entity_id = src.id").
		Where("src.tenant_id = ? AND src.kb_id = ?", tenantID, kbID).
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *kgRepository) CreateKGSupertagCommand(ctx context.Context, cmd *types.KGSupertagCommand) error {
	return r.db.WithContext(ctx).Create(cmd).Error
}

func (r *kgRepository) ListKGSupertagCommands(ctx context.Context, supertagID, event string) ([]*types.KGSupertagCommand, error) {
	var out []*types.KGSupertagCommand
	if err := r.db.WithContext(ctx).
		Where("supertag_id = ? AND event = ?", supertagID, event).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
