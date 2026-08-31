package authz

import (
	"context"
	"strings"
)

// KBAdapter answers checks against KB objects. The decision model
// has three layers, evaluated in order:
//
//  1. Same-tenant role cap: the caller's tenant role sets a ceiling
//     (Viewer / Contributor / Admin / Owner) — mirrors the matrix
//     in TenantRoleAdapter.
//  2. KB ownership: the KB's creator always has RelationOwner
//     regardless of their current tenant role (a former Admin who
//     has been demoted still owns their KB; demotion cannot erase
//     authorship).
//  3. Cross-tenant KB Share: when the caller's tenant has an org-
//     level share to the source tenant, the effective role from
//     the share (already 3-D-capped by the underlying service) is
//     honoured.
//
// Deny reasons encode which layer failed so an admin debugging a
// 403 can see at a glance whether the problem is the caller's
// role, the KB ownership, or the share grant.
type KBAdapter struct {
	CreatorLookup KBCreatorLookup
	ShareLookup   KBShareLookup
}

// NewKBAdapter constructs the adapter.
func NewKBAdapter(creator KBCreatorLookup, share KBShareLookup) *KBAdapter {
	return &KBAdapter{CreatorLookup: creator, ShareLookup: share}
}

// ObjectType returns the KB object type.
func (a *KBAdapter) ObjectType() ObjectType { return ObjectTypeKB }

