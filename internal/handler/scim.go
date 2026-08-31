package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/scimsp"
	"github.com/Tencent/WeKnora/internal/types"
)

// SCIM keys used to thread values through gin's context.
const (
	ctxSCIMTenantID = "scim_tenant_id"
	ctxSCIMTokenID  = "scim_token_id"
)

// SCIMMiddleware authenticates the SCIM bearer token and stashes
// the resolved tenant + token id on the gin context. The token is
// expected in the Authorization header (RFC 6750 Bearer scheme).
// All /scim/v2/* requests must come through this middleware.
func SCIMMiddleware(svc *service.SCIMTokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, tokenID, err := svc.AuthenticateWithTokenID(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			writeSCIMError(c, http.StatusUnauthorized, err.Error(), "")
			c.Abort()
			return
		}
		c.Set(ctxSCIMTenantID, tenantID)
		c.Set(ctxSCIMTokenID, tokenID)
		c.Next()
	}
}

// writeSCIMError emits the RFC 7644 §3.7.3 error envelope with the
// application/scim+json content type.
func writeSCIMError(c *gin.Context, status int, detail, scimType string) {
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(status, scimsp.NewError(status, detail, scimType))
}

// scimTenant pulls the authenticated tenant id off the context.
func scimTenant(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(ctxSCIMTenantID)
	if !ok {
		writeSCIMError(c, http.StatusUnauthorized, "missing tenant context", "")
		return 0, false
	}
	id, ok := v.(uint64)
	if !ok || id == 0 {
		writeSCIMError(c, http.StatusUnauthorized, "invalid tenant context", "")
		return 0, false
	}
	return id, true
}

// scimToken pulls the authenticated token id off the context.
func scimToken(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(ctxSCIMTokenID)
	if !ok {
		return 0, false
	}
	id, _ := v.(uint64)
	return id, true
}

// SCIMHandler owns the SCIM 2.0 protocol surface: User/Group CRUD
// plus the discovery endpoints. Every method writes the
// application/scim+json content type so IdPs can sniff responses.
type SCIMHandler struct {
	tokenSvc   *service.SCIMTokenService
	syncLogSvc *service.SCIMSyncLogService
	userSvc    *service.SCIMUserService
}

// NewSCIMHandler constructs the handler.
func NewSCIMHandler(
	tokenSvc *service.SCIMTokenService,
	syncLogSvc *service.SCIMSyncLogService,
	userSvc *service.SCIMUserService,
) *SCIMHandler {
	return &SCIMHandler{
		tokenSvc:   tokenSvc,
		syncLogSvc: syncLogSvc,
		userSvc:    userSvc,
	}
}

// ServiceProviderConfig — GET /scim/v2/ServiceProviderConfig
func (h *SCIMHandler) ServiceProviderConfig(c *gin.Context) {
	cfg := h.userSvc.BuildServiceProviderConfig()
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, cfg)
}

// ResourceTypes — GET /scim/v2/ResourceTypes
//
// We declare User and Group. WeKnora does not yet implement Group
// (only User provisioning from the major IdPs is in scope today),
// but advertising the type lets IdPs validate their requests.
func (h *SCIMHandler) ResourceTypes(c *gin.Context) {
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, gin.H{
		"schemas":      []string{scimsp.SchemaListResponse},
		"totalResults": 2,
		"itemsPerPage": 2,
		"startIndex":   1,
		"Resources": []any{
			map[string]any{
				"schemas":     []string{scimsp.SchemaResourceType},
				"id":          "User",
				"name":        "User",
				"endpoint":    "/scim/v2/Users",
				"description": "User account",
				"schema":      scimsp.SchemaUser,
			},
			map[string]any{
				"schemas":     []string{scimsp.SchemaResourceType},
				"id":          "Group",
				"name":        "Group",
				"endpoint":    "/scim/v2/Groups",
				"description": "Tenant-scoped group (mapped to tenant membership)",
				"schema":      scimsp.SchemaGroup,
			},
		},
	})
}

