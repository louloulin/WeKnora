package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/samlsp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SAMLLogin errors are returned by LoginWithSAMLAssertion. The
// handler maps each sentinel into a distinct HTTP status so the SAML
// login UI can show a meaningful message instead of a generic
// "something went wrong" toast.
var (
	// ErrSAMLIdentityNotFound is re-exported from the repository so
	// callers can use a single sentinel regardless of layer.
	ErrSAMLIdentityNotFound = apprepo.ErrSAMLIdentityNotFound
	// ErrSAMLIdentityRevoked signals that the binding exists but has
	// been revoked by an admin. Surfaced as 403 in the ACS handler.
	ErrSAMLIdentityRevoked = apprepo.ErrSAMLIdentityRevoked
	// ErrSAMLAssertionMissingEmail is returned when neither the
	// assertion attributes nor the NameID carry a usable email; we
	// refuse to auto-provision a user without one because the local
	// user table requires it (NOT NULL, UNIQUE).
	ErrSAMLAssertionMissingEmail = errors.New("saml: assertion did not contain an email attribute")
	// ErrSAMLIdentityLinkingDisabled is returned when the IdP returned
	// an unknown (IdP, NameID) tuple, the email matched an existing
	// local user, and AllowEmailLinking is false. Mirrors the OIDC
	// behaviour so the admin opt-in is symmetric across protocols.
	ErrSAMLIdentityLinkingDisabled = errors.New("saml: identity linking is disabled; administrator linking is required")
)

