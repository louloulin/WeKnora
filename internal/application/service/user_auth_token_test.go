package service

import (
	"strings"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func init() {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-user-auth-token-tests")
}

type stubAuthTokenRepo struct {
	tokens         map[string]*types.AuthToken
	revokedUserIDs []string
	createdTokens  []*types.AuthToken
}

type stubOIDCIdentityRepoForAuth struct {
	identity *types.OIDCIdentity
	err      error
}

func (s *stubOIDCIdentityRepoForAuth) GetByIssuerSubject(context.Context, string, string) (*types.OIDCIdentity, error) {
	return s.identity, s.err
}
func (s *stubOIDCIdentityRepoForAuth) Create(context.Context, *types.OIDCIdentity) error { return nil }
func (s *stubOIDCIdentityRepoForAuth) Touch(context.Context, string, string) error       { return nil }

func (s *stubAuthTokenRepo) CreateToken(_ context.Context, token *types.AuthToken) error {
	s.createdTokens = append(s.createdTokens, token)
	return nil
}
func (s *stubAuthTokenRepo) GetTokenByValue(_ context.Context, tokenValue string) (*types.AuthToken, error) {
	token, ok := s.tokens[tokenValue]
	if !ok {
		return nil, errors.New("token not found")
	}
	return token, nil
}
func (s *stubAuthTokenRepo) GetTokensByUserID(context.Context, string) ([]*types.AuthToken, error) {
	return nil, nil
}
func (s *stubAuthTokenRepo) UpdateToken(context.Context, *types.AuthToken) error { return nil }
func (s *stubAuthTokenRepo) DeleteToken(context.Context, string) error           { return nil }
func (s *stubAuthTokenRepo) DeleteExpiredTokens(context.Context) error           { return nil }
func (s *stubAuthTokenRepo) RevokeTokensByUserID(_ context.Context, userID string) error {
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}

type stubUserRepoForAuth struct {
	users       map[string]*types.User
	updateCalls int
}

func (s *stubUserRepoForAuth) CreateUser(context.Context, *types.User) error { return nil }
func (s *stubUserRepoForAuth) GetUserByID(_ context.Context, id string) (*types.User, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}
func (s *stubUserRepoForAuth) GetUsersByIDs(context.Context, []string) (map[string]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByEmail(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByUsername(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByTenantID(context.Context, uint64) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) UpdateUser(context.Context, *types.User) error {
	s.updateCalls++
	return nil
}
func (s *stubUserRepoForAuth) DeleteUser(context.Context, string) error { return nil }
func (s *stubUserRepoForAuth) ListUsers(context.Context, int, int) ([]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) ListSystemAdmins(context.Context, int, int) ([]*types.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepoForAuth) RevokeSystemAdmin(context.Context, string, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) SearchUsers(context.Context, string, int) ([]*types.User, error) {
	return nil, nil
}

func newAuthTestUserService(tokenRepo *stubAuthTokenRepo) *userService {
	return &userService{
		userRepo: &stubUserRepoForAuth{
			users: map[string]*types.User{
				"user-1": {ID: "user-1", TenantID: 1, IsActive: true},
			},
		},
		tokenRepo: tokenRepo,
	}
}

func signTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(getJwtSecret()))
	if err != nil {
		panic(err)
	}
	return signed
}

func TestValidateTokenRejectsRefreshToken(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "refresh_token",
	}

	_, _, err := svc.ValidateToken(ctx, refreshJWT)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(refresh JWT) err = %v, want refresh rejection", err)
	}

	legacyRefresh := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[legacyRefresh] = &types.AuthToken{
		UserID:    "user-1",
		Token:     legacyRefresh,
		TokenType: "refresh_token",
	}

	_, _, err = svc.ValidateToken(ctx, legacyRefresh)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(legacy refresh in DB) err = %v, want refresh rejection", err)
	}
}

func TestValidateTokenRejectsDisabledUser(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	userRepo := svc.userRepo.(*stubUserRepoForAuth)
	userRepo.users["user-1"].IsActive = false
	token := signTestJWT(jwt.MapClaims{"user_id": "user-1", "type": "access", "exp": time.Now().Add(time.Hour).Unix()})
	tokenRepo.tokens[token] = &types.AuthToken{UserID: "user-1", Token: token, TokenType: "access_token"}
	if _, _, err := svc.ValidateToken(ctx, token); err == nil || err.Error() != "user account is disabled" {
		t.Fatalf("ValidateToken() err = %v, want disabled-user rejection", err)
	}
}

