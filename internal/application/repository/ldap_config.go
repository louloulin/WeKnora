package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrLDAPConfigNotFound is returned by LDAPConfigRepository when a
// lookup matches no rows. Callers use errors.Is to distinguish
// missing rows from generic driver errors.
var ErrLDAPConfigNotFound = errors.New("ldap: config not found")

// LDAPConfigRepository is the GORM-backed implementation of
// interfaces.LDAPConfigRepository. The bind password is treated as
// opaque — the encryption layer lives in the service so the same
// encryption key surface used for SAML applies to LDAP.
type LDAPConfigRepository struct {
	db *gorm.DB
}

// NewLDAPConfigRepository constructs the repository.
func NewLDAPConfigRepository(db *gorm.DB) *LDAPConfigRepository {
	return &LDAPConfigRepository{db: db}
}

// Create inserts the row, normalising the (TenantID) uniqueness so
// callers don't have to remember the constraint name.
func (r *LDAPConfigRepository) Create(ctx context.Context, cfg *types.LDAPConfig) error {
	if cfg == nil {
		return errors.New("ldap config: nil")
	}
	return r.db.WithContext(ctx).Create(cfg).Error
}

// GetByID returns one row by primary key.
func (r *LDAPConfigRepository) GetByID(ctx context.Context, id uint64) (*types.LDAPConfig, error) {
	var out types.LDAPConfig
	err := r.db.WithContext(ctx).First(&out, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLDAPConfigNotFound
		}
		return nil, err
	}
	return &out, nil
}

// GetByTenant returns the unique config for a tenant.
func (r *LDAPConfigRepository) GetByTenant(ctx context.Context, tenantID uint64) (*types.LDAPConfig, error) {
	var out types.LDAPConfig
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&out).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLDAPConfigNotFound
		}
		return nil, err
	}
	return &out, nil
}

// List returns every non-deleted config.
func (r *LDAPConfigRepository) List(ctx context.Context) ([]*types.LDAPConfig, error) {
	var rows []*types.LDAPConfig
	err := r.db.WithContext(ctx).Order("tenant_id asc, id asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Update applies a non-nil patch field by field. Returns the
// refreshed row so callers can react to defaulting.
func (r *LDAPConfigRepository) Update(ctx context.Context, id uint64, patch *types.LDAPConfigUpdateRequest) (*types.LDAPConfig, error) {
	if patch == nil {
		return nil, errors.New("ldap config: nil patch")
	}
	current, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Host != nil {
		current.Host = *patch.Host
	}
	if patch.Port != nil {
		current.Port = *patch.Port
	}
	if patch.UseTLS != nil {
		current.UseTLS = *patch.UseTLS
	}
	if patch.SkipVerify != nil {
		current.SkipVerify = *patch.SkipVerify
	}
	if patch.BindDN != nil {
		current.BindDN = *patch.BindDN
	}
	if patch.BindPassword != nil {
		current.BindPassword = *patch.BindPassword
	}
	if patch.BaseDN != nil {
		current.BaseDN = *patch.BaseDN
	}
	if patch.UserFilter != nil {
		current.UserFilter = *patch.UserFilter
	}
	if patch.UsernameAttr != nil {
		current.UsernameAttr = *patch.UsernameAttr
	}
	if patch.EmailAttr != nil {
		current.EmailAttr = *patch.EmailAttr
	}
	if patch.DisplayNameAttr != nil {
		current.DisplayNameAttr = *patch.DisplayNameAttr
	}
	if patch.GroupAttr != nil {
		current.GroupAttr = *patch.GroupAttr
	}
	if patch.GroupSearchBaseDN != nil {
		current.GroupSearchBaseDN = *patch.GroupSearchBaseDN
	}
	if patch.GroupFilter != nil {
		current.GroupFilter = *patch.GroupFilter
	}
	if patch.Vendor != nil {
		current.Vendor = *patch.Vendor
	}
	if patch.Enabled != nil {
		current.Enabled = *patch.Enabled
	}
	if err := r.db.WithContext(ctx).Save(current).Error; err != nil {
		return nil, err
	}
	return current, nil
}

// Delete soft-removes the row.
func (r *LDAPConfigRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&types.LDAPConfig{}, id).Error
}
