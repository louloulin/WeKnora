package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/authz"
)

// wikiSyncBlockAuthzAdapter bridges WikiSyncBlockService to the rich AuthZ
// phase-3 composite checker. Same pattern as wiki_realtime_authz.go.
type wikiSyncBlockAuthzAdapter struct {
	checker *authz.CompositeChecker
}

// NewWikiSyncBlockAuthzAdapter returns an adapter backed by the supplied
// composite checker. Pass nil for fail-closed (no access).
func NewWikiSyncBlockAuthzAdapter(checker *authz.CompositeChecker) WikiSyncBlockAuthorizer {
	if checker == nil {
		return nil
	}
	return &wikiSyncBlockAuthzAdapter{checker: checker}
}

func (a *wikiSyncBlockAuthzAdapter) CanReadKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error) {
	if a.checker == nil {
		return false, nil
	}
	d := a.checker.Check(ctx, authz.CheckRequest{
		User:     authz.User{ID: formatID(userID), TenantID: tenantID},
		Object:   authz.Object{Type: "knowledge_base", TenantID: tenantID, ID: kbID},
		Relation: authz.Relation("read"),
	})
	return d.Allowed, nil
}

func (a *wikiSyncBlockAuthzAdapter) CanWriteKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error) {
	if a.checker == nil {
		return false, nil
	}
	d := a.checker.Check(ctx, authz.CheckRequest{
		User:     authz.User{ID: formatID(userID), TenantID: tenantID},
		Object:   authz.Object{Type: "knowledge_base", TenantID: tenantID, ID: kbID},
		Relation: authz.Relation("write"),
	})
	return d.Allowed, nil
}

// formatID is a tiny helper kept local to avoid importing strconv here;
// authz.User.ID is a string per the type definition.
func formatID(id uint64) string {
	if id == 0 {
		return ""
	}
	return uintToString(id)
}

// uintToString avoids pulling strconv into the adapter when only this
// one call site needs it. The authz package would silently accept "" as
// a "service user" id; we always pass a positive uint64 here so the
// fallback path is unreachable.
func uintToString(v uint64) string {
	if v == 0 {
		return ""
	}
	// Hand-rolled to dodge the strconv dependency in this file. Cap at
	// 20 digits (max uint64 is 20 digits in base 10).
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
