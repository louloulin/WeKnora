package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// ConditionalAccessRepository is the persistence contract for
// conditional access policies. The hot read path (login) only
// needs ListEnabled; the admin CRUD surface uses the rest.
type ConditionalAccessRepository interface {
	// Create persists a new policy. The handler must ensure the
	// (tenant_id, name) uniqueness constraint is honoured; the
	// repository surfaces a duplicate as ErrConditionalAccessPolicyExists.
	Create(ctx context.Context, policy *types.ConditionalAccessPolicy) error

	// GetByID returns the policy regardless of soft-delete state so
	// the handler can render 404 vs 410 distinctly. The evaluator
	// only ever reads ListEnabled, so this method is admin-only.
	GetByID(ctx context.Context, tenantID string, id uint64) (*types.ConditionalAccessPolicy, error)

	// ListEnabled returns all live, enabled policies for the tenant,
	// ordered by priority ASC. The evaluator uses this as its hot
	// read input; the index idx_cond_acc_tenant_enabled_priority
	// supports it directly.
	ListEnabled(ctx context.Context, tenantID string) ([]*types.ConditionalAccessPolicy, error)

	// ListAll returns every policy (enabled + disabled, including
	// soft-deleted?) for the admin UI. The admin UI calls this; the
	// evaluator never does.
	ListAll(ctx context.Context, tenantID string, limit, offset int) ([]*types.ConditionalAccessPolicy, int64, error)

	// Update replaces the mutable fields (description, conditions,
	// action, priority, enabled). TenantID + name + created_by +
	// created_at are immutable.
	Update(ctx context.Context, policy *types.ConditionalAccessPolicy) error

	// SoftDelete flips deleted_at. Idempotent — calling on an
	// already-deleted row is a no-op success.
	SoftDelete(ctx context.Context, tenantID string, id uint64) error
}

// Sentinel errors re-exported at the service layer so the handler
// only ever switches on service-level sentinels.
var (
	ErrConditionalAccessPolicyNotFound = constErr("conditional_access_policy_not_found")
	ErrConditionalAccessPolicyExists    = constErr("conditional_access_policy_exists")
	ErrConditionalAccessInvalidRequest  = constErr("conditional_access_invalid_request")
)

// constErr is the tiny typed-error helper shared across the interfaces
// package. We define it locally per file so existing files do not have
// to be re-touched when new sentinel errors are added.
type constErr string

func (e constErr) Error() string { return string(e) }
