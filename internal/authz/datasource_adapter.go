package authz

import (
	"context"
)

// DataSourceAdapter answers checks against DataSource objects. The
// decision model mirrors KBAdapter with three layers:
//
//  1. Datasource creator short-circuit (mirrors KB creator rule).
//  2. KB-level permission inheritance — a user with role X on the
//     parent KB has role X on every datasource under it.
//  3. Direct cross-tenant share on the datasource.
//
// A datasource with no recorded creator falls through to KB-level
// inheritance so we never accidentally 404 a legacy row.
type DataSourceAdapter struct {
	CreatorLookup DataSourceCreatorLookup
	ShareLookup   DataSourceShareLookup
	// KBAccessLookup resolves the caller's effective role on the
	// parent KB. The KBShareLookup signature is reused because the
	// (effectiveRole, isShared) shape is identical.
	KBAccessLookup KBShareLookup
}

// NewDataSourceAdapter constructs the adapter. The three lookups
// are independent: a partially-configured adapter degrades to
// "owner + KB inheritance" (the share layer becomes a no-op) so
// single-tenant deployments without cross-tenant data source
// sharing still work.
func NewDataSourceAdapter(creator DataSourceCreatorLookup, share DataSourceShareLookup, kbAccess KBShareLookup) *DataSourceAdapter {
	return &DataSourceAdapter{
		CreatorLookup:  creator,
		ShareLookup:    share,
		KBAccessLookup: kbAccess,
	}
}

// ObjectType returns the datasource object type.
func (a *DataSourceAdapter) ObjectType() ObjectType { return ObjectTypeDatasource }

// Check answers the request.
func (a *DataSourceAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "datasource"
	if a.CreatorLookup == nil {
		return Deny(CodeError, source, "creator lookup is not configured")
	}
	tenantID, creatorID, kbID, found, err := a.CreatorLookup(ctx, req.Object.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !found {
		return Deny(CodeNoSuchResource, source, "data source does not exist")
	}
	if req.Object.TenantID == 0 {
		req.Object.TenantID = tenantID
	}
	// Defensive cross-tenant guard (composite already short-circuits).
	if req.User.TenantID != 0 && req.User.TenantID != tenantID {
		return a.checkShare(ctx, req, tenantID)
	}

	// Same-tenant path: creator always wins.
	if creatorID != "" && req.User.ID == creatorID {
		return Allow(source, "user is the datasource creator")
	}
	// KB inheritance: the caller's tenant role on the parent KB
	// grants the same role on every datasource under it.
	if kbID != "" && a.KBAccessLookup != nil {
		effectiveRole, isShared, err := a.KBAccessLookup(ctx, kbID, req.User.TenantID, req.User.Role)
		if err == nil && isShared && effectiveRole != "" {
			required := requiredRankForRelation(req.Relation)
			if orgRoleRank(effectiveRole) >= required {
				return Allow(source,
					"kb role "+effectiveRole+" satisfies "+string(req.Relation))
			}
		}
	}
	// Fall back to caller role for same-tenant.
	rank := roleRank(req.User.Role)
	required := requiredRankForRelation(req.Relation)
	if rank >= required {
		return Allow(source, "tenant role "+req.User.Role+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"tenant role "+req.User.Role+" below required for "+string(req.Relation))
}

// checkShare consults the cross-tenant datasource share layer.
func (a *DataSourceAdapter) checkShare(ctx context.Context, req CheckRequest, sourceTenantID uint64) Decision {
	source := "datasource_share"
	if a.ShareLookup == nil {
		return Deny(CodeNotShared, source, "no share grant and no same-tenant access")
	}
	effectiveRole, isShared, err := a.ShareLookup(ctx, req.Object.ID, req.User.TenantID, req.User.Role)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !isShared {
		return Deny(CodeNotShared, source,
			"caller's tenant has no org-level share to this datasource")
	}
	required := requiredRankForRelation(req.Relation)
	if orgRoleRank(effectiveRole) >= required {
		return Allow(source,
			"share grant effective role "+effectiveRole+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"share role "+effectiveRole+" below required for "+string(req.Relation))
}

// Invalidate is a no-op for the datasource adapter.
func (a *DataSourceAdapter) Invalidate(_ context.Context, _ Object) error { return nil }

// Compile-time guard.
var _ Adapter = (*DataSourceAdapter)(nil)
