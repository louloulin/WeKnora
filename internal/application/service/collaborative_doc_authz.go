// Package service — v0.7.25 collaborative_docs default authorizer.
//
// The default policy mirrors the wiki realtime default: the doc's owner
// has full access; everyone else in the same tenant gets read access when
// the doc visibility is "shared" / "tenant", and write access only when
// "shared-editable" or higher. This is a deliberately simple seam so the
// real AuthZ phase-3 policy can drop in later without changing the
// service.
package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// CollabDocDefaultAuthorizer is the default ACL implementation.
type CollabDocDefaultAuthorizer struct {
	docRepo interfaces.CollabDocRepository
}

// NewCollabDocDefaultAuthorizer wires the default policy.
func NewCollabDocDefaultAuthorizer(docRepo interfaces.CollabDocRepository) *CollabDocDefaultAuthorizer {
	return &CollabDocDefaultAuthorizer{docRepo: docRepo}
}

// CanRead returns true if the user is allowed to read the doc.
func (a *CollabDocDefaultAuthorizer) CanRead(ctx context.Context, tenantID, userID uint64, docID string) (bool, error) {
	d, err := a.docRepo.Get(ctx, tenantID, docID)
	if err != nil || d == nil {
		return false, nil
	}
	if d.OwnerUserID == userID {
		return true, nil
	}
	switch strings.ToLower(d.Visibility) {
	case "tenant", "shared":
		return true, nil
	}
	return false, nil
}

// CanWrite returns true if the user is allowed to write the doc. The
// owner always has write access; non-owners need visibility "shared-editable"
// or higher.
func (a *CollabDocDefaultAuthorizer) CanWrite(ctx context.Context, tenantID, userID uint64, docID string) (bool, error) {
	d, err := a.docRepo.Get(ctx, tenantID, docID)
	if err != nil || d == nil {
		return false, nil
	}
	if d.OwnerUserID == userID {
		return true, nil
	}
	if strings.EqualFold(d.Visibility, "shared-editable") || strings.EqualFold(d.Visibility, "tenant-editable") {
		return true, nil
	}
	return false, nil
}

// Interface guards.
var _ CollabDocAuthorizer = (*CollabDocDefaultAuthorizer)(nil)
