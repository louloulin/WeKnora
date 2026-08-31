package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/scimsp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SCIM errors surfaced to the handler.
var (
	ErrSCIMUserNotFound       = errors.New("scim: user not found")
	ErrSCIMUserAlreadyExists = errors.New("scim: user already exists")
)

// SCIM errors mapped from the auth hot path. The handler translates
// each into a 401 response with the matching scimType.
var (
	ErrSCIMTokenInvalid     = errors.New("scim: token invalid")
	ErrSCIMTokenUnauthorized = errors.New("scim: token missing or malformed")
)

// SCIMTokenService manages the per-tenant bearer tokens used to
// authenticate SCIM clients. The plaintext token never reaches
// storage: CreateToken returns it once, then we keep only the
// SHA-256 hash.
type SCIMTokenService struct {
	repo interfaces.SCIMTokenRepository
}

// NewSCIMTokenService constructs the service.
func NewSCIMTokenService(repo interfaces.SCIMTokenRepository) *SCIMTokenService {
	return &SCIMTokenService{repo: repo}
}

// CreateToken mints a fresh bearer token. The plaintext is returned
// exactly once — the operator must store it client-side because we
// only persist the hash.
func (s *SCIMTokenService) CreateToken(ctx context.Context, tenantID uint64, createdBy string, req *types.SCIMTokenCreateRequest) (*types.SCIMTokenCreateResponse, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("scim token: name is required")
	}
	if tenantID == 0 {
		return nil, errors.New("scim token: tenantID is required")
	}
	plaintext, err := generateSCIMToken()
	if err != nil {
		return nil, err
	}
	hash := hashSCIMToken(plaintext)
	prefix := plaintext[:8]
	row := &types.SCIMToken{
		TenantID:    tenantID,
		Name:        req.Name,
		TokenHash:   hash,
		TokenPrefix: prefix,
		CreatedBy:   createdBy,
		ExpiresAt:   req.ExpiresAt,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	return &types.SCIMTokenCreateResponse{
		ID:          row.ID,
		Name:        row.Name,
		Token:       plaintext,
		TokenPrefix: prefix,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// ListTokens returns every token for the tenant (metadata only;
// the plaintext is never returned).
func (s *SCIMTokenService) ListTokens(ctx context.Context, tenantID uint64) ([]*types.SCIMToken, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

// RevokeToken soft-deletes the token by primary key.
func (s *SCIMTokenService) RevokeToken(ctx context.Context, id uint64) error {
	return s.repo.Revoke(ctx, id)
}

// Authenticate validates the bearer token (as presented in the
// Authorization header) and returns the matching tenant. The
// returned tenant ID scopes every downstream operation.
func (s *SCIMTokenService) Authenticate(ctx context.Context, bearer string) (uint64, error) {
	id, _, err := s.AuthenticateWithTokenID(ctx, bearer)
	return id, err
}

// AuthenticateWithTokenID returns both the tenant id and the
// authenticated token id so the middleware can attribute the
// request to a specific credential in the sync log.
func (s *SCIMTokenService) AuthenticateWithTokenID(ctx context.Context, bearer string) (uint64, uint64, error) {
	plaintext, err := extractBearer(bearer)
	if err != nil {
		return 0, 0, err
	}
	hash := hashSCIMToken(plaintext)
	row, err := s.repo.GetByTokenHash(ctx, hash)
	if err != nil {
		return 0, 0, err
	}
	_ = s.repo.Touch(ctx, row.ID)
	return row.TenantID, row.ID, nil
}

// generateSCIMToken returns a 32-byte URL-safe token. Cryptographic
// randomness — never use math/rand here.
func generateSCIMToken() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, 32)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return "scim_" + string(out), nil
}

// hashSCIMToken returns the hex SHA-256 of the plaintext token.
func hashSCIMToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// extractBearer pulls the token out of an Authorization header
// value. Accepts both "Bearer <tok>" (RFC 6750) and the bare
// "<tok>" form some IdPs use in their SCIM clients.
func extractBearer(header string) (string, error) {
	h := strings.TrimSpace(header)
	if h == "" {
		return "", ErrSCIMTokenUnauthorized
	}
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		h = strings.TrimSpace(h[len("bearer "):])
	}
	if h == "" {
		return "", ErrSCIMTokenUnauthorized
	}
	return h, nil
}