// Check answers the request. Cross-tenant has already been
// short-circuited by the composite when the User and Object
// tenants differ — here we still re-check inside the share layer
// because the lookup itself is per-tenant.
func (a *KBAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "kb"
	if a.CreatorLookup == nil {
		return Deny(CodeError, source, "creator lookup is not configured")
	}
	tenantID, creatorID, found, err := a.CreatorLookup(ctx, req.Object.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !found {
		return Deny(CodeNoSuchResource, source, "knowledge base does not exist")
	}
	// Fill the tenant from the KB so subsequent share lookups
	// can compare the caller's tenant against the source tenant.
	if req.Object.TenantID == 0 {
		req.Object.TenantID = tenantID
	}
	// Cross-tenant deny (defensive; the composite already did this).
	if req.User.TenantID != 0 && req.User.TenantID != tenantID {
		// Same-tenant access denied; check share layer.
		return a.checkShare(ctx, req, tenantID)
	}

	// Same-tenant path: owner always wins; otherwise role matrix.
	if creatorID != "" && req.User.ID == creatorID {
		return Allow(source, "user is the KB creator")
	}
	rank := roleRank(req.User.Role)
	required := requiredRankForRelation(req.Relation)
	if rank >= required {
		return Allow(source, "tenant role "+req.User.Role+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"tenant role "+req.User.Role+" below required for "+string(req.Relation))
}

// checkShare consults the cross-tenant KB Share layer. It is only
// invoked when the caller's tenant differs from the KB's source
// tenant (same-tenant denies fall through from Check).
func (a *KBAdapter) checkShare(ctx context.Context, req CheckRequest, sourceTenantID uint64) Decision {
	source := "kb_share"
	if a.ShareLookup == nil {
		return Deny(CodeNotShared, source, "no share grant and no same-tenant access")
	}
	effectiveRole, isShared, err := a.ShareLookup(ctx, req.Object.ID, req.User.TenantID, req.User.Role)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !isShared {
		return Deny(CodeNotShared, source,
			"caller's tenant has no org-level share to this KB")
	}
	if effectiveRole == "" {
		return Deny(CodeNotShared, source,
			"share exists but resolved to no effective role")
	}
	// Effective role "owner" > "admin" > "editor" > "viewer".
	required := requiredRankForRelation(req.Relation)
	effectiveRank := orgRoleRank(effectiveRole)
	if effectiveRank >= required {
		return Allow(source,
			"share grant effective role "+effectiveRole+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"share role "+effectiveRole+" below required for "+string(req.Relation))
}

// orgRoleRank maps the share role string to the same rank scale
// TenantRoleAdapter uses so a relation required-rank comparison is
// apples-to-apples.
func orgRoleRank(role string) int {
	switch strings.ToLower(role) {
	case "owner":
		return 40
	case "admin":
		return 30
	case "editor":
		return 20
	case "viewer":
		return 10
	}
	return 0
}

// Invalidate is a no-op for KB; the share / creator lookups are
// direct reads, and the composite cache is invalidated by the
// caller.
func (a *KBAdapter) Invalidate(_ context.Context, _ Object) error { return nil }

// WikiPageAdapter answers checks against wiki pages. The model is
// intentionally minimal — pages inherit from their KB unless they
// have an explicit ACL row, in which case the ACL mode decides.
//
// Decision flow:
//   - System principal → allow.
//   - Page creator → allow.
//   - KB Admin / Owner role → allow.
//   - ACL row with mode=inherit → allow (every KB member).
//   - ACL row with mode=allow_list and caller in allow_list (incl.
//     group expansion) → allow.
//   - ACL row with mode=private → deny_not_shared.
//   - Page does not exist → deny_no_such_resource.
type WikiPageAdapter struct {
	ResolveLookup WikiPageResolveLookup
	OwnerLookup   WikiPageOwnerLookup
}

// NewWikiPageAdapter constructs the adapter.
func NewWikiPageAdapter(resolve WikiPageResolveLookup, owner WikiPageOwnerLookup) *WikiPageAdapter {
	return &WikiPageAdapter{ResolveLookup: resolve, OwnerLookup: owner}
}

// ObjectType returns the wiki-page object type.
func (a *WikiPageAdapter) ObjectType() ObjectType { return ObjectTypeWikiPage }

// Check answers the request.
func (a *WikiPageAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "wiki_page"
	if a.OwnerLookup == nil {
		return Deny(CodeError, source, "owner lookup is not configured")
	}
	if req.User.Type == UserTypeSystem {
		return Allow(source, "system principal allowed")
	}
	// kbID is encoded in Object.ID as "<kbID>:<slug>" because the
	// wiki-page Object does not have a dedicated kb field.
	// Format: "<kbID>/<slug>".
	parts := strings.SplitN(req.Object.ID, "/", 2)
	if len(parts) != 2 {
		return Deny(CodeError, source,
			"wiki page object id must be 'kbID/slug'")
	}
	kbID := parts[0]
	slug := parts[1]

	creatorID, kbTenantID, pageFound, err := a.OwnerLookup(ctx, kbID, slug)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !pageFound {
		return Deny(CodeNoSuchResource, source, "wiki page does not exist")
	}
	if req.Object.TenantID == 0 {
		req.Object.TenantID = kbTenantID
	}
	// Owner short-circuit.
	if creatorID != "" && req.User.ID == creatorID {
		return Allow(source, "user is the page creator")
	}
	// KB Admin / Owner bypass.
	if roleRank(req.User.Role) >= roleRank("admin") {
		return Allow(source, "tenant role "+req.User.Role+" can manage wiki pages")
	}
	// Defer to ACL.
	if a.ResolveLookup == nil {
		return Deny(CodeError, source, "resolve lookup is not configured")
	}
	decision, resolveFound, err := a.ResolveLookup(ctx, kbID, slug, req.User.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !resolveFound {
		// No ACL row AND not owner AND not admin → "page not found"
		// to avoid existence leaks. Owner/admin paths already
		// returned above.
		return Deny(CodeNoSuchResource, source, "wiki page not visible to user")
	}
	switch decision {
	case "allow":
		return Allow(source, "ACL allows user")
	case "deny_allow_list":
		return Deny(CodeNotShared, source, "user not on page allow_list")
	case "deny_private":
		return Deny(CodeNotShared, source, "page is private")
	default:
		return Deny(CodeError, source, "unknown ACL decision: "+decision)
	}
}

// Invalidate is a no-op here; the underlying WikiAclService has its
// own 60s cache and invalidation surface.
func (a *WikiPageAdapter) Invalidate(_ context.Context, _ Object) error { return nil }

// AgentAdapter answers checks against Agent objects. Same shape as
// KBAdapter: ownership + same-tenant role matrix + cross-tenant
// Agent Share.
type AgentAdapter struct {
	CreatorLookup AgentCreatorLookup
	ShareLookup   AgentShareLookup
}

// NewAgentAdapter constructs the adapter.
func NewAgentAdapter(creator AgentCreatorLookup, share AgentShareLookup) *AgentAdapter {
	return &AgentAdapter{CreatorLookup: creator, ShareLookup: share}
}

// ObjectType returns the agent object type.
func (a *AgentAdapter) ObjectType() ObjectType { return ObjectTypeAgent }

// Check answers the request.
func (a *AgentAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "agent"
	if a.CreatorLookup == nil {
		return Deny(CodeError, source, "creator lookup is not configured")
	}
	tenantID, creatorID, found, err := a.CreatorLookup(ctx, req.Object.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !found {
		return Deny(CodeNoSuchResource, source, "agent does not exist")
	}
	if req.Object.TenantID == 0 {
		req.Object.TenantID = tenantID
	}
	if req.User.TenantID != 0 && req.User.TenantID != tenantID {
		return a.checkShare(ctx, req, tenantID)
	}
	if creatorID != "" && req.User.ID == creatorID {
		return Allow(source, "user is the agent creator")
	}
	rank := roleRank(req.User.Role)
	required := requiredRankForRelation(req.Relation)
	if rank >= required {
		return Allow(source, "tenant role "+req.User.Role+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"tenant role "+req.User.Role+" below required for "+string(req.Relation))
}

// checkShare mirrors KBAdapter.checkShare for agents.
func (a *AgentAdapter) checkShare(ctx context.Context, req CheckRequest, sourceTenantID uint64) Decision {
	source := "agent_share"
	if a.ShareLookup == nil {
		return Deny(CodeNotShared, source, "no share grant and no same-tenant access")
	}
	effectiveRole, isShared, err := a.ShareLookup(ctx, req.Object.ID, req.User.TenantID, req.User.Role)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !isShared {
		return Deny(CodeNotShared, source,
			"caller's tenant has no share to this agent")
	}
	if effectiveRole == "" {
		return Deny(CodeNotShared, source,
			"share exists but resolved to no effective role")
	}
	required := requiredRankForRelation(req.Relation)
	effectiveRank := orgRoleRank(effectiveRole)
	if effectiveRank >= required {
		return Allow(source,
			"agent share effective role "+effectiveRole+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"agent share role "+effectiveRole+" below required for "+string(req.Relation))
}

// Invalidate is a no-op for agent; share / creator lookups are
// direct reads.
func (a *AgentAdapter) Invalidate(_ context.Context, _ Object) error { return nil }
