package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// samlIdPRepository is the GORM-backed storage for SAML IdP configs.
type samlIdPRepository struct {
	db *gorm.DB
}

// NewSAMLIdPRepository wires the repository.
func NewSAMLIdPRepository(db *gorm.DB) *samlIdPRepository {
	return &samlIdPRepository{db: db}
}

// Create inserts a new IdP config. Returns an error on duplicate
// tenant_id (the unique index fires).
func (r *samlIdPRepository) Create(ctx context.Context, cfg *types.SAMLIdPConfig) error {
	if err := r.db.WithContext(ctx).Create(cfg).Error; err != nil {
		logger.Errorf(ctx, "saml_idp create: %v", err)
		return err
	}
	return nil
}

// GetByTenantID returns the active (non-soft-deleted) IdP config
// for a tenant.
func (r *samlIdPRepository) GetByTenantID(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error) {
	var cfg types.SAMLIdPConfig
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSAMLIdPNotFound
		}
		return nil, err
	}
	return &cfg, nil
}

// Update mutates an existing IdP config.
func (r *samlIdPRepository) Update(ctx context.Context, cfg *types.SAMLIdPConfig) error {
	if err := r.db.WithContext(ctx).Save(cfg).Error; err != nil {
		logger.Errorf(ctx, "saml_idp update: %v", err)
		return err
	}
	return nil
}

// Delete soft-deletes the IdP config (sets deleted_at). The audit
// trail is preserved; an admin can restore via direct SQL.
func (r *samlIdPRepository) Delete(ctx context.Context, tenantID uint64) error {
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Delete(&types.SAMLIdPConfig{}).Error; err != nil {
		logger.Errorf(ctx, "saml_idp delete: %v", err)
		return err
	}
	return nil
}

// ErrSAMLIdPNotFound is the typed not-found sentinel for the
// repository. The service layer maps it to a 404.
var ErrSAMLIdPNotFound = errors.New("saml: idp config not found")