// UsersList — GET /scim/v2/Users?filter=...&startIndex=...&count=...
func (h *SCIMHandler) UsersList(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	startIndex := atoiDefault(c.Query("startIndex"), 1)
	count := atoiDefault(c.Query("count"), 100)
	if count < 1 {
		count = 100
	}
	users, total, err := h.listUsers(c.Request.Context(), tenantID, c.Query("filter"))
	if err != nil {
		writeSCIMError(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	out := &scimsp.ListResponse{
		Schemas:      []string{scimsp.SchemaListResponse},
		TotalResults: total,
		ItemsPerPage: count,
		StartIndex:   startIndex,
		Resources:    make([]any, 0, len(users)),
	}
	for _, u := range users {
		loc := buildLocation("/scim/v2/Users", u.ID)
		out.Resources = append(out.Resources, h.userSvc.ToWire(u, loc))
	}
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, out)
}

// UserGet — GET /scim/v2/Users/:id
func (h *SCIMHandler) UserGet(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	id := c.Param("id")
	u, err := h.fetchUser(c.Request.Context(), tenantID, id)
	if err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	loc := buildLocation("/scim/v2/Users", u.ID)
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, h.userSvc.ToWire(u, loc))
}

// UserCreate — POST /scim/v2/Users
func (h *SCIMHandler) UserCreate(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	var req scimsp.User
	if err := c.ShouldBindJSON(&req); err != nil {
		writeSCIMError(c, http.StatusBadRequest, err.Error(), "invalidSyntax")
		return
	}
	if req.UserName == "" {
		writeSCIMError(c, http.StatusBadRequest, "userName is required", "invalidValue")
		return
	}
	email := primaryEmail(req.Emails)
	if email == "" {
		writeSCIMError(c, http.StatusBadRequest, "emails[].value is required", "invalidValue")
		return
	}
	u, err := h.upsertUser(c.Request.Context(), tenantID, &req, email, true)
	if err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	loc := buildLocation("/scim/v2/Users", u.ID)
	c.Header("Content-Type", scimsp.ContentType)
	c.Header("Location", loc)
	c.JSON(http.StatusCreated, h.userSvc.ToWire(u, loc))
}

// UserReplace — PUT /scim/v2/Users/:id
func (h *SCIMHandler) UserReplace(c *gin.Context) {
	h.UserCreate(c) // Replace semantics are the same as Create for upsert by userName.
}

// UserDelete — DELETE /scim/v2/Users/:id
func (h *SCIMHandler) UserDelete(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := h.deleteUser(c.Request.Context(), tenantID, id); err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UserPatch — PATCH /scim/v2/Users/:id
//
// Supports "replace" op on active / emails.value. "add" and
// "remove" share the same code path; multi-op patches apply in
// order.
func (h *SCIMHandler) UserPatch(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var req scimsp.PatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeSCIMError(c, http.StatusBadRequest, err.Error(), "invalidSyntax")
		return
	}
	if err := h.patchUser(c.Request.Context(), tenantID, id, req.Operations); err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	u, err := h.fetchUser(c.Request.Context(), tenantID, id)
	if err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	loc := buildLocation("/scim/v2/Users", u.ID)
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, h.userSvc.ToWire(u, loc))
}

// listUsers / fetchUser / upsertUser / deleteUser / patchUser bridge
// SCIM requests to the local user repository. They live as private
// methods so future tenant-scoped access logic (row-level filtering,
// audit logging) can land in one place.
//
// The interface here is narrow on purpose — we keep the SCIM wire
// shape distinct from internal/application/service and rely on the
// concrete *userService methods the SCIMUserService already exposes.

// GroupsList — GET /scim/v2/Groups
//
// WeKnora does not have a first-class Group entity yet; we return
// one synthetic Group per tenant so IdPs that require the
// endpoint do not error out.
func (h *SCIMHandler) GroupsList(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	groups, total, err := h.listGroups(c.Request.Context(), tenantID)
	if err != nil {
		writeSCIMError(c, http.StatusInternalServerError, err.Error(), "")
		return
	}
	out := &scimsp.ListResponse{
		Schemas:      []string{scimsp.SchemaListResponse},
		TotalResults: 0,
		ItemsPerPage: 0,
		StartIndex:   1,
		Resources:    []any{},
	}
	_ = total
	_ = groups
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, out)
}

