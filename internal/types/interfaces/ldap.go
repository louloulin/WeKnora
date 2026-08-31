package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// LDAPConfigRepository persists per-tenant directory server configs.
type LDAPConfigRepository interface {
	// Create inserts a new config. The implementation is responsible
	// for envelope-encrypting sensitive fields (BindPassword) before
	// they reach storage and decrypting them on read.
	Create(ctx context.Context, cfg *types.LDAPConfig) error
	// GetByID loads a single config. Returns ErrLDAPConfigNotFound
	// when no row matches.
	GetByID(ctx context.Context, id uint64) (*types.LDAPConfig, error)
	// GetByTenant loads the unique config for a tenant (or
	// ErrLDAPConfigNotFound).
	GetByTenant(ctx context.Context, tenantID uint64) (*types.LDAPConfig, error)
	// List returns every active config across all tenants. Used by
	// the admin diagnostics view; deliberately unbounded because the
	// typical installation has at most a handful of directories.
	List(ctx context.Context) ([]*types.LDAPConfig, error)
	// Update applies a partial patch.
	Update(ctx context.Context, id uint64, patch *types.LDAPConfigUpdateRequest) (*types.LDAPConfig, error)
	// Delete removes the row (soft delete via gorm.DeletedAt).
	Delete(ctx context.Context, id uint64) error
}

// LDAPFederationIdentityRepository binds directory entries to users.
type LDAPFederationIdentityRepository interface {
	// GetByEntry looks up the federation row by its directory
	// identity (config + DN). Returns ErrLDAPFederationNotFound
	// when no row matches.
	GetByEntry(ctx context.Context, ldapConfigID uint64, entryDN string) (*types.LDAPFederationIdentity, error)
	// ListByUser returns every directory bound to a single local
	// user so the admin UI can show "Alice is bound to AD + LDAP".
	ListByUser(ctx context.Context, userID string) ([]*types.LDAPFederationIdentity, error)
	// Create inserts a new federation row.
	Create(ctx context.Context, fed *types.LDAPFederationIdentity) error
	// Touch updates LastLoginAt to now.
	Touch(ctx context.Context, id uint64) error
	// Revoke marks the binding as revoked (soft-disable).
	Revoke(ctx context.Context, id uint64) error
}
