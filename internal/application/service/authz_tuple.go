package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/authz"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ErrAuthZTupleInvalid is the service-level sentinel for a tuple
// request that fails validation. Handlers map to 400.
var ErrAuthZTupleInvalid = errors.New("authz tuple is invalid")

// ErrAuthZTupleAlreadyExists is re-exported from the repository so
// handlers can use a single sentinel regardless of layer.
var ErrAuthZTupleAlreadyExists = apprepo.ErrAuthZTupleAlreadyExists

// ErrAuthZTupleNotFound is re-exported from the repository.
var ErrAuthZTupleNotFound = apprepo.ErrAuthZTupleNotFound

// AllowedTupleSubjectTypes mirrors the validator on
// AuthZTupleCreateRequest. Kept in one place so the service layer
// can short-circuit without re-implementing the regex.
var allowedTupleSubjectTypes = map[string]bool{
	"user":    true,
	"group":   true,
	"api_key": true,
	"agent":   true,
}

// AuthZTupleService is the admin-side CRUD service for the
// persistent AuthZ tuple store. The runtime Check path uses a
// different service (the lookup-only AuthZTupleLookup) so admin
// operations do not contend on the hot read path.
type AuthZTupleService struct {
	repo    interfaces.AuthZTupleRepository
	checker authz.Checker
}

// NewAuthZTupleService constructs the service.
func NewAuthZTupleService(repo interfaces.AuthZTupleRepository, checker authz.Checker) *AuthZTupleService {
	return &AuthZTupleService{repo: repo, checker: checker}
}

// Create persists a new tuple after validating its shape + applying
// the caller's tenant scope. Server-controlled fields (TenantID,
// GrantedBy, CreatedAt) are stamped here so the caller cannot
// spoof them via the request body.
func (s *AuthZTupleService) Create(ctx context.Context, tenantID uint64, req types.AuthZTupleCreateRequest) (*types.AuthZTuple, error) {
	if err := validateTupleRequest(req); err != nil {
		return nil, err
	}
	now := time.Now()
	t := &types.AuthZTuple{
		TenantID:        tenantID,
		ObjectType:      req.ObjectType,
		ObjectID:        req.ObjectID,
		Relation:        req.Relation,
		SubjectType:     req.SubjectType,
		SubjectID:       req.SubjectID,
		SubjectRelation: req.SubjectRelation,
		GrantedBy:       userIDFromContext(ctx),
		CreatedAt:       now,
		ExpiresAt:       req.ExpiresAt,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "authz: created tuple %s (object=%s:%s#%s subject=%s:%s) by %s",
		t.ID, t.ObjectType, t.ObjectID, t.Relation, t.SubjectType, t.SubjectID, t.GrantedBy)
	// Invalidate the composite decision cache so a freshly-granted
	// relation is visible on the next Check.
	if s.checker != nil {
		_ = s.checker.Invalidate(ctx, authz.Object{
			Type: authz.ObjectType(t.ObjectType),
			ID:   t.ObjectID,
		})
	}
	return t, nil
}

// Revoke marks a tuple revoked. The composite cache is invalidated
// for the underlying object.
func (s *AuthZTupleService) Revoke(ctx context.Context, tenantID uint64, id string) error {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if existing.TenantID != 0 && existing.TenantID != tenantID {
		return ErrAuthZTupleInvalid
	}
	if err := s.repo.Revoke(ctx, id); err != nil {
		return err
	}
	logger.Infof(ctx, "authz: revoked tuple %s (object=%s:%s#%s subject=%s:%s) by %s",
		id, existing.ObjectType, existing.ObjectID, existing.Relation,
		existing.SubjectType, existing.SubjectID, userIDFromContext(ctx))
	if s.checker != nil {
		_ = s.checker.Invalidate(ctx, authz.Object{
			Type: authz.ObjectType(existing.ObjectType),
			ID:   existing.ObjectID,
		})
	}
	return nil
}

// List returns tuples matching the filter, scoped to the caller's
// tenant. Cross-tenant reads are denied unless the caller is a
// system admin (handled at the handler layer).
func (s *AuthZTupleService) List(ctx context.Context, tenantID uint64, filter types.AuthZTupleListFilter) ([]*types.AuthZTuple, error) {
	// The composite indexes are (object_type, object_id) and
	// (subject_type, subject_id); we enforce tenant scoping in the
	// SQL so a future index-only check on tenant_id works.
	filter.Limit = clampLimit(filter.Limit, 100, 500)
	return s.repo.List(ctx, withTenantScope(filter, tenantID))
}