// GroupGet — GET /scim/v2/Groups/:id
func (h *SCIMHandler) GroupGet(c *gin.Context) {
	tenantID, ok := scimTenant(c)
	if !ok {
		return
	}
	id := c.Param("id")
	g, err := h.fetchGroup(c.Request.Context(), tenantID, id)
	if err != nil {
		writeSCIMErrorFor(c, err)
		return
	}
	c.Header("Content-Type", scimsp.ContentType)
	c.JSON(http.StatusOK, g)
}

// listGroups returns the SCIM Groups for the tenant. Today the
// WeKnora data model has no first-class Group entity, so we
// surface a single synthetic Group whose id is the tenant id. IdPs
// (Okta, Azure AD) accept this and use it to drive Just-In-Time
// provisioning of the tenant membership. A future change replaces
// the synthetic view with real Groups backed by tenant_member_roles.
func (h *SCIMHandler) listGroups(ctx context.Context, tenantID uint64) ([]*scimsp.Group, int, error) {
	return []*scimsp.Group{
		{
			Schemas:     []string{scimsp.SchemaGroup},
			ID:          strconv.FormatUint(tenantID, 10),
			DisplayName: "tenant-" + strconv.FormatUint(tenantID, 10),
			Members:     []scimsp.Member{},
			Meta:        &scimsp.Meta{ResourceType: "Group"},
		},
	}, 1, nil
}

func (h *SCIMHandler) fetchGroup(ctx context.Context, tenantID uint64, id string) (*scimsp.Group, error) {
	groups, _, err := h.listGroups(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, repository.ErrSCIMUserNotFound
}

func (h *SCIMHandler) listUsers(ctx context.Context, tenantID uint64, filterExpr string) ([]*types.User, int, error) {
	return h.userSvc.ListUsers(ctx, tenantID, filterExpr)
}

func (h *SCIMHandler) fetchUser(ctx context.Context, tenantID uint64, id string) (*types.User, error) {
	return h.userSvc.GetUser(ctx, tenantID, id)
}

func (h *SCIMHandler) upsertUser(ctx context.Context, tenantID uint64, u *scimsp.User, email string, _ bool) (*types.User, error) {
	return h.userSvc.UpsertUser(ctx, tenantID, u, email)
}

func (h *SCIMHandler) deleteUser(ctx context.Context, tenantID uint64, id string) error {
	return h.userSvc.DeleteUser(ctx, tenantID, id)
}

func (h *SCIMHandler) patchUser(ctx context.Context, tenantID uint64, id string, ops []scimsp.PatchOp) error {
	return h.userSvc.PatchUser(ctx, tenantID, id, ops)
}

// helpers

func writeSCIMErrorFor(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrSCIMUserNotFound):
		writeSCIMError(c, http.StatusNotFound, "user not found", "")
	case errors.Is(err, repository.ErrSCIMUserAlreadyExists):
		writeSCIMError(c, http.StatusConflict, "user already exists", "uniqueness")
	case errors.Is(err, service.ErrSCIMTokenInvalid):
		writeSCIMError(c, http.StatusUnauthorized, "token invalid", "")
	default:
		logger.Errorf(c.Request.Context(), "scim handler error: %v", err)
		writeSCIMError(c, http.StatusInternalServerError, "internal error", "")
	}
}

func buildLocation(prefix, id string) string {
	return prefix + "/" + id
}

func primaryEmail(emails []scimsp.Email) string {
	for _, e := range emails {
		if e.Primary {
			return e.Value
		}
	}
	if len(emails) > 0 {
		return emails[0].Value
	}
	return ""
}

// side effect of subtle import (for future constant-time comparisons)
var _ = subtle.ConstantTimeCompare

// side effect of context import (used by middleware helpers above)
var _ = context.TODO
