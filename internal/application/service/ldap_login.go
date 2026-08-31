package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/ldapsp"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// LDAPLogin errors are returned by LoginWithLDAPCredentials. The
// handler maps each sentinel into a distinct HTTP status so the
// LDAP login UI can show a meaningful message.
var (
	// ErrLDAPFederationNotFound is re-exported so callers can use a
	// single sentinel regardless of layer.
	ErrLDAPFederationNotFound = repository.ErrLDAPFederationNotFound
	// ErrLDAPFederationRevoked signals that the binding exists but
	// has been revoked by an admin. Surfaced as 403 in the login
	// handler so admins can re-bind.
	ErrLDAPFederationRevoked = repository.ErrLDAPFederationRevoked
	// ErrLDAPInvalidCredentials wraps an LDAP result code 49.
	ErrLDAPInvalidCredentials = ldapsp.ErrInvalidCredentials
	// ErrLDAPMissingEmail is returned when the directory entry has
	// no mail attribute — we refuse to JIT provision a user without
	// one because the local user table requires it (NOT NULL).
	ErrLDAPMissingEmail = errors.New("ldap: directory entry did not contain an email attribute")
	// ErrLDAPIdentityLinkingDisabled mirrors the OIDC / SAML
	// behaviour: when an unknown (Config, DN) pair matches an
	// existing local user by email and AllowEmailLinking is false,
	// we refuse to silently link.
	ErrLDAPIdentityLinkingDisabled = errors.New("ldap: identity linking is disabled; administrator linking is required")
	// ErrLDAPEntryNotFound means the search returned zero results;
	// treated as invalid credentials to avoid leaking which
	// usernames exist.
	ErrLDAPEntryNotFound = errors.New("ldap: no directory entry matched the supplied username")
)

// LDAPLoginOptions carries the federation policy applied at login
// time. Kept separate from LDAPConfigService so the policy can be
// tuned without touching per-tenant directory configs.
type LDAPLoginOptions struct {
	AllowEmailLinking bool
	DefaultTenantMode types.TenantProvisioningMode
}

// LDAPLoginDeps is the subset of dependencies the LDAP login flow
// needs beyond what userService already holds. We collect them in a
// single struct so userService stays a thin wrapper and the LDAP
// method has a clear contract. Exported because the container
// wiring (container.Invoke) needs to populate it from outside the
// service package.
type LDAPLoginDeps struct {
	ConfigSvc *LDAPConfigService
	Dialer    ldapsp.Dialer
	FedRepo   *repository.LDAPFederationIdentityRepository
	Opts      LDAPLoginOptions
}

// LDAPLoginOptionsFromDefaults is a constructor that pulls the
// defaults that match the SAML behaviour (AllowEmailLinking false —
// admin opt-in only).
func LDAPLoginOptionsFromDefaults() LDAPLoginOptions {
	return LDAPLoginOptions{
		AllowEmailLinking: false,
		DefaultTenantMode: "",
	}
}

// LoginWithLDAPCredentials authenticates a user against the tenant's
// directory and exchanges the successful bind for a pair of local
// JWTs. The flow mirrors LoginWithOIDC and LoginWithSAMLAssertion:
//
//  1. Load the tenant's LDAPConfig (if any).
//  2. Search the directory by the supplied username.
//  3. Re-bind as the returned DN with the supplied password.
//  4. Look up the federation row by (LDAPConfigID, DN).
//  5. If found and not revoked, touch + load the bound user + mint tokens.
//  6. If not found, attempt to link to an existing user by email when
//     deps.Opts.AllowEmailLinking is true.
//  7. If no user exists, JIT-provision one.
//  8. Persist a new federation row that links the user to the
//     directory so subsequent logins skip step 6.
//
// provisioning is the default tenant mode applied only when a
// brand-new local user is auto-created; it is resolved by the caller
// from the shared auth.default_tenant_mode policy so password
// registration, OIDC, and SAML all converge on the same default.
func (s *userService) LoginWithLDAPCredentials(
	ctx context.Context,
	tenantID uint64,
	username, password string,
	provisioning types.TenantProvisioningMode,
) (*types.LoginResponse, error) {
	if s.ldapLoginDeps == nil || s.ldapLoginDeps.ConfigSvc == nil {
		return nil, errors.New("ldap: service not configured")
	}
	if tenantID == 0 {
		return nil, errors.New("ldap: tenantID is required")
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, errors.New("ldap: username and password are required")
	}
	if provisioning == "" {
		provisioning = s.ldapLoginDeps.Opts.DefaultTenantMode
	}

	deps := s.ldapLoginDeps
	cfg, err := deps.ConfigSvc.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, errors.New("ldap: directory integration is disabled")
	}

	dialer := deps.Dialer
	if dialer == nil {
		dialer = ldapsp.DefaultDialer{}
	}
	conn, err := dialer.Dial(toLDAPSPConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("ldap: dial: %w", err)
	}
	defer conn.Close()

	// Step 2: search for the user DN.
	users, err := ldapsp.SearchUser(conn, toLDAPSPConfig(cfg), username)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrLDAPEntryNotFound
	}
	if len(users) > 1 {
		// Directory mis-config: the filter is too permissive. We
		// refuse to guess which entry the user meant.
		return nil, fmt.Errorf("ldap: search returned %d entries; tighten UserFilter", len(users))
	}
	user := users[0]
	if user.Email == "" {
		return nil, ErrLDAPMissingEmail
	}

	// Step 3: rebind as the user DN. This is the actual auth.
	if err := conn.Bind(user.DN, password); err != nil {
		wrapped := ldapsp.AsInvalidCredentials(err)
		if wrapped != nil {
			return nil, wrapped
		}
		return nil, fmt.Errorf("ldap: bind: %w", err)
	}

	// Step 3.5: optional group resolution. Best-effort; never
	// blocks the login.
	groups, _ := ldapsp.SearchGroups(conn, toLDAPSPConfig(cfg), user.DN)

	info := types.LDAPIdentityInfo{
		LDAPConfigID: cfg.ID,
		EntryDN:      user.DN,
		Username:     username,
		Email:        user.Email,
		DisplayName:  user.DisplayName,
		GroupDNs:     groups,
	}

	return s.completeLDAPLogin(ctx, tenantID, info, provisioning)
}