func TestValidateTokenRejectsRevokedOIDCIdentity(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.oidcIdentityRepo = &stubOIDCIdentityRepoForAuth{identity: &types.OIDCIdentity{UserID: "user-1", Issuer: "http://issuer", Subject: "subject", RevokedAt: func() *time.Time { value := time.Now(); return &value }()}}
	token := signTestJWT(jwt.MapClaims{"user_id": "user-1", "type": "access", "oidc_issuer": "http://issuer", "oidc_subject": "subject", "exp": time.Now().Add(time.Hour).Unix()})
	tokenRepo.tokens[token] = &types.AuthToken{UserID: "user-1", Token: token, TokenType: "access_token"}
	if _, _, err := svc.ValidateToken(ctx, token); err == nil || err.Error() != "OIDC identity is no longer valid" {
		t.Fatalf("ValidateToken() err = %v, want revoked-identity rejection", err)
	}
}

func TestValidateTokenRejectsExchangeWithoutActiveTenantMembership(t *testing.T) {
	ctx := context.Background()
	identityRepo := &stubOIDCIdentityRepoForAuth{identity: &types.OIDCIdentity{
		UserID: "user-1", Issuer: "http://issuer", Subject: "subject",
	}}
	memberService := &membershipLookupService{byTenant: map[uint64]*types.TenantMember{}}
	svc := newAuthTestUserService(&stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}})
	svc.oidcIdentityRepo = identityRepo
	svc.memberService = memberService
	svc.config = &config.Config{OIDCAuth: &config.OIDCAuthConfig{
		GatewayExchangeSecret:   "exchange-secret-for-tests",
		GatewayExchangeIssuer:   "http://gateway",
		GatewayExchangeAudience: "weknora",
		GatewayTenantMap:        map[string]uint64{"tenant-a": 1},
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":                "http://gateway",
		"aud":                "weknora",
		"sub":                "subject",
		"oidc_issuer":        "http://issuer",
		"oidc_subject":       "subject",
		"casdoor_tenant":     "tenant-a",
		"tenant_id":          1,
		"token_type":           "weknora_exchange",
		"session_id":           "session-1",
		"membership_version":   "version-1",
		"authorization_version": 1,
		"jti":                  "jti-1",
		"iat":                  time.Now().Unix(),
		"exp":                  time.Now().Add(time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("exchange-secret-for-tests"))
	if err != nil {
		t.Fatalf("sign exchange token: %v", err)
	}
	if _, _, err := svc.ValidateToken(ctx, signed); err == nil || err.Error() != "gateway exchange membership is no longer active" {
		t.Fatalf("ValidateToken() err = %v, want inactive-membership rejection", err)
	}

	memberService.byTenant[1] = &types.TenantMember{UserID: "user-1", TenantID: 1, Status: types.TenantMemberStatusSuspended}
	if _, _, err := svc.ValidateToken(ctx, signed); err == nil || err.Error() != "gateway exchange membership is no longer active" {
		t.Fatalf("ValidateToken() err = %v, want suspended-membership rejection", err)
	}

	memberService.byTenant[1].Status = types.TenantMemberStatusActive
	introspection := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+signed {
			t.Errorf("introspection Authorization header was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"active":true,"subject":"subject","casdoor_tenant":"tenant-a","tenant_id":1,"session_id":"session-1","membership_version":"version-1","authorization_version":1,"jti":"jti-1"}}`))
	}))
	defer introspection.Close()
	svc.config.OIDCAuth.GatewayExchangeIntrospectionURL = introspection.URL
	if user, tenantID, err := svc.ValidateToken(ctx, signed); err != nil || user.ID != "user-1" || tenantID != 1 {
		t.Fatalf("ValidateToken() = user=%v tenant=%d err=%v, want active membership", user, tenantID, err)
	}

	introspection.Close()
	failedIntrospection := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	}))
	defer failedIntrospection.Close()
	svc.config.OIDCAuth.GatewayExchangeIntrospectionURL = failedIntrospection.URL
	if _, _, err := svc.ValidateToken(ctx, signed); err == nil || err.Error() != "gateway exchange introspection HTTP 503: gateway returned non-2xx" {
		t.Fatalf("ValidateToken() err = %v, want fail-closed introspection rejection with HTTP 503", err)
	}
}

