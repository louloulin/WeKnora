package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// RegionRepository persists regions, tenant-region bindings, and the
// cross-region audit log. Implementations must be safe for concurrent
// use because the RegionEnforcer middleware reads bindings on every
// request.
type RegionRepository interface {
	// Region catalog.
	ListRegions(ctx context.Context) ([]*types.RegionRecord, error)
	GetRegion(ctx context.Context, id string) (*types.RegionRecord, error)
	UpsertRegion(ctx context.Context, r *types.RegionRecord) error

	// Tenant bindings.
	GetTenantBinding(ctx context.Context, tenantID uint64) (*types.TenantRegionBinding, error)
	UpsertTenantBinding(ctx context.Context, b *types.TenantRegionBinding) error
	DeleteTenantBinding(ctx context.Context, tenantID uint64) error
	ListAllTenantBindings(ctx context.Context) ([]*types.TenantRegionBinding, error)

	// Cross-region audit log.
	AppendAudit(ctx context.Context, log *types.CrossRegionAuditLog) error
	ListAuditByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.CrossRegionAuditLog, error)
	ListDeniedAudit(ctx context.Context, limit int) ([]*types.CrossRegionAuditLog, error)
}
