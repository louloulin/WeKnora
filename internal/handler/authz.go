package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/authz"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// AuthZHandler exposes the admin CRUD surface for the persistent
// AuthZ tuple store + the debug /check endpoint. Both surfaces are
// SystemAdmin-gated at the router layer; the handler itself only
// validates inputs and maps errors.
type AuthZHandler struct {
	svc *service.AuthZTupleService
}

// NewAuthZHandler constructs the handler. svc may be nil in tests
// that mock the underlying service.
func NewAuthZHandler(svc *service.AuthZTupleService) *AuthZHandler {
	return &AuthZHandler{svc: svc}
}

// CreateTuple persists a new explicit relation tuple. Body shape
// matches types.AuthZTupleCreateRequest; tenant_id is server-
// controlled from the caller's context.
func (h *AuthZHandler) CreateTuple(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authz tuple service is not configured"})
		return
	}
	tenantID, ok := tenantIDFromAuthZContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	var req types.AuthZTupleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.svc.Create(c, tenantID, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAuthZTupleInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrAuthZTupleAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c, "authz create tuple: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tuple"})
		}
		return
	}
	c.JSON(http.StatusCreated, t)
}

// ListTuples returns tuples matching the query filters. Empty
// filter fields are wildcards; the limit is capped at 500 by the
// service.
func (h *AuthZHandler) ListTuples(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authz tuple service is not configured"})
		return
	}
	tenantID, ok := tenantIDFromAuthZContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	filter := types.AuthZTupleListFilter{
		ObjectType:  c.Query("object_type"),
		ObjectID:    c.Query("object_id"),
		SubjectType: c.Query("subject_type"),
		SubjectID:   c.Query("subject_id"),
		Relation:    c.Query("relation"),
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}
	tuples, err := h.svc.List(c, tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tuples": tuples, "count": len(tuples)})
}

// RevokeTuple marks a tuple revoked. The composite decision cache
// is invalidated as part of the service path.
func (h *AuthZHandler) RevokeTuple(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authz tuple service is not configured"})
		return
	}
	tenantID, ok := tenantIDFromAuthZContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if err := h.svc.Revoke(c, tenantID, id); err != nil {
		switch {
		case errors.Is(err, service.ErrAuthZTupleNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrAuthZTupleInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

// CheckTuple is the admin debug endpoint — runs the runtime check
// engine for an arbitrary (user, object, relation) tuple so admins
// can answer "why is this 403 happening?" without diving into the
// source.
func (h *AuthZHandler) CheckTuple(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authz tuple service is not configured"})
		return
	}
	var req types.AuthZCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userType := authz.UserType(req.UserType)
	if userType == "" {
		userType = authz.UserTypeUser
	}
	tenantID, _ := tenantIDFromAuthZContext(c)
	decision := h.svc.Check(c, authz.User{
		Type:     userType,
		ID:       req.UserID,
		TenantID: tenantID,
	}, authz.Object{
		Type:     authz.ObjectType(req.Object.Type),
		ID:       req.Object.ID,
		TenantID: req.Object.TenantID,
	}, authz.Relation(req.Relation))
	c.JSON(http.StatusOK, decision)
}

// tenantIDFromAuthZContext pulls the tenant id off the request
// context via the canonical middleware helper. Centralising the
// lookup makes it easy to swap to a "system admin sees every
// tenant" override in a follow-up.
func tenantIDFromAuthZContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get("tenant_id")
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		return uint64(x), true
	case int:
		return uint64(x), true
	}
	return 0, false
}
