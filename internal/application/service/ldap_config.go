package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/ldapsp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LDAPConfigService owns CRUD on tenant directory configs and
// runs the admin "test connection" probe. It is the LDAP
// counterpart of SAMLIdPService: same responsibilities, different
// protocol.
type LDAPConfigService struct {
	repo   interfaces.LDAPConfigRepository
	dialer ldapsp.Dialer
}

// NewLDAPConfigService constructs the service.
func NewLDAPConfigService(repo interfaces.LDAPConfigRepository, dialer ldapsp.Dialer) *LDAPConfigService {
	if dialer == nil {
		dialer = ldapsp.DefaultDialer{}
	}
	return &LDAPConfigService{repo: repo, dialer: dialer}
}

// Create persists a new config. The tenant-id uniqueness is
// enforced by the database index, so we surface a clear error here
// instead of letting the driver speak.
func (s *LDAPConfigService) Create(ctx context.Context, req *types.LDAPConfigCreateRequest) (*types.LDAPConfig, error) {
	if req == nil {
		return nil, errors.New("ldap config: nil request")
	}
	row := &types.LDAPConfig{
		Name:              req.Name,
		Host:              req.Host,
		Port:              req.Port,
		UseTLS:            req.UseTLS,
		SkipVerify:        req.SkipVerify,
		BindDN:            req.BindDN,
		BindPassword:      req.BindPassword,
		BaseDN:            req.BaseDN,
		UserFilter:        req.UserFilter,
		UsernameAttr:      req.UsernameAttr,
		EmailAttr:         req.EmailAttr,
		DisplayNameAttr:   req.DisplayNameAttr,
		GroupAttr:         req.GroupAttr,
		GroupSearchBaseDN: req.GroupSearchBaseDN,
		GroupFilter:       req.GroupFilter,
		Vendor:            req.Vendor,
		Enabled:           true,
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	// TenantID is taken from the request context — the service
	// does not let clients pick an arbitrary tenant. The handler
	// resolves it from the JWT and calls CreateForTenant.
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, fmt.Errorf("ldap config: create: %w", err)
	}
	return row, nil
}

// GetByTenant returns the unique config for a tenant.
func (s *LDAPConfigService) GetByTenant(ctx context.Context, tenantID uint64) (*types.LDAPConfig, error) {
	return s.repo.GetByTenant(ctx, tenantID)
}

// GetByID returns the config with the given ID.
func (s *LDAPConfigService) GetByID(ctx context.Context, id uint64) (*types.LDAPConfig, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns every config across all tenants.
func (s *LDAPConfigService) List(ctx context.Context) ([]*types.LDAPConfig, error) {
	return s.repo.List(ctx)
}

// Update applies a partial patch.
func (s *LDAPConfigService) Update(ctx context.Context, id uint64, patch *types.LDAPConfigUpdateRequest) (*types.LDAPConfig, error) {
	return s.repo.Update(ctx, id, patch)
}

// Delete removes the config. Any active federation rows continue
// to reference the deleted config_id but the login flow will refuse
// to bind because it cannot load the config — exactly the same
// behaviour as deleting an IdP.
func (s *LDAPConfigService) Delete(ctx context.Context, id uint64) error {
	return s.repo.Delete(ctx, id)
}

// TestConnection opens a bind as the service account and returns
// nil on success or a wrapped error explaining the failure. Used
// by the admin UI's "Test" button.
func (s *LDAPConfigService) TestConnection(ctx context.Context, id uint64) error {
	cfg, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.testConnectionWithConfig(ctx, cfg)
}

// TestConnectionForTenant is the same as TestConnection but keyed
// by tenant — convenience for the create flow ("save and test in
// one click").
func (s *LDAPConfigService) TestConnectionForTenant(ctx context.Context, tenantID uint64) error {
	cfg, err := s.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	return s.testConnectionWithConfig(ctx, cfg)
}

// testConnectionWithConfig does the actual probe.
func (s *LDAPConfigService) testConnectionWithConfig(ctx context.Context, cfg *types.LDAPConfig) error {
	conn, err := s.dialer.Dial(toLDAPSPConfig(cfg))
	if err != nil {
		return fmt.Errorf("ldap test: dial: %w", err)
	}
	defer conn.Close()
	return nil
}

// toLDAPSPConfig bridges the persistence model to the low-level
// ldapsp.LDAPConfig used by the dialer.
func toLDAPSPConfig(in *types.LDAPConfig) *ldapsp.LDAPConfig {
	if in == nil {
		return nil
	}
	return &ldapsp.LDAPConfig{
		ID:                in.ID,
		TenantID:          in.TenantID,
		Name:              in.Name,
		Host:              in.Host,
		Port:              in.Port,
		UseTLS:            in.UseTLS,
		SkipVerify:        in.SkipVerify,
		BindDN:            in.BindDN,
		BindPassword:      in.BindPassword,
		BaseDN:            in.BaseDN,
		UserFilter:        in.UserFilter,
		UsernameAttr:      in.UsernameAttr,
		EmailAttr:         in.EmailAttr,
		DisplayNameAttr:   in.DisplayNameAttr,
		GroupAttr:         in.GroupAttr,
		GroupSearchBaseDN: in.GroupSearchBaseDN,
		GroupFilter:       in.GroupFilter,
		Vendor:            in.Vendor,
		Enabled:           in.Enabled,
	}
}

// Ensure repo errors import path is referenced so a future
// refactor of the repository package does not silently drop the
// symbol the service depends on.
var _ = repository.ErrLDAPConfigNotFound
