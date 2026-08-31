package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SAMLIdPService is the per-tenant IdP CRUD surface. The service
// is also responsible for parsing the X509 certificate the admin
// pastes into the request body and surfacing only validated
// configs to the SAML SP layer; invalid certificates are rejected
// with a typed error.
type SAMLIdPService interface {
	// Create persists a new IdP config. Returns
	// ErrSAMLIdPAlreadyExists if the tenant already has one.
	Create(ctx context.Context, tenantID uint64, req types.SAMLIdPConfigCreateRequest) (*types.SAMLIdPConfig, error)
	// Get returns the IdP config for a tenant, or
	// ErrSAMLIdPNotFound if there is none.
	Get(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error)
	// GetEnabled is a fast-path used by the SAML login flow: it
	// skips soft-deleted and disabled rows.
	GetEnabled(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error)
	// Update mutates the IdP config in place.
	Update(ctx context.Context, tenantID uint64, req types.SAMLIdPConfigUpdateRequest) (*types.SAMLIdPConfig, error)
	// Delete soft-deletes the IdP config (keeps the audit trail
	// intact for forensic replay).
	Delete(ctx context.Context, tenantID uint64) error
}

// SAMLIdPRepository is the storage layer. The repository is the
// only place the certificate encryption envelope is applied; the
// service layer above deals in plaintext strings.
type SAMLIdPRepository interface {
	Create(ctx context.Context, cfg *types.SAMLIdPConfig) error
	GetByTenantID(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error)
	Update(ctx context.Context, cfg *types.SAMLIdPConfig) error
	Delete(ctx context.Context, tenantID uint64) error
}