// SCIMSyncLogService wraps the write-only sync log repository. Kept
// separate from SCIMTokenService so the auth hot path does not pull
// in the audit-writer dependency.
type SCIMSyncLogService struct {
	repo interfaces.SCIMSyncLogRepository
}

// NewSCIMSyncLogService constructs the service.
func NewSCIMSyncLogService(repo interfaces.SCIMSyncLogRepository) *SCIMSyncLogService {
	return &SCIMSyncLogService{repo: repo}
}

// Record writes one sync log entry. Safe to call from middleware on
// the response path.
func (s *SCIMSyncLogService) Record(ctx context.Context, entry *types.SCIMSyncLog) error {
	return s.repo.Create(ctx, entry)
}

// List returns the most recent entries for the tenant.
func (s *SCIMSyncLogService) List(ctx context.Context, tenantID uint64, limit int) ([]*types.SCIMSyncLog, error) {
	return s.repo.ListByTenant(ctx, tenantID, limit)
}

// SCIMUserService exposes the SCIM User CRUD operations on top of
// the existing userService. We bridge the SCIM wire model to the
// local types.User by reusing the same JIT/lookup helpers.
type SCIMUserService struct {
	users   *userService
	logSvc  *SCIMSyncLogService
}

// NewSCIMUserService constructs the service.
func NewSCIMUserService(users *userService, logSvc *SCIMSyncLogService) *SCIMUserService {
	return &SCIMUserService{users: users, logSvc: logSvc}
}

// ListUsers returns the local users that match an optional SCIM
// filter. The total count is returned alongside the slice so the
// handler can populate ListResponse.TotalResults. An empty filter
// returns every user in the tenant.
func (s *SCIMUserService) ListUsers(ctx context.Context, tenantID uint64, filterExpr string) ([]*types.User, int, error) {
	users, total, err := s.users.userRepo.ListUsersByTenant(ctx, tenantID, 0, 1000)
	if err != nil {
		return nil, 0, err
	}
	if filterExpr == "" {
		return users, total, nil
	}
	f, err := scimsp.ParseFilter(filterExpr)
	if err != nil {
		return nil, 0, err
	}
	matched := make([]*types.User, 0, len(users))
	for _, u := range users {
		if s.matchUser(u, f) {
			matched = append(matched, u)
		}
	}
	return matched, len(matched), nil
}