// completeLDAPLogin resolves the federation row, links or provisions
// as needed, and mints the local token pair.
func (s *userService) completeLDAPLogin(
	ctx context.Context,
	tenantID uint64,
	info types.LDAPIdentityInfo,
	provisioning types.TenantProvisioningMode,
) (*types.LoginResponse, error) {
	isNewUser := false

	deps := s.ldapLoginDeps
	fed, err := deps.FedRepo.GetByEntry(ctx, info.LDAPConfigID, info.EntryDN)
	switch {
	case err == nil:
		_ = deps.FedRepo.Touch(ctx, fed.ID)
	case errors.Is(err, ErrLDAPFederationRevoked):
		return nil, ErrLDAPFederationRevoked
	case errors.Is(err, ErrLDAPFederationNotFound):
		// fall through to link/provision
	default:
		return nil, err
	}

	var localUser *types.User
	if fed != nil {
		localUser, err = s.userRepo.GetUserByID(ctx, fed.UserID)
		if err != nil {
			return nil, fmt.Errorf("ldap: load bound user: %w", err)
		}
	} else {
		// Step 6: link by email.
		existing, lookupErr := s.userRepo.GetUserByEmail(ctx, info.Email)
		switch {
		case lookupErr == nil && existing != nil:
			if !deps.Opts.AllowEmailLinking {
				return nil, ErrLDAPIdentityLinkingDisabled
			}
			localUser = existing
		case isUserLookupNotFound(lookupErr):
			// fall through to JIT
		case lookupErr != nil:
			return nil, fmt.Errorf("ldap: lookup by email: %w", lookupErr)
		}

		if localUser == nil {
			// Step 7: JIT provision via the shared helper. Same
			// path OIDC and SAML use.
			name := info.DisplayName
			if strings.TrimSpace(name) == "" {
				name = info.Username
			}
			created, err := s.jitProvisionUserFromExternal(ctx, tenantID, info.Email, name, provisioning)
			if err != nil {
				return nil, fmt.Errorf("ldap: JIT provision: %w", err)
			}
			localUser = created
			isNewUser = true
		}

		// Step 8: persist federation row.
		if err := deps.FedRepo.Create(ctx, &types.LDAPFederationIdentity{
			TenantID:     tenantID,
			LDAPConfigID: info.LDAPConfigID,
			EntryDN:      info.EntryDN,
			EntryUUID:    info.EntryUUID,
			UserID:       localUser.ID,
			Username:     info.Username,
			Email:        info.Email,
		}); err != nil {
			return nil, fmt.Errorf("ldap: persist federation: %w", err)
		}
	}

	if !localUser.IsActive {
		return &types.LoginResponse{Success: false, Message: "Account is disabled"}, nil
	}

	// Ensure tenant membership; same helper SAML uses.
	if err := s.ensureLDAPTenantMembership(ctx, localUser, tenantID); err != nil {
		logger.Warnf(ctx, "ldap login: failed to ensure tenant membership for user %s in tenant %d: %v",
			localUser.ID, tenantID, err)
	}

	// Mint tokens via the same helper SAML uses.
	resolvedTenantID := s.resolveLoginTenantID(ctx, localUser)
	if resolvedTenantID == 0 {
		resolvedTenantID = tenantID
	}
	accessToken, refreshToken, err := s.generateTokensForLDAP(ctx, localUser, resolvedTenantID)
	if err != nil {
		return nil, fmt.Errorf("ldap: mint tokens: %w", err)
	}

	tenant, _ := s.tenantService.GetTenantByID(ctx, resolvedTenantID)
	memberships := s.buildMembershipsForUser(ctx, localUser, tenant)

	if isNewUser {
		logger.Infof(ctx, "ldap login: JIT-provisioned user %s from directory %d (DN=%s)",
			localUser.ID, info.LDAPConfigID, info.EntryDN)
	}

	return &types.LoginResponse{
		Success:      true,
		Message:      "登录成功",
		User:         localUser,
		ActiveTenant: tenant,
		Memberships:  memberships,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// ensureLDAPTenantMembership mirrors ensureSAMLTenantMembership.
func (s *userService) ensureLDAPTenantMembership(ctx context.Context, user *types.User, tenantID uint64) error {
	if tenantID == 0 || s.memberService == nil {
		return nil
	}
	membership, err := s.memberService.GetMembership(ctx, user.ID, tenantID)
	if err == nil && membership != nil {
		return nil
	}
	if err != nil && !isTenantMemberNotFound(err) {
		return err
	}
	inviter := user.ID
	_, err = s.memberService.AddMember(ctx, user.ID, tenantID, types.TenantRoleViewer, &inviter)
	return err
}

// generateTokensForLDAP mirrors generateTokensForSAML: thin wrapper
// that keeps the token-issuing logic in one place. The SAML helper
// threads the federation row into the JWT claims; for LDAP we add
// the directory-config id and DN so a downstream audit log can
// recover the originating directory entry.
func (s *userService) generateTokensForLDAP(
	ctx context.Context,
	user *types.User,
	tenantID uint64,
) (string, string, error) {
	return s.generateTokensForTenant(ctx, user, tenantID)
}
