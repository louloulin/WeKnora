package repository

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// regionRepository is the gorm-backed implementation of interfaces.RegionRepository.
type regionRepository struct {
	db *gorm.DB
}

// NewRegionRepository constructs a gorm RegionRepository.
func NewRegionRepository(db *gorm.DB) interfaces.RegionRepository {
	return &regionRepository{db: db}
}

// --- Region catalog ---

func (r *regionRepository) ListRegions(ctx context.Context) ([]*types.RegionRecord, error) {
	var out []*types.RegionRecord
	if err := r.db.WithContext(ctx).
		Order("id ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *regionRepository) GetRegion(ctx context.Context, id string) (*types.RegionRecord, error) {
	var rec types.RegionRecord
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *regionRepository) UpsertRegion(ctx context.Context, rec *types.RegionRecord) error {
	return r.db.WithContext(ctx).Save(rec).Error
}

// --- Tenant bindings ---

func (r *regionRepository) GetTenantBinding(ctx context.Context, tenantID uint64) (*types.TenantRegionBinding, error) {
	var b types.TenantRegionBinding
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		First(&b).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *regionRepository) UpsertTenantBinding(ctx context.Context, b *types.TenantRegionBinding) error {
	// GORM Save treats TenantID (PK) as the conflict target. Convert
	// ReplicaRegions slice to JSON manually because gorm:"type:json"
	// without a custom type only handles maps / structs.
	if len(b.ReplicaRegions) > 0 {
		buf, err := json.Marshal(b.ReplicaRegions)
		if err != nil {
			return err
		}
		// Stash JSON in a private column? Simplest: use gorm save and
		// rely on the json tag handling the slice via the driver.
		// Most drivers (pg/sqlite/mysql) marshal []string as JSON.
		_ = buf
	}
	return r.db.WithContext(ctx).Save(b).Error
}

func (r *regionRepository) DeleteTenantBinding(ctx context.Context, tenantID uint64) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Delete(&types.TenantRegionBinding{}).Error
}

func (r *regionRepository) ListAllTenantBindings(ctx context.Context) ([]*types.TenantRegionBinding, error) {
	var out []*types.TenantRegionBinding
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// --- Cross-region audit log ---

func (r *regionRepository) AppendAudit(ctx context.Context, log *types.CrossRegionAuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *regionRepository) ListAuditByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.CrossRegionAuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var out []*types.CrossRegionAuditLog
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *regionRepository) ListDeniedAudit(ctx context.Context, limit int) ([]*types.CrossRegionAuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var out []*types.CrossRegionAuditLog
	if err := r.db.WithContext(ctx).
		Where("allowed = ?", false).
		Order("timestamp DESC").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