// LoginWithSAMLAssertion exchanges a validated SAML assertion for a
// pair of local JWTs. The flow mirrors LoginWithOIDC:
//
//  1. Look up the federation row by (IdPEntityID, NameID).
//  2. If found and not revoked, touch + load the bound user + mint tokens.
//  3. If not found, attempt to link to an existing user by email when
//     cfg.SAMLAuth.AllowEmailLinking is true.
//  4. If no user exists, JIT-provision one with the same default
//     tenant mode the OIDC flow uses.
//  5. Persist a new federation row that links the user to the IdP so
//     subsequent logins skip step 3.
//
// provisioning is the default tenant mode applied only when a brand-new
// local user is auto-created; it is resolved by the caller from the
// shared auth.default_tenant_mode policy so password registration,
// OIDC, and SAML all converge on the same default.
func (s *userService) LoginWithSAMLAssertion(
	ctx context.Context,
	tenantID uint64,
	info types.SAMLIdentityInfo,
	provisioning types.TenantProvisioningMode,
) (*types.LoginResponse, error) {
	if strings.TrimSpace(info.IdPEntityID) == "" {
		return nil, errors.New("saml: IdP entity id is required")
	}
	if strings.TrimSpace(info.NameID) == "" {
		return nil, errors.New("saml: NameID is required")
	}
	if s.samlIdentityRepo == nil {
		return nil, errors.New("saml: identity repository is not configured")
	}
	if s.config == nil || s.config.SAMLAuth == nil {
		return nil, errors.New("saml: SAMLAuth config is not initialised")
	}

	now := time.Now()
	var user *types.User
	var binding *types.SAMLFederationIdentity
	isNewUser := false

	// Step 1+2: try the federation lookup. A revoked row returns a
	// non-nil identity AND ErrSAMLIdentityRevoked so we can short-
	// circuit with the right error message.
	identity, err := s.samlIdentityRepo.GetByIdPAndNameID(ctx, info.IdPEntityID, info.NameID)
	switch {
	case err == nil:
		user, err = s.userRepo.GetUserByID(ctx, identity.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to load SAML-bound user: %w", err)
		}
		if err := s.samlIdentityRepo.Touch(ctx, identity.ID, info.Email, info.DisplayName); err != nil {
			return nil, fmt.Errorf("failed to update SAML identity: %w", err)
		}
		binding = identity
	case errors.Is(err, ErrSAMLIdentityRevoked):
		// Binding exists but is revoked. The handler maps this to 403.
		return nil, ErrSAMLIdentityRevoked
	case errors.Is(err, ErrSAMLIdentityNotFound):
		// Step 3: no binding yet — try to link to an existing user
		// by email when allowed, otherwise JIT-provision.
		email := strings.TrimSpace(info.Email)
		if email == "" {
			// Last-ditch fallback: some IdPs put the email into
			// the NameID when NameIDFormat=emailAddress. Accept
			// that only if it looks like an email.
			if strings.Contains(info.NameID, "@") {
				email = info.NameID
			}
		}
		if email == "" {
			return nil, ErrSAMLAssertionMissingEmail
		}

		if !s.config.SAMLAuth.AllowEmailLinking {
			// Disallow silent linking. If a user already exists
			// with this email we surface a clear error; if no
			// user exists, JIT provisioning is still allowed so
			// the admin opt-in is about the dangerous case
			// (linking to an existing local account).
			if existing, lookupErr := s.userRepo.GetUserByEmail(ctx, email); lookupErr == nil && existing != nil {
				return nil, ErrSAMLIdentityLinkingDisabled
			} else if lookupErr != nil && !isUserLookupNotFound(lookupErr) {
				return nil, fmt.Errorf("failed to query user by email: %w", lookupErr)
			}
		} else {
			existing, lookupErr := s.userRepo.GetUserByEmail(ctx, email)
			if lookupErr != nil && !isUserLookupNotFound(lookupErr) {
				return nil, fmt.Errorf("failed to query user by email: %w", lookupErr)
			}
			if existing != nil {
				user = existing
			}
		}

		if user == nil {
			// Step 4: JIT-provision. We pass a synthetic OIDCUserInfo
			// so we can reuse provisionOIDCUser without duplicating
			// the username-generation / password-randomisation logic.
			synthetic := &types.OIDCUserInfo{
				Subject:  info.NameID,
				Issuer:   info.IdPEntityID,
				Email:    email,
				Username: info.DisplayName,
			}
			user, err = s.provisionOIDCUser(ctx, synthetic, provisioning)
			if err != nil {
				return nil, err
			}
			isNewUser = true
		}

		// Step 5: persist the federation row. Unique index on
		// (idp_entity_id, name_id) protects against concurrent ACS
		// hits racing to create the same binding.
		newBinding := &types.SAMLFederationIdentity{
			UserID:       user.ID,
			TenantID:     tenantID,
			IdPEntityID:  info.IdPEntityID,
			NameID:       info.NameID,
			NameIDFormat: defaultIfEmpty(info.NameIDFormat, "emailAddress"),
			SessionIndex: info.SessionIndex,
			EmailAtLast:  email,
			DisplayName:  info.DisplayName,
			LastLoginAt:  now,
		}
		if err := s.samlIdentityRepo.Create(ctx, newBinding); err != nil {
			return nil, fmt.Errorf("failed to persist SAML identity: %w", err)
		}
		binding = newBinding
	default:
		return nil, fmt.Errorf("failed to query SAML identity: %w", err)
	}

	if !user.IsActive {
		return &types.LoginResponse{Success: false, Message: "Account is disabled"}, nil
	}

	// Step 6: ensure the user has a membership row for the tenant
	// the IdP belongs to. JIT-provisioned users may not have one
	// yet (provisionOIDCUser routes through Register which honours
	// the default tenant mode).
	if err := s.ensureSAMLTenantMembership(ctx, user, tenantID); err != nil {
		logger.Warnf(ctx, "saml login: failed to ensure tenant membership for user %s in tenant %d: %v",
			user.ID, tenantID, err)
	}

	// Step 7: mint tokens. resolveLoginTenantID honours the user's
	// preferred tenant preference (last-active or home); we fall
	// back to the IdP's tenant if the user has no preference yet.
	resolvedTenantID := s.resolveLoginTenantID(ctx, user)
	if resolvedTenantID == 0 {
		resolvedTenantID = tenantID
	}
	accessToken, refreshToken, err := s.generateTokensForSAML(ctx, user, resolvedTenantID, binding)
	if err != nil {
		return nil, fmt.Errorf("failed to generate local tokens: %w", err)
	}

	var tenant *types.Tenant
	if resolvedTenantID > 0 {
		if t, terr := s.tenantService.GetTenantByID(ctx, resolvedTenantID); terr == nil {
			tenant = t
		} else {
			logger.Warnf(ctx, "saml login: failed to load tenant %d for user %s: %v",
				resolvedTenantID, user.ID, terr)
		}
	}
	memberships := s.buildMembershipsForUser(ctx, user, tenant)

	// Log IsNewUser through logger so admins can spot unusual
	// federation activity in the audit trail.
	if isNewUser {
		logger.Infof(ctx, "saml login: JIT-provisioned user %s from IdP %s (NameID=%s)",
			user.ID, info.IdPEntityID, info.NameID)
	}

	return &types.LoginResponse{
		Success:      true,
		Message:      "登录成功",
		User:         user,
		ActiveTenant: tenant,
		Memberships:  memberships,
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// ensureSAMLTenantMembership adds the user to the tenant the IdP is
// configured for when no row exists. Idempotent: when the membership
// already exists the call is a no-op so repeated logins do not
// duplicate the row.
func (s *userService) ensureSAMLTenantMembership(ctx context.Context, user *types.User, tenantID uint64) error {
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
	// Add the user as a member with the default viewer role; admins
	// promote them out-of-band once they trust the IdP. Using viewer
	// is intentionally conservative: the user can read knowledge they
	// explicitly get shared into, but cannot write or admin.
	inviter := user.ID
	_, err = s.memberService.AddMember(ctx, user.ID, tenantID, types.TenantRoleViewer, &inviter)
	return err
}

// generateTokensForSAML is a thin wrapper around generateTokensForTenant
// that maps the SAML federation row onto the same shape OIDC uses.
// Keeping it as a helper means the token-issuing logic stays in one
// place and future protocol additions (LDAP, SCIM) can share it.
func (s *userService) generateTokensForSAML(
	ctx context.Context,
	user *types.User,
	tenantID uint64,
	binding *types.SAMLFederationIdentity,
) (string, string, error) {
	if binding == nil {
		return s.generateTokensForTenant(ctx, user, tenantID)
	}
	// Pull the underlying OIDC identity from generateTokensForTenant
	// to keep the JWT shape uniform across federation protocols.
	// We synthesise an OIDCIdentity with the SAML (IdP, NameID) as the
	// (Issuer, Subject) pair so existing token-validation code does
	// not need a separate SAML branch.
	oidcBinding := &types.OIDCIdentity{
		UserID:           binding.UserID,
		Issuer:           binding.IdPEntityID,
		Subject:          binding.NameID,
		Provider:         "saml",
		EmailAtLastLogin: binding.EmailAtLast,
		LastLoginAt:      binding.LastLoginAt,
	}
	return s.generateTokensForTenant(ctx, user, tenantID, oidcBinding)
}

// isTenantMemberNotFound matches the sentinel that the tenant-member
// repository returns for missing rows without us having to import the
// repository package (which would create a cycle). We mirror the
// gorm.ErrRecordNotFound branch by matching the literal "not found"
// substring on the error message; this is intentionally permissive
// because the tenant-member service never returns another not-found
// variant.
func isTenantMemberNotFound(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "record not found")
}

// Compile-time guard: pull samlsp so future expansion that needs the
// assertion type does not have to touch imports.
var _ = (*samlsp.Assertion)(nil)

// Compile-time guard: ensure SAMLIdentityRepository is wired via
// interfaces — surfaces breakage at compile time if the interface
// drifts.
var _ interfaces.SAMLIdentityRepository = (interfaces.SAMLIdentityRepository)(nil)
