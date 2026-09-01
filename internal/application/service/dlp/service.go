package dlp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Scanner errors.
var (
	ErrPolicyNotFound  = errors.New("dlp.scanner: policy not found")
	ErrInvalidSeverity = errors.New("dlp.scanner: invalid severity")
	ErrInvalidAction   = errors.New("dlp.scanner: invalid action")
)

// DLPScanner is the public façade for the v0.7.22 Data Loss Prevention
// feature. Composed of:
//
//   * newScanner(policyRules)  — internal regex / dictionary compiler
//   * repo                     — load policies + rules, log violations
//
// Three sub-services are exposed to the handler:
//
//   Policies()  — CRUD for dlp_policies + dlp_rules
//   Scan()      — run the scanner on input text, log violations
//   Violations() — query the violation history (SOC 2 / GDPR audits)
type DLPScanner struct {
	repo typesRepo
	// Compile rules only once per tenant per minute via this cache.
	// For v0.7.22 we cache in-memory only; production would plug in Redis.
	cache *ruleCache
}

// typesRepo is the minimal repo contract DLPScanner needs. Defined here
// instead of importing interfaces.DLPAuthZRepository so the service
// package stays free of an import cycle.
type typesRepo interface {
	CreateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	GetDLPPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error)
	ListDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	ListActiveDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	UpdateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	NextDLPPolicyVersion(ctx context.Context, tenantID uint64, name string) (int64, error)
	CreateDLPRule(ctx context.Context, r *types.DLPRule) error
	ListDLPRulesByPolicy(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error)
	DeleteDLPRule(ctx context.Context, tenantID uint64, ruleID uint64) error
	CreateDLPViolation(ctx context.Context, v *types.DLPViolation) error
	ListDLPViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error)
}

// NewDLPScanner wires the scanner to the repo.
func NewDLPScanner(repo typesRepo) *DLPScanner {
	return &DLPScanner{repo: repo, cache: newRuleCache()}
}

// --- policies ---

// CreatePolicy creates a new version-1 policy (or bumps the version if
// the name already exists). Returns the persisted row.
func (s *DLPScanner) CreatePolicy(ctx context.Context, tenantID, createdBy uint64,
	name, resourceScope, severity, action, description string,
) (*types.DLPPolicy, error) {
	if !types.DLPValidSeverity(severity) {
		return nil, ErrInvalidSeverity
	}
	if !types.DLPValidAction(action) {
		return nil, ErrInvalidAction
	}
	version, err := s.repo.NextDLPPolicyVersion(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	p := &types.DLPPolicy{
		TenantID:      tenantID,
		Name:          name,
		Version:       version,
		ResourceScope: resourceScope,
		Severity:      severity,
		Action:        action,
		IsActive:      false,
		Description:   description,
		CreatedBy:     createdBy,
	}
	if err := s.repo.CreateDLPPolicy(ctx, p); err != nil {
		return nil, err
	}
	s.cache.invalidate(tenantID)
	return p, nil
}

// ListPolicies returns every policy (all versions) for a tenant.
func (s *DLPScanner) ListPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error) {
	return s.repo.ListDLPPolicies(ctx, tenantID)
}

// GetPolicy returns one policy row.
func (s *DLPScanner) GetPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error) {
	return s.repo.GetDLPPolicy(ctx, tenantID, id)
}

// ActivatePolicy flips is_active=true and deactivates other policies
// with the same name. Atomic transaction.
func (s *DLPScanner) ActivatePolicy(ctx context.Context, tenantID uint64, id uint64) error {
	p, err := s.repo.GetDLPPolicy(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPolicyNotFound
	}
	p.IsActive = true
	return s.repo.UpdateDLPPolicy(ctx, p)
}

// --- rules ---

