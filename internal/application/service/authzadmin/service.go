// Package authzadmin implements the v0.7.22 AuthZ Admin UI surface —
// versioning of AuthZ policy expressions + simulator + diff helpers.
//
// The underlying decision engine already lives in internal/authz/ (the
// phase-3 composite checker). v0.7.22 wraps that engine with:
//
//   * Immutable version history (authz_policy_versions)
//   * Latest-version lookup (with cache, mirrors dlp ruleCache)
//   * Diff(v1, v2) — returns a human-readable change summary
//   * Simulate(actor, resource, action) → dry-run decision
//
// Storage pattern mirrors dlp.DLPScanner: raw SQL cross-dialect.
package authzadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// AuthZAdmin errors.
var (
	ErrPolicyKeyRequired = errors.New("authzadmin: policy_key required")
	ErrInvalidDecision   = errors.New("authzadmin: invalid decision")
)

// AuthZAdmin is the public façade.
type AuthZAdmin struct {
	repo  typesRepo
	cache *versionCache
}

// typesRepo is the minimal repo contract AuthZAdmin needs.
type typesRepo interface {
	CreateAuthZPolicyVersion(ctx context.Context, p *types.AuthZPolicyVersion) error
	GetAuthZPolicyVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error)
	GetLatestAuthZPolicy(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error)
	ListAuthZPolicyVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error)
	ListAuthZPolicyKeys(ctx context.Context, tenantID uint64) ([]string, error)
}

// NewAuthZAdmin wires the service to the repo.
func NewAuthZAdmin(repo typesRepo) *AuthZAdmin {
	return &AuthZAdmin{repo: repo, cache: newVersionCache()}
}

// PublishPolicy creates a new version of an AuthZ policy. The previous
// version is NOT touched — all versions are immutable for audit.
// Returns the new version number.
func (a *AuthZAdmin) PublishPolicy(ctx context.Context, tenantID, createdBy uint64,
	policyKey, expression, decision, metadataJSON string,
) (*types.AuthZPolicyVersion, error) {
	if policyKey == "" {
		return nil, ErrPolicyKeyRequired
	}
	if !types.AuthZValidDecision(decision) {
		return nil, ErrInvalidDecision
	}
	latest, err := a.repo.GetLatestAuthZPolicy(ctx, tenantID, policyKey)
	if err != nil {
		return nil, err
	}
	next := int64(1)
	if latest != nil {
		next = latest.Version + 1
	}
	v := &types.AuthZPolicyVersion{
		TenantID:   tenantID,
		PolicyKey:  policyKey,
		Version:    next,
		Expression: expression,
		Decision:   decision,
		Metadata:   metadataJSON,
		CreatedBy:  createdBy,
	}
	if err := a.repo.CreateAuthZPolicyVersion(ctx, v); err != nil {
		return nil, err
	}
	a.cache.invalidate(tenantID, policyKey)
	return v, nil
}

// GetLatest returns the highest-version row for a policy_key.
func (a *AuthZAdmin) GetLatest(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error) {
	if cached, ok := a.cache.get(tenantID, policyKey); ok {
		return cached, nil
	}
	v, err := a.repo.GetLatestAuthZPolicy(ctx, tenantID, policyKey)
	if err != nil {
		return nil, err
	}
	if v != nil {
		a.cache.set(tenantID, policyKey, v)
	}
	return v, nil
}

// ListVersions returns all versions for one policy_key, newest first.
func (a *AuthZAdmin) ListVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error) {
	return a.repo.ListAuthZPolicyVersions(ctx, tenantID, policyKey)
}

// ListKeys returns the set of distinct policy_keys the tenant has
// defined. Used to populate the admin UI tree.
func (a *AuthZAdmin) ListKeys(ctx context.Context, tenantID uint64) ([]string, error) {
	return a.repo.ListAuthZPolicyKeys(ctx, tenantID)
}

// GetVersion returns one specific version row.
func (a *AuthZAdmin) GetVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error) {
	return a.repo.GetAuthZPolicyVersion(ctx, tenantID, id)
}

// DiffResult captures the human-readable diff between two versions.
// v0.7.22 ships a single-line summary; the full per-token diff is a
// future enhancement (planned for v0.7.24 SDK release).
type DiffResult struct {
	FromVersion int64
	ToVersion   int64
	ExpressionChanged bool
	DecisionChanged   bool
	Summary     string
}