// matchUser evaluates the parsed filter against a user row.
func (s *SCIMUserService) matchUser(u *types.User, f *scimsp.Filter) bool {
	if u == nil {
		return false
	}
	doc := map[string]string{
		"username":   u.Username,
		"id":         u.ID,
		"email":      u.Email,
		"active":     boolStr(u.IsActive),
		"externalid": "",
	}
	get := func(attr string) (string, bool) {
		v, ok := doc[attr]
		return v, ok
	}
	return f.Match(get)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// GetUser returns a single user by id within the tenant scope.
func (s *SCIMUserService) GetUser(ctx context.Context, tenantID uint64, id string) (*types.User, error) {
	u, err := s.users.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.TenantID != tenantID {
		return nil, ErrSCIMUserNotFound
	}
	return u, nil
}

// UpsertUser creates or updates a local user from a SCIM request.
// Matches by userName; on conflict returns ErrSCIMUserAlreadyExists
// unless the existing row is already bound to the tenant (we then
// update in place).
func (s *SCIMUserService) UpsertUser(ctx context.Context, tenantID uint64, u *scimsp.User, email string) (*types.User, error) {
	if u == nil {
		return nil, ErrSCIMUserNotFound
	}
	if u.UserName == "" || email == "" {
		return nil, ErrSCIMUserNotFound
	}
	existing, err := s.users.userRepo.GetUserByEmail(ctx, email)
	if err == nil && existing != nil && existing.TenantID != tenantID {
		return nil, ErrSCIMUserAlreadyExists
	}
	if existing != nil {
		existing.Username = u.UserName
		existing.Email = email
		existing.IsActive = u.Active
		if err := s.users.userRepo.UpdateUser(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	return s.users.jitProvisionUserFromExternal(ctx, tenantID, email, u.UserName, "")
}

// DeleteUser soft-deletes the user by id within the tenant scope.
func (s *SCIMUserService) DeleteUser(ctx context.Context, tenantID uint64, id string) error {
	u, err := s.GetUser(ctx, tenantID, id)
	if err != nil {
		return err
	}
	return s.users.userRepo.DeleteUser(ctx, u.ID)
}

// PatchUser applies a SCIM PatchOp list to the user. We support
// replace on active / emails.value / userName.
func (s *SCIMUserService) PatchUser(ctx context.Context, tenantID uint64, id string, ops []scimsp.PatchOp) error {
	u, err := s.GetUser(ctx, tenantID, id)
	if err != nil {
		return err
	}
	for _, op := range ops {
		switch op.Op {
		case "replace", "add":
			switch op.Path {
			case "active":
				if b, ok := op.Value.(bool); ok {
					u.IsActive = b
				}
			case "userName":
				if s, ok := op.Value.(string); ok {
					u.Username = s
				}
			case "emails", "emails.value":
				// op.Value is either an array or a single object.
				if arr, ok := op.Value.([]any); ok && len(arr) > 0 {
					if m, ok := arr[0].(map[string]any); ok {
						if v, ok := m["value"].(string); ok {
							u.Email = v
						}
					}
				} else if m, ok := op.Value.(map[string]any); ok {
					if v, ok := m["value"].(string); ok {
						u.Email = v
					}
				}
			default:
				// path on a sub-attribute e.g. name.givenName: treat
				// as unsupported rather than erroring — IdPs will
				// re-PATCH until the call succeeds.
			}
		case "remove":
			// SCIM remove on a single value resets it. Today we
			// only honour remove on active=false (deactivate).
			if op.Path == "active" {
				u.IsActive = false
			}
		}
	}
	u.UpdatedAt = time.Now()
	return s.users.userRepo.UpdateUser(ctx, u)
}

// guard: keep import used regardless of future refactors.
var _ = scimsp.SchemaUser

// BuildServiceProviderConfig returns the discovery document. We
// advertise Patch + Filter + ETag, no Bulk (RFC 7644 §3.7
// recommends honouring client bulk requests only when needed) and a
// single Bearer authentication scheme.
func (s *SCIMUserService) BuildServiceProviderConfig() *scimsp.ServiceProviderConfig {
	return &scimsp.ServiceProviderConfig{
		Schemas:          []string{scimsp.SchemaServiceProviderCfg},
		DocumentationURI: "https://example.com/docs/scim",
		Patch:            scimsp.FeatureSupport{Supported: true},
		Bulk:             scimsp.BulkSupport{Supported: false},
		Filter:           scimsp.FeatureSupport{Supported: true},
		ETag:             scimsp.FeatureSupport{Supported: true},
		SortSupported:    false,
		AuthenticationSchemes: []scimsp.AuthenticationScheme{
			{
				Type:             "httpbearer",
				Name:             "SCIM Bearer Token",
				SpecURI:          "https://datatracker.ietf.org/doc/html/rfc6750",
				DocumentationURI: "https://example.com/docs/scim/auth",
			},
		},
	}
}

// ToWire maps a local types.User onto the SCIM 2.0 wire shape. The
// returned user is immutable — callers must copy if they need to
// mutate.
func (s *SCIMUserService) ToWire(u *types.User, location string) *scimsp.User {
	if u == nil {
		return nil
	}
	name := displayName(u)
	wire := &scimsp.User{
		Schemas:     []string{scimsp.SchemaUser},
		ID:          u.ID,
		UserName:    u.Username,
		DisplayName: name,
		Active:      u.IsActive,
		Emails: []scimsp.Email{{
			Value:   u.Email,
			Type:    "work",
			Primary: true,
		}},
		Meta: &scimsp.Meta{
			ResourceType: "User",
			Location:     location,
			Created:      u.CreatedAt.UTC().Format(time.RFC3339),
			LastModified: u.UpdatedAt.UTC().Format(time.RFC3339),
			Version:      fmt.Sprintf("W/\"%d\"", u.UpdatedAt.Unix()),
		},
	}
	if name != "" {
		wire.Name = &scimsp.Name{
			Formatted:  name,
			GivenName:  strings.SplitN(name, " ", 2)[0],
			FamilyName: lastName(name),
		}
	}
	return wire
}

// displayName picks the best-effort display name for a local
// user — username today; a populated profile.FullName when
// the profile model lands.
func displayName(u *types.User) string {
	if u == nil {
		return ""
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Email
}

// lastName extracts the family name from a formatted display name.
func lastName(formatted string) string {
	parts := strings.Fields(formatted)
	if len(parts) <= 1 {
		return ""
	}
	return parts[len(parts)-1]
}

// guard so the import of repository is retained when future
// refactors prune references.
var _ = repository.NewSCIMTokenRepository