// AddRule appends a regex / dictionary / builtin rule to a policy.
// Returns ErrInvalidSeverity if severity is not one of the canonical
// values.
func (s *DLPScanner) AddRule(ctx context.Context, tenantID uint64, policyID uint64,
	patternType, patternValue, severity, description string,
) (*types.DLPRule, error) {
	if !types.DLPValidPatternType(patternType) {
		return nil, fmt.Errorf("dlp.scanner: invalid pattern_type %q", patternType)
	}
	if !types.DLPValidSeverity(severity) {
		return nil, ErrInvalidSeverity
	}
	if patternType == types.DLPPatternBuiltin && !IsBuiltinName(patternValue) {
		return nil, fmt.Errorf("dlp.scanner: unknown builtin pattern %q", patternValue)
	}
	r := &types.DLPRule{
		PolicyID:     policyID,
		TenantID:     tenantID,
		PatternType:  patternType,
		PatternValue: patternValue,
		Severity:     severity,
		Enabled:      true,
		Description:  description,
	}
	if err := s.repo.CreateDLPRule(ctx, r); err != nil {
		return nil, err
	}
	s.cache.invalidate(tenantID)
	return r, nil
}

// ListRules returns all rules for one policy.
func (s *DLPScanner) ListRules(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error) {
	return s.repo.ListDLPRulesByPolicy(ctx, tenantID, policyID)
}

// DeleteRule removes one rule from a policy. Idempotent on missing.
func (s *DLPScanner) DeleteRule(ctx context.Context, tenantID uint64, ruleID uint64) error {
	if err := s.repo.DeleteDLPRule(ctx, tenantID, ruleID); err != nil {
		return err
	}
	s.cache.invalidate(tenantID)
	return nil
}

// --- scan ---

// ScanInput captures the inputs needed for one scan.
type ScanInput struct {
	TenantID   uint64
	Resource   string // e.g. "wiki_page"
	ResourceID string
	ActorID    uint64
	Text       string
}

// ScanResult bundles the matches + the chosen actions.
type ScanResult struct {
	Matches    []Match
	ViolationIDs []uint64
	ActionCounts map[string]int // action → count
}

// Scan runs every active DLP policy against the input text, logs
// violations to dlp_violations, and returns the matches. Returns an
// empty result (not error) if no policies are active.
func (s *DLPScanner) Scan(ctx context.Context, in ScanInput) (*ScanResult, error) {
	policies, err := s.repo.ListActiveDLPPolicies(ctx, in.TenantID)
	if err != nil {
		return nil, fmt.Errorf("dlp.scan.list_policies: %w", err)
	}
	if len(policies) == 0 {
		return &ScanResult{Matches: nil, ViolationIDs: nil, ActionCounts: map[string]int{}}, nil
	}
	// Load + compile rules once per tenant.
	allRules, err := s.loadAllRules(ctx, in.TenantID, policies)
	if err != nil {
		return nil, err
	}
	sc := newScanner(allRules)
	matches := sc.scan(in.Text, 200)
	res := &ScanResult{
		Matches:      matches,
		ViolationIDs: make([]uint64, 0, len(matches)),
		ActionCounts: map[string]int{},
	}
	for _, m := range matches {
		v := &types.DLPViolation{
			TenantID:       in.TenantID,
			PolicyID:       m.PolicyID,
			RuleID:         &m.RuleID,
			Resource:       in.Resource,
			ResourceID:     in.ResourceID,
			ActorID:        in.ActorID,
			MatchedValue:   m.MatchedValue,
			Context:        m.Context,
			MatchedPattern: m.PatternName,
			ActionTaken:    m.Action,
			Severity:       m.Severity,
		}
		if err := s.repo.CreateDLPViolation(ctx, v); err != nil {
			// Don't fail the whole scan on a single violation insert.
			continue
		}
		res.ViolationIDs = append(res.ViolationIDs, v.ID)
		res.ActionCounts[m.Action]++
	}
	return res, nil
}

