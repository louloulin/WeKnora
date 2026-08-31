package service

import (
	"context"
	"strconv"

	"github.com/Tencent/WeKnora/internal/authz"
)

// wikiRealtimeAuthzAdapter bridges the realtime service's minimal authorizer
// seam (WikiRealtimeAuthorizer) to the rich AuthZ phase-3 composite checker.
//
// The adapter is intentionally narrow: it answers "can this user read or
// write this page" without dragging the full RBAC role matrix into the
// hot path. The composite Checker still runs the full tuple evaluation
// behind the scenes, so the result reflects phase-3 semantics.
type wikiRealtimeAuthzAdapter struct {
	checker *authz.CompositeChecker
}

// NewWikiRealtimeAuthzAdapter returns an adapter backed by the supplied
// composite checker. Pass nil if no AuthZ wiring exists (the realtime
// service refuses all access in that mode — fail-closed).
func NewWikiRealtimeAuthzAdapter(checker *authz.CompositeChecker) WikiRealtimeAuthorizer {
	if checker == nil {
		return nil
	}
	return &wikiRealtimeAuthzAdapter{checker: checker}
}

func (a *wikiRealtimeAuthzAdapter) CanRead(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error) {
	if a.checker == nil {
		return false, nil
	}
	d := a.checker.Check(ctx, authz.CheckRequest{
		User:     authz.User{ID: strconv.FormatUint(userID, 10), TenantID: tenantID},
		Object:   authz.Object{Type: "wiki_page", TenantID: tenantID, ID: pageID},
		Relation: authz.Relation("read"),
	})
	return d.Allowed, nil
}

func (a *wikiRealtimeAuthzAdapter) CanWrite(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error) {
	if a.checker == nil {
		return false, nil
	}
	// KB-level write first (cheap check), then page-level.
	d := a.checker.Check(ctx, authz.CheckRequest{
		User:     authz.User{ID: strconv.FormatUint(userID, 10), TenantID: tenantID},
		Object:   authz.Object{Type: "knowledge_base", TenantID: tenantID, ID: kbID},
		Relation: authz.Relation("write"),
	})
	if !d.Allowed {
		return false, nil
	}
	d = a.checker.Check(ctx, authz.CheckRequest{
		User:     authz.User{ID: strconv.FormatUint(userID, 10), TenantID: tenantID},
		Object:   authz.Object{Type: "wiki_page", TenantID: tenantID, ID: pageID},
		Relation: authz.Relation("write"),
	})
	return d.Allowed, nil
}
