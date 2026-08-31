package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// DLPAuthZRepository bundles CRUD for the v0.7.22 DLP + AuthZ tables:
// dlp_policies, dlp_rules, dlp_violations, authz_policy_versions.
// Defined as a single seam so future bulk inserts (e.g. migration
// uploads of canned Microsoft Purview rule packs) have one place to
// add batch methods.
type DLPAuthZRepository interface {
	// DLP policies
	CreateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	GetDLPPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error)
	ListDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	ListActiveDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error)
	UpdateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error
	NextDLPPolicyVersion(ctx context.Context, tenantID uint64, name string) (int64, error)

	// DLP rules
	CreateDLPRule(ctx context.Context, r *types.DLPRule) error
	ListDLPRulesByPolicy(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error)
	DeleteDLPRule(ctx context.Context, tenantID uint64, ruleID uint64) error

	// DLP violations (append-only)
	CreateDLPViolation(ctx context.Context, v *types.DLPViolation) error
	ListDLPViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error)

	// AuthZ policy versions
	CreateAuthZPolicyVersion(ctx context.Context, p *types.AuthZPolicyVersion) error
	GetAuthZPolicyVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error)
	GetLatestAuthZPolicy(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error)
	ListAuthZPolicyVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error)
	ListAuthZPolicyKeys(ctx context.Context, tenantID uint64) ([]string, error)
}

// Compile-time check that the repo satisfies the contract.
var _ DLPAuthZRepository = (DLPAuthZRepository)(nil)