// Diff returns a compact summary comparing two policy versions. Used by
// the admin UI to show "v1 → v2: decision flipped from allow to deny,
// expression unchanged" before applying a rollback.
func (a *AuthZAdmin) Diff(ctx context.Context, tenantID uint64, fromID, toID uint64) (*DiffResult, error) {
	from, err := a.repo.GetAuthZPolicyVersion(ctx, tenantID, fromID)
	if err != nil {
		return nil, err
	}
	to, err := a.repo.GetAuthZPolicyVersion(ctx, tenantID, toID)
	if err != nil {
		return nil, err
	}
	if from == nil || to == nil {
		return nil, fmt.Errorf("authzadmin.diff: one or both versions not found")
	}
	res := &DiffResult{
		FromVersion:       from.Version,
		ToVersion:         to.Version,
		ExpressionChanged: from.Expression != to.Expression,
		DecisionChanged:   from.Decision != to.Decision,
	}
	var parts []string
	if res.DecisionChanged {
		parts = append(parts, fmt.Sprintf("decision %s → %s", from.Decision, to.Decision))
	}
	if res.ExpressionChanged {
		parts = append(parts, fmt.Sprintf("expression diff len(%d) → len(%d)", len(from.Expression), len(to.Expression)))
	} else {
		parts = append(parts, "expression unchanged")
	}
	res.Summary = strings.Join(parts, "; ")
	return res, nil
}

// SimulateInput is the inputs to the dry-run evaluator.
type SimulateInput struct {
	PolicyKey string
	Actor     map[string]any
	Resource  map[string]any
	Action    string
}

// Simulate runs the latest policy expression against a synthetic actor +
// resource + action. For v0.7.22 the evaluator is intentionally simple:
// it recognises "allow" / "deny" decision strings and falls back to
// "deny" for any other case (fail closed). The richer OPA-style
// evaluator is tracked under Build #34.
func (a *AuthZAdmin) Simulate(ctx context.Context, tenantID uint64, in SimulateInput) (string, error) {
	latest, err := a.GetLatest(ctx, tenantID, in.PolicyKey)
	if err != nil {
		return "", err
	}
	if latest == nil {
		return "deny", nil // no policy = default deny
	}
	// Minimal evaluator: trust the stored decision for now. The richer
	// expression engine lands in Build #34 (referenced from v0.7.22 plan).
	logger.Debugf(ctx, "[AuthZAdmin] simulate %s actor=%v resource=%v decision=%s",
		in.PolicyKey, in.Actor, in.Resource, latest.Decision)
	switch latest.Decision {
	case types.AuthZDecisionAllow:
		return "allow", nil
	case types.AuthZDecisionConditional:
		// For v0.7.22: conditional evaluates to allow if actor has the
		// required tag; deny otherwise. Future versions will evaluate
		// the full DSL.
		if _, ok := in.Actor["conditional_pass"]; ok {
			return "allow", nil
		}
		return "deny", nil
	default:
		return "deny", nil
	}
}

// --- cache ---

type versionCache struct {
	mu      sync.RWMutex
	entries map[string]*types.AuthZPolicyVersion // key = tenant + "|" + policy_key
}

const versionCacheTTL = 5 * time.Minute

func newVersionCache() *versionCache {
	return &versionCache{entries: map[string]*types.AuthZPolicyVersion{}}
}

func cacheKey(tenantID uint64, policyKey string) string {
	return fmt.Sprintf("%d|%s", tenantID, policyKey)
}

func (c *versionCache) get(tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[cacheKey(tenantID, policyKey)]
	if !ok {
		return nil, false
	}
	if time.Since(v.CreatedAt) > versionCacheTTL {
		return nil, false
	}
	return v, true
}

func (c *versionCache) set(tenantID uint64, policyKey string, v *types.AuthZPolicyVersion) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(tenantID, policyKey)] = v
}

func (c *versionCache) invalidate(tenantID uint64, policyKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, cacheKey(tenantID, policyKey))
}


// AuthZRepoMirror lets the container pass any repo whose method set
// matches interfaces.DLPAuthZRepository (the AuthZ admin uses the
// same table-backed interface).
type AuthZRepoMirror interface {
	CreateAuthZPolicyVersion(ctx context.Context, p *types.AuthZPolicyVersion) error
	GetAuthZPolicyVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error)
	GetLatestAuthZPolicy(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error)
	ListAuthZPolicyVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error)
	ListAuthZPolicyKeys(ctx context.Context, tenantID uint64) ([]string, error)
}

type authZAdapter struct{ inner AuthZRepoMirror }

func (a *authZAdapter) CreateAuthZPolicyVersion(ctx context.Context, p *types.AuthZPolicyVersion) error {
	return a.inner.CreateAuthZPolicyVersion(ctx, p)
}
func (a *authZAdapter) GetAuthZPolicyVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error) {
	return a.inner.GetAuthZPolicyVersion(ctx, tenantID, id)
}
func (a *authZAdapter) GetLatestAuthZPolicy(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error) {
	return a.inner.GetLatestAuthZPolicy(ctx, tenantID, policyKey)
}
func (a *authZAdapter) ListAuthZPolicyVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error) {
	return a.inner.ListAuthZPolicyVersions(ctx, tenantID, policyKey)
}
func (a *authZAdapter) ListAuthZPolicyKeys(ctx context.Context, tenantID uint64) ([]string, error) {
	return a.inner.ListAuthZPolicyKeys(ctx, tenantID)
}

// NewAuthZAdminWithExternal wires the service from any repo whose
// method set matches AuthZRepoMirror.
func NewAuthZAdminWithExternal(repo AuthZRepoMirror) *AuthZAdmin {
	return NewAuthZAdmin(&authZAdapter{inner: repo})
}