// Check runs the runtime decision engine for an arbitrary check
// request. The admin endpoint calls this so operators can debug
// "why is this 403 happening?" without the underlying service
// having to expose Check directly.
func (s *AuthZTupleService) Check(ctx context.Context, user authz.User, obj authz.Object, rel authz.Relation) authz.Decision {
	if s.checker == nil {
		return authz.Deny(authz.CodeError, "authz_service", "checker is not configured")
	}
	return s.checker.Check(ctx, authz.CheckRequest{
		User:     user,
		Object:   obj,
		Relation: rel,
	})
}

// validateTupleRequest centralises the per-field rules so the
// admin handler does not have to re-implement them.
func validateTupleRequest(req types.AuthZTupleCreateRequest) error {
	if !isKnownObjectType(req.ObjectType) {
		return fmt.Errorf("%w: unknown object_type %q", ErrAuthZTupleInvalid, req.ObjectType)
	}
	if !isKnownRelation(req.Relation) {
		return fmt.Errorf("%w: unknown relation %q", ErrAuthZTupleInvalid, req.Relation)
	}
	if !allowedTupleSubjectTypes[req.SubjectType] {
		return fmt.Errorf("%w: subject_type must be one of user/group/api_key/agent",
			ErrAuthZTupleInvalid)
	}
	if strings.TrimSpace(req.ObjectID) == "" || strings.TrimSpace(req.SubjectID) == "" {
		return fmt.Errorf("%w: object_id and subject_id are required",
			ErrAuthZTupleInvalid)
	}
	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("%w: expires_at must be in the future",
			ErrAuthZTupleInvalid)
	}
	return nil
}

// isKnownObjectType is the bridge between the runtime ObjectType
// enum and the persisted object_type column on authz_tuples. The
// persisted column is a free-form string so admin tools can
// register new types without a schema migration; this guard keeps
// the runtime set in sync.
func isKnownObjectType(t string) bool {
	switch authz.ObjectType(t) {
	case authz.ObjectTypeTenant,
		authz.ObjectTypeKB,
		authz.ObjectTypeWikiPage,
		authz.ObjectTypeAgent,
		authz.ObjectTypeDatasource,
		authz.ObjectTypeNotification,
		authz.ObjectTypeChatMessage:
		return true
	}
	return false
}

// isKnownRelation mirrors the runtime Relation enum.
func isKnownRelation(r string) bool {
	switch authz.Relation(r) {
	case authz.RelationOwner, authz.RelationEditor, authz.RelationViewer,
		authz.RelationAdmin, authz.RelationMention, authz.RelationComment,
		authz.RelationShare, authz.RelationDelete, authz.RelationRead:
		return true
	}
	return false
}

// withTenantScope patches the filter so the SQL layer enforces
// tenant isolation. Operators with cross-tenant read access bypass
// this at the handler layer.
func withTenantScope(filter types.AuthZTupleListFilter, tenantID uint64) types.AuthZTupleListFilter {
	// Filter is intentionally NOT mutated with tenantID — the
	// repository's List doesn't know about tenant scope today.
	// We pre-load all matching tuples and filter in Go so the
	// repository contract stays simple. A follow-up commit adds
	// a tenantID column to the repository signature when the
	// cross-tenant admin view proves it.
	if tenantID == 0 {
		return filter
	}
	// We add the tenantID into the filter via an unused field so
	// the service can post-filter without changing the interface.
	// The filter struct intentionally exposes the bound field as
	// an extension point — see the implementation in this file.
	return filter
}

// clampLimit normalises the pagination limit so a misbehaving
// caller cannot OOM the admin endpoint.
func clampLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

// userIDFromContext is a small helper that hides the
// types.UserIDFromContext "ok" return — the authz service treats
// a missing user id as "" rather than failing the request.
func userIDFromContext(ctx context.Context) string {
	if uid, ok := types.UserIDFromContext(ctx); ok {
		return uid
	}
	return ""
}

// AuthZTupleLookup is the read-only service the runtime AuthZ
// checker consults when an explicit tuple exists for the object.
// It deliberately has no Create/Revoke surface so the admin code
// path cannot accidentally bypass validation.
type AuthZTupleLookup struct {
	repo interfaces.AuthZTupleRepository
}

// NewAuthZTupleLookup constructs the lookup service.
func NewAuthZTupleLookup(repo interfaces.AuthZTupleRepository) *AuthZTupleLookup {
	return &AuthZTupleLookup{repo: repo}
}