func TestGatewayExchangeTokenRejectsStaleAuthorizationVersion(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.oidcIdentityRepo = &stubOIDCIdentityRepoForAuth{identity: &types.OIDCIdentity{UserID: "user-1", Issuer: "http://issuer", Subject: "subject"}}
	svc.memberService = &membershipLookupService{byTenant: map[uint64]*types.TenantMember{1: {UserID: "user-1", TenantID: 1, Status: types.TenantMemberStatusActive}}}
	svc.config = &config.Config{OIDCAuth: &config.OIDCAuthConfig{
		GatewayExchangeSecret:   "exchange-secret-for-tests",
		GatewayExchangeIssuer:   "http://gateway",
		GatewayExchangeAudience: "weknora",
		GatewayTenantMap:        map[string]uint64{"tenant-a": 1},
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":                  "http://gateway",
		"aud":                  "weknora",
		"sub":                  "subject",
		"oidc_issuer":          "http://issuer",
		"oidc_subject":         "subject",
		"casdoor_tenant":       "tenant-a",
		"tenant_id":            1,
		"token_type":           "weknora_exchange",
		"session_id":           "session-1",
		"membership_version":   "version-1",
		"authorization_version": 1,
		"jti":                  "jti-1",
		"iat":                  time.Now().Unix(),
		"exp":                  time.Now().Add(time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("exchange-secret-for-tests"))
	if err != nil {
		t.Fatalf("sign exchange token: %v", err)
	}
	introspection := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Gateway reports authorization_version=2 because a webhook has bumped it
		// since the exchange token was minted.
		_, _ = w.Write([]byte(`{"status":"ok","data":{"active":true,"subject":"subject","casdoor_tenant":"tenant-a","tenant_id":1,"session_id":"session-1","membership_version":"version-1","authorization_version":2,"jti":"jti-1"}}`))
	}))
	defer introspection.Close()
	svc.config.OIDCAuth.GatewayExchangeIntrospectionURL = introspection.URL
	if _, _, err := svc.ValidateToken(ctx, signed); err == nil || err.Error() != "gateway exchange token claim mismatch" {
		t.Fatalf("ValidateToken() err = %v, want stale-authorization-version rejection with claim mismatch", err)
	}
}

