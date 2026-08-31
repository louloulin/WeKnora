package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// marketplaceRepository is the gorm-backed implementation of
// interfaces.MarketplaceRepository.
type marketplaceRepository struct {
	db *gorm.DB
}

// NewMarketplaceRepository constructs a gorm MarketplaceRepository.
func NewMarketplaceRepository(db *gorm.DB) interfaces.MarketplaceRepository {
	return &marketplaceRepository{db: db}
}

// --- Vendors ---

func (r *marketplaceRepository) UpsertVendor(ctx context.Context, v *types.PluginVendor) error {
	if v.ID == 0 {
		return r.db.WithContext(ctx).Create(v).Error
	}
	return r.db.WithContext(ctx).Save(v).Error
}

func (r *marketplaceRepository) GetVendorBySlug(ctx context.Context, slug string) (*types.PluginVendor, error) {
	var v types.PluginVendor
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *marketplaceRepository) GetVendorByPublicKey(ctx context.Context, publicKey string) (*types.PluginVendor, error) {
	var v types.PluginVendor
	if err := r.db.WithContext(ctx).Where("public_key = ?", publicKey).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *marketplaceRepository) ListVendors(ctx context.Context) ([]*types.PluginVendor, error) {
	var out []*types.PluginVendor
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// --- Plugins ---

func (r *marketplaceRepository) UpsertPlugin(ctx context.Context, p *types.PluginRecord) error {
	if p.ID == 0 {
		return r.db.WithContext(ctx).Create(p).Error
	}
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *marketplaceRepository) GetPlugin(ctx context.Context, pluginID, version string) (*types.PluginRecord, error) {
	var p types.PluginRecord
	if err := r.db.WithContext(ctx).
		Where("plugin_id = ? AND version = ?", pluginID, version).
		First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *marketplaceRepository) ListPlugins(ctx context.Context, status types.PluginReviewStatus, limit int) ([]*types.PluginRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := r.db.WithContext(ctx).Order("updated_at DESC").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []*types.PluginRecord
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *marketplaceRepository) ListVersionsByPlugin(ctx context.Context, pluginID string) ([]*types.PluginRecord, error) {
	var out []*types.PluginRecord
	if err := r.db.WithContext(ctx).
		Where("plugin_id = ?", pluginID).
		Order("version DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *marketplaceRepository) UpdatePluginStatus(ctx context.Context, pluginID, version string, status types.PluginReviewStatus, reviewerNote string) error {
	res := r.db.WithContext(ctx).
		Model(&types.PluginRecord{}).
		Where("plugin_id = ? AND version = ?", pluginID, version).
		Updates(map[string]any{
			"status":        status,
			"reviewer_note": reviewerNote,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *marketplaceRepository) IncrementDownloads(ctx context.Context, pluginID, version string) error {
	return r.db.WithContext(ctx).
		Model(&types.PluginRecord{}).
		Where("plugin_id = ? AND version = ?", pluginID, version).
		UpdateColumn("downloads", gorm.Expr("downloads + 1")).Error
}

// --- Tenant installs ---

func (r *marketplaceRepository) UpsertTenantPlugin(ctx context.Context, t *types.TenantPlugin) error {
	if t.ID == 0 {
		return r.db.WithContext(ctx).Create(t).Error
	}
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *marketplaceRepository) DeleteTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) error {
	res := r.db.WithContext(ctx).
		Where("tenant_id = ? AND plugin_id = ?", tenantID, pluginID).
		Delete(&types.TenantPlugin{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *marketplaceRepository) GetTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) (*types.TenantPlugin, error) {
	var t types.TenantPlugin
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND plugin_id = ?", tenantID, pluginID).
		First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *marketplaceRepository) ListTenantPlugins(ctx context.Context, tenantID uint64) ([]*types.TenantPlugin, error) {
	var out []*types.TenantPlugin
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("installed_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// --- Audit log ---

func (r *marketplaceRepository) AppendPluginAudit(ctx context.Context, a *types.PluginAuditLog) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *marketplaceRepository) ListPluginAudit(ctx context.Context, tenantID uint64, limit int) ([]*types.PluginAuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var out []*types.PluginAuditLog
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