// loadAllRules loads every rule from every active policy. The result
// is cached for ~1 minute per tenant to avoid hammering the DB on hot
// scan paths.
func (s *DLPScanner) loadAllRules(ctx context.Context, tenantID uint64,
	policies []*types.DLPPolicy,
) ([]policyRule, error) {
	if cached, ok := s.cache.get(tenantID); ok {
		return cached, nil
	}
	var out []policyRule
	for _, p := range policies {
			rules, err := s.repo.ListDLPRulesByPolicy(ctx, tenantID, p.ID)
			if err != nil {
				return nil, err
			}
			for _, r := range rules {
				out = append(out, policyRule{
					Rule:     *r,
					PolicyID: p.ID,
					Action:   p.Action,
					Severity: r.Severity,
				})
			}
		}
	s.cache.set(tenantID, out)
	return out, nil
}

// --- violations ---

// ListViolations returns the violation history. resource is optional —
// pass empty string for "all resources".
func (s *DLPScanner) ListViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error) {
	return s.repo.ListDLPViolations(ctx, tenantID, resource, limit, offset)
}

// --- helper to import strings & time without breaking unused-import lints ---

// TrimLower normalises a string for dictionary storage / matching.
// Public so the handler can pre-validate dictionary entries.
func TrimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Now is exposed so tests can freeze the clock without exporting the
// internal rule-cache clock.
func Now() time.Time { return time.Now().UTC() }


// DLPAuthZRepoMirror matches interfaces.DLPAuthZRepository so the
// container can pass it in without going through the package-local
// typesRepo interface.
type DLPAuthZRepoMirror interface {
	CreateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	GetDLPPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error)
	ListDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	ListActiveDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	UpdateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	NextDLPPolicyVersion(ctx context.Context, tenantID uint64, name string) (int64, error)
	CreateDLPRule(ctx context.Context, r *types.DLPRule) error
	ListDLPRulesByPolicy(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error)
	DeleteDLPRule(ctx context.Context, tenantID uint64, ruleID uint64) error
	CreateDLPViolation(ctx context.Context, v *types.DLPViolation) error
	ListDLPViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error)
}

type dlpAdapter struct{ inner DLPAuthZRepoMirror }

func (a *dlpAdapter) CreateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error { return a.inner.CreateDLPPolicy(ctx, p) }
func (a *dlpAdapter) GetDLPPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error) { return a.inner.GetDLPPolicy(ctx, tenantID, id) }
func (a *dlpAdapter) ListDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error) { return a.inner.ListDLPPolicies(ctx, tenantID) }
func (a *dlpAdapter) ListActiveDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error) { return a.inner.ListActiveDLPPolicies(ctx, tenantID) }
func (a *dlpAdapter) UpdateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error { return a.inner.UpdateDLPPolicy(ctx, p) }
func (a *dlpAdapter) NextDLPPolicyVersion(ctx context.Context, tenantID uint64, name string) (int64, error) { return a.inner.NextDLPPolicyVersion(ctx, tenantID, name) }
func (a *dlpAdapter) CreateDLPRule(ctx context.Context, r *types.DLPRule) error { return a.inner.CreateDLPRule(ctx, r) }
func (a *dlpAdapter) ListDLPRulesByPolicy(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error) { return a.inner.ListDLPRulesByPolicy(ctx, tenantID, policyID) }
func (a *dlpAdapter) DeleteDLPRule(ctx context.Context, tenantID uint64, ruleID uint64) error { return a.inner.DeleteDLPRule(ctx, tenantID, ruleID) }
func (a *dlpAdapter) CreateDLPViolation(ctx context.Context, v *types.DLPViolation) error { return a.inner.CreateDLPViolation(ctx, v) }
func (a *dlpAdapter) ListDLPViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error) { return a.inner.ListDLPViolations(ctx, tenantID, resource, limit, offset) }

// NewDLPScannerWithExternal wires the scanner from any repo whose
// method set matches DLPAuthZRepoMirror (typically the exported
// interfaces.DLPAuthZRepository from the interfaces package).
func NewDLPScannerWithExternal(repo DLPAuthZRepoMirror) *DLPScanner {
	return NewDLPScanner(&dlpAdapter{inner: repo})
}