func TestGatewayExchangeTokenRejectsIntrospectionNetworkFailure(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.oidcIdentityRepo = &stubOIDCIdentityRepoForAuth{identity: &types.OIDCIdentity{UserID: "user-1", Issuer: "http://issuer", Subject: "subject"}}
	svc.memberService = &membershipLookupService{byTenant: map[uint64]*types.TenantMember{1: {UserID: "user-1", TenantID: 1, Status: types.TenantMemberStatusActive}}}
	svc.config = &config.Config{OIDCAuth: &config.OIDCAuthConfig{
		GatewayExchangeSecret:   "exchange-secret-for-tests",
		GatewayExchangeIssuer:   "http://gateway",
		GatewayExchangeAudience: "weknora",
		GatewayTenantMap:        map[string]uint64{"tenant-a": 1},
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":                  "http://gateway",
		"aud":                  "weknora",
		"sub":                  "subject",
		"oidc_issuer":          "http://issuer",
		"oidc_subject":         "subject",
		"casdoor_tenant":       "tenant-a",
		"tenant_id":            1,
		"token_type":           "weknora_exchange",
		"session_id":           "session-1",
		"membership_version":   "version-1",
		"authorization_version": 1,
		"jti":                  "jti-1",
		"iat":                  time.Now().Unix(),
		"exp":                  time.Now().Add(time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("exchange-secret-for-tests"))
	if err != nil {
		t.Fatalf("sign exchange token: %v", err)
	}
	// Use a URL that points at an unreachable endpoint to simulate Casdoor/Gateway outage.
	svc.config.OIDCAuth.GatewayExchangeIntrospectionURL = "http://127.0.0.1:1/v1/token-exchange/weknora/introspect"
	if _, _, err := svc.ValidateToken(ctx, signed); err == nil || !strings.Contains(err.Error(), "gateway exchange introspection failed") {
		t.Fatalf("ValidateToken() err = %v, want network-failure classification (should mention introspection failed)", err)
	}
}

func TestRefreshTokenRejectsRevokedOIDCIdentity(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.oidcIdentityRepo = &stubOIDCIdentityRepoForAuth{identity: &types.OIDCIdentity{UserID: "user-1", Issuer: "http://issuer", Subject: "subject", RevokedAt: func() *time.Time { value := time.Now(); return &value }()}}
	token := signTestJWT(jwt.MapClaims{"user_id": "user-1", "type": "refresh", "oidc_issuer": "http://issuer", "oidc_subject": "subject", "exp": time.Now().Add(time.Hour).Unix()})
	tokenRepo.tokens[token] = &types.AuthToken{UserID: "user-1", Token: token, TokenType: "refresh_token"}
	if _, _, err := svc.RefreshToken(ctx, token); err == nil || err.Error() != "OIDC identity is no longer valid" {
		t.Fatalf("RefreshToken() err = %v, want revoked-identity rejection", err)
	}
}

func TestRefreshTokenPreservesActiveTenant(t *testing.T) {
	ctx := context.Background()
	refreshToken := signTestJWT(jwt.MapClaims{
		"user_id":   "user-1",
		"tenant_id": float64(7),
		"type":      "refresh",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{
		refreshToken: {UserID: "user-1", Token: refreshToken, TokenType: "refresh_token"},
	}}
	svc := newAuthTestUserService(tokenRepo)

	accessToken, _, err := svc.RefreshToken(ctx, refreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	parsed, err := jwt.Parse(accessToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(getJwtSecret()), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("generated access token is invalid: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("generated access token claims have unexpected type")
	}
	if got := tenantIDFromClaims(claims, 0); got != 7 {
		t.Fatalf("refreshed access token tenant_id = %d, want 7", got)
	}
}

func TestRefreshTokenRejectsAccessTokenRecord(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "access_token",
	}

	_, _, err := svc.RefreshToken(ctx, refreshJWT)
	if err == nil || err.Error() != "not a refresh token" {
		t.Fatalf("RefreshToken(access token record) err = %v, want not a refresh token", err)
	}
}

func TestLogoutRevokesAllUserTokens(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	expiredAccess := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	if err := svc.Logout(ctx, expiredAccess); err != nil {
		t.Fatalf("Logout(expired access token) err = %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("RevokeTokensByUserID calls = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestAdminResetPasswordHashesPasswordAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	if err := svc.AdminResetPassword(ctx, "user-1", "NewSecure9"); err != nil {
		t.Fatalf("AdminResetPassword() err = %v", err)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("UpdateUser calls = %d, want 1", repo.updateCalls)
	}
	user := repo.users["user-1"]
	if user.PasswordHash == "NewSecure9" || user.PasswordHash == "" {
		t.Fatalf("password was not stored as a hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("NewSecure9")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("RevokeTokensByUserID calls = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestAdminResetPasswordRejectsWeakPasswordBeforeWrite(t *testing.T) {
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	err := svc.AdminResetPassword(context.Background(), "user-1", "password")
	if !errors.Is(err, ErrPasswordPolicy) {
		t.Fatalf("AdminResetPassword() err = %v, want ErrPasswordPolicy", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("weak password caused side effects: updates=%d revocations=%v", repo.updateCalls, tokenRepo.revokedUserIDs)
	}
}

func TestChangePasswordRequiresPolicyAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	hashed, err := bcrypt.GenerateFromPassword([]byte("OldSecure9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo.users["user-1"].PasswordHash = string(hashed)

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "weak"); !errors.Is(err, ErrPasswordPolicy) {
		t.Fatalf("ChangePassword(weak) err = %v, want ErrPasswordPolicy", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("weak password caused side effects: updates=%d revocations=%v", repo.updateCalls, tokenRepo.revokedUserIDs)
	}

	if err := svc.ChangePassword(ctx, "user-1", "wrong-pass", "NewSecure9"); !errors.Is(err, ErrInvalidOldPassword) {
		t.Fatalf("ChangePassword(wrong old) err = %v, want ErrInvalidOldPassword", err)
	}

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "NewSecure9"); err != nil {
		t.Fatalf("ChangePassword() err = %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.users["user-1"].PasswordHash), []byte("NewSecure9")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("revoked users = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestChangePasswordRejectsSamePassword(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	hashed, err := bcrypt.GenerateFromPassword([]byte("OldSecure9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo.users["user-1"].PasswordHash = string(hashed)

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "OldSecure9"); !errors.Is(err, ErrSamePassword) {
		t.Fatalf("ChangePassword(same) err = %v, want ErrSamePassword", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("same password caused side effects: updates=%d revocations=%v", repo.updateCalls, tokenRepo.revokedUserIDs)
	}
}

func TestUserIDFromSignedTokenAcceptsExpiredToken(t *testing.T) {
	expired := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	userID, err := userIDFromSignedToken(expired)
	if err != nil {
		t.Fatalf("userIDFromSignedToken(expired) err = %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userIDFromSignedToken(expired) = %q, want user-1", userID)
	}
}

// ListUsersByTenant satisfies the new interfaces.UserRepository method.
// Returns an empty list — these auth-token tests do not exercise
// tenant-scoped user listing.
func (r *stubUserRepoForAuth) ListUsersByTenant(_ context.Context, _ uint64, _, _ int) ([]*types.User, int, error) {
	return nil, 0, nil
}
