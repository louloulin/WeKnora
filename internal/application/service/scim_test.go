package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

// stubSCIMRepo is the in-memory token repository the tests use.
// Same shape as the GORM impl but without a database roundtrip.
type stubSCIMRepo struct {
	byHash map[string]*types.SCIMToken
	byID   map[uint64]*types.SCIMToken
	nextID uint64
}

func newStubSCIMRepo() *stubSCIMRepo {
	return &stubSCIMRepo{
		byHash: map[string]*types.SCIMToken{},
		byID:   map[uint64]*types.SCIMToken{},
	}
}

func (s *stubSCIMRepo) Create(_ context.Context, tok *types.SCIMToken) error {
	s.nextID++
	tok.ID = s.nextID
	if tok.CreatedAt.IsZero() {
		tok.CreatedAt = time.Now()
	}
	s.byHash[tok.TokenHash] = tok
	s.byID[tok.ID] = tok
	return nil
}

func (s *stubSCIMRepo) GetByID(_ context.Context, id uint64) (*types.SCIMToken, error) {
	if t, ok := s.byID[id]; ok {
		return t, nil
	}
	return nil, repository.ErrSCIMTokenNotFound
}

func (s *stubSCIMRepo) GetByTokenHash(_ context.Context, hash string) (*types.SCIMToken, error) {
	t, ok := s.byHash[hash]
	if !ok {
		return nil, repository.ErrSCIMTokenNotFound
	}
	if t.Revoked {
		return nil, repository.ErrSCIMTokenRevoked
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, repository.ErrSCIMTokenExpired
	}
	return t, nil
}

func (s *stubSCIMRepo) ListByTenant(_ context.Context, tenantID uint64) ([]*types.SCIMToken, error) {
	var out []*types.SCIMToken
	for _, t := range s.byID {
		if t.TenantID == tenantID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *stubSCIMRepo) Revoke(_ context.Context, id uint64) error {
	t, ok := s.byID[id]
	if !ok {
		return repository.ErrSCIMTokenNotFound
	}
	t.Revoked = true
	now := time.Now()
	t.RevokedAt = &now
	return nil
}

func (s *stubSCIMRepo) Touch(_ context.Context, id uint64) error {
	t, ok := s.byID[id]
	if !ok {
		return repository.ErrSCIMTokenNotFound
	}
	now := time.Now()
	t.LastUsedAt = &now
	return nil
}

func TestSCIMCreateTokenReturnsPlaintextOnce(t *testing.T) {
	repo := newStubSCIMRepo()
	svc := NewSCIMTokenService(repo)
	resp, err := svc.CreateToken(context.Background(), 1, "admin-1", &types.SCIMTokenCreateRequest{Name: "okta-prod"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.Token == "" {
		t.Fatalf("plaintext token missing")
	}
	if resp.TokenPrefix == "" || len(resp.TokenPrefix) > 16 {
		t.Fatalf("token prefix malformed: %q", resp.TokenPrefix)
	}
	// Hash is persisted; plaintext is not retrievable via List.
	rows, _ := svc.ListTokens(context.Background(), 1)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].TokenHash == "" || rows[0].TokenHash == resp.Token {
		t.Fatalf("hash not persisted correctly")
	}
}

func TestSCIMAuthenticateRoundTrip(t *testing.T) {
	repo := newStubSCIMRepo()
	svc := NewSCIMTokenService(repo)
	resp, err := svc.CreateToken(context.Background(), 7, "admin", &types.SCIMTokenCreateRequest{Name: "azure-ad"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tenant, _, err := svc.AuthenticateWithTokenID(context.Background(), "Bearer "+resp.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if tenant != 7 {
		t.Fatalf("tenant: got %d want 7", tenant)
	}
}

func TestSCIMAuthenticateRejectsUnknown(t *testing.T) {
	svc := NewSCIMTokenService(newStubSCIMRepo())
	_, _, err := svc.AuthenticateWithTokenID(context.Background(), "Bearer scim_doesnotexist")
	if err == nil {
		t.Fatalf("expected error for unknown token")
	}
}

func TestSCIMAuthenticateRejectsRevoked(t *testing.T) {
	repo := newStubSCIMRepo()
	svc := NewSCIMTokenService(repo)
	resp, _ := svc.CreateToken(context.Background(), 1, "admin", &types.SCIMTokenCreateRequest{Name: "x"})
	_ = svc.RevokeToken(context.Background(), 1)
	_ = resp
	// nextID is 1 because we create then revoke; the first row is
	// id=1.
	if err := svc.RevokeToken(context.Background(), 1); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// Authenticate should fail.
	if _, _, err := svc.AuthenticateWithTokenID(context.Background(), "Bearer "+resp.Token); err == nil {
		t.Fatalf("expected error after revoke")
	}
}

func TestSCIMAuthenticateAcceptsBareToken(t *testing.T) {
	repo := newStubSCIMRepo()
	svc := NewSCIMTokenService(repo)
	resp, _ := svc.CreateToken(context.Background(), 1, "admin", &types.SCIMTokenCreateRequest{Name: "x"})
	// Some IdPs send the token without the "Bearer " prefix.
	tenant, _, err := svc.AuthenticateWithTokenID(context.Background(), resp.Token)
	if err != nil {
		t.Fatalf("bare-token auth: %v", err)
	}
	if tenant != 1 {
		t.Fatalf("tenant: got %d want 1", tenant)
	}
}

func TestSCIMAuthenticateRejectsMissing(t *testing.T) {
	svc := NewSCIMTokenService(newStubSCIMRepo())
	if _, _, err := svc.AuthenticateWithTokenID(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty auth header")
	}
}
