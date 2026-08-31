package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrLDAPFederationNotFound is returned when a directory lookup
// matches no federation row.
var ErrLDAPFederationNotFound = errors.New("ldap: federation identity not found")

// ErrLDAPFederationRevoked is returned when the binding is in place
// but has been revoked by an admin. The login flow treats this as a
// hard failure, not a silent re-link.
var ErrLDAPFederationRevoked = errors.New("ldap: federation identity is revoked")

// LDAPFederationIdentityRepository is the GORM-backed implementation
// of interfaces.LDAPFederationIdentityRepository.
type LDAPFederationIdentityRepository struct {
	db *gorm.DB
}

// NewLDAPFederationIdentityRepository constructs the repository.
func NewLDAPFederationIdentityRepository(db *gorm.DB) *LDAPFederationIdentityRepository {
	return &LDAPFederationIdentityRepository{db: db}
}

// GetByEntry looks up the (LDAPConfigID, EntryDN) pair.
func (r *LDAPFederationIdentityRepository) GetByEntry(ctx context.Context, ldapConfigID uint64, entryDN string) (*types.LDAPFederationIdentity, error) {
	var out types.LDAPFederationIdentity
	err := r.db.WithContext(ctx).
		Where("ldap_config_id = ? AND entry_dn = ?", ldapConfigID, entryDN).
		First(&out).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLDAPFederationNotFound
		}
		return nil, err
	}
	if out.Revoked {
		return nil, ErrLDAPFederationRevoked
	}
	return &out, nil
}

// ListByUser returns every federation row bound to a local user.
func (r *LDAPFederationIdentityRepository) ListByUser(ctx context.Context, userID string) ([]*types.LDAPFederationIdentity, error) {
	var rows []*types.LDAPFederationIdentity
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Create inserts a new row. The caller fills in TenantID,
// LDAPConfigID, EntryDN, UserID, Username, Email.
func (r *LDAPFederationIdentityRepository) Create(ctx context.Context, fed *types.LDAPFederationIdentity) error {
	if fed == nil {
		return errors.New("ldap federation: nil")
	}
	return r.db.WithContext(ctx).Create(fed).Error
}

// Touch stamps the row's LastLoginAt. Idempotent.
func (r *LDAPFederationIdentityRepository) Touch(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&types.LDAPFederationIdentity{}).
		Where("id = ?", id).
		Update("last_login_at", now).Error
}

// Revoke marks the row revoked without deleting it, so an admin can
// audit who used to be bound.
func (r *LDAPFederationIdentityRepository) Revoke(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).
		Model(&types.LDAPFederationIdentity{}).
		Where("id = ?", id).
		Update("revoked", true).Error
}