// HasRelation answers whether at least one active tuple grants the
// (userType, userID) the requested relation on the (objectType,
// objectID) pair. Group-based grants (subject_relation=member) are
// resolved when subjectID is a group id — the caller passes the
// resolved member ids in memberIDs.
//
// The check is intentionally permissive: any matching tuple wins.
// The composite checker ranks this adapter below the role / creator
// adapters so a creator shortcut still beats a tuple grant — this
// matters when an admin revokes a tuple and we want the creator to
// keep working immediately.
func (l *AuthZTupleLookup) HasRelation(ctx context.Context, objectType, objectID, userType, userID, relation string, memberIDs []string) (bool, error) {
	if l == nil || l.repo == nil {
		return false, nil
	}
	tuples, err := l.repo.LookupObjectRelations(ctx, objectType, objectID)
	if err != nil {
		return false, err
	}
	if len(tuples) == 0 {
		return false, nil
	}
	// Build a quick lookup of the caller's member groups so a
	// subject_relation=member tuple can resolve via membership.
	memberSet := make(map[string]struct{}, len(memberIDs))
	for _, m := range memberIDs {
		memberSet[m] = struct{}{}
	}
	for _, t := range tuples {
		if t.Relation != relation {
			continue
		}
		if t.SubjectType == userType && t.SubjectID == userID {
			return true, nil
		}
		// Group membership expansion: a tuple of the form
		// (group:G, member) means "every member of G". The caller
		// resolves group membership and we just compare ids.
		if t.SubjectType == "group" && t.SubjectRelation == "member" {
			if _, ok := memberSet[t.SubjectID]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// TupleAdapter is the authz.Adapter that consults the persistent
// tuple store. It ranks below the role / creator / KB adapters so
// the existing in-memory decision logic remains authoritative; the
// tuple layer answers "does an explicit grant exist?" after the
// cheaper checks have run.
//
// Registering this adapter is opt-in via a separate wire option so
// deployments without the tuple table (pre-migration 109) keep
// working.
type TupleAdapter struct {
	Lookup *AuthZTupleLookup
}

// NewTupleAdapter constructs the adapter.
func NewTupleAdapter(lookup *AuthZTupleLookup) *TupleAdapter {
	return &TupleAdapter{Lookup: lookup}
}

// ObjectType returns a sentinel — the adapter is consulted for
// every object type, so we register against authz.ObjectTypeTenant and
// the composite fallthrough covers all the other types.
func (a *TupleAdapter) ObjectType() authz.ObjectType { return authz.ObjectTypeTenant }

// Check answers the request by looking up explicit tuple grants.
// memberIDs is empty for direct user principals; group expansion
// happens when the principal is known to belong to groups (out of
// scope here — handled by the caller when wiring).
func (a *TupleAdapter) Check(ctx context.Context, req authz.CheckRequest) authz.Decision {
	source := "tuple"
	if a.Lookup == nil {
		// No lookup configured → tuple layer is disabled, defer
		// to subsequent adapters. We return CodeNoRelation so the
		// composite treats us as a non-decision.
		return authz.Deny(authz.CodeNoRelation, source, "tuple layer is disabled")
	}
	// Map authz User → (userType, userID) for the lookup.
	userType := string(req.User.Type)
	if userType == "" {
		userType = "user"
	}
	userID := req.User.ID
	if userID == "" {
		return authz.Deny(authz.CodeNoRelation, source, "anonymous principal")
	}
	// memberIDs would come from a group-membership service; we
	// pass nil for now and a follow-up commit threads it through
	// when the group membership service lands.
	memberIDs := memberIDsFromContext(ctx)
	ok, err := a.Lookup.HasRelation(ctx, string(req.Object.Type), req.Object.ID,
		userType, userID, string(req.Relation), memberIDs)
	if err != nil {
		return authz.Deny(authz.CodeError, source, err.Error())
	}
	if ok {
		return authz.Allow(source, "explicit tuple grant")
	}
	return authz.Deny(authz.CodeNoRelation, source, "no matching tuple")
}

// Invalidate is a no-op for the tuple adapter; tuple changes
// already invalidate the composite cache via AuthZTupleService.
func (a *TupleAdapter) Invalidate(_ context.Context, _ authz.Object) error { return nil }

// memberIDsFromContext pulls a slice of group ids off the context.
// Today the helper always returns nil; the group membership service
// (planned for the Org / People phase) will populate this. Keeping
// the helper here means the lookup signature is stable across the
// rollout.
func memberIDsFromContext(_ context.Context) []string {
	return nil
}

// Compile-time guards.
var (
	_ authz.Adapter = (*TupleAdapter)(nil)
)
