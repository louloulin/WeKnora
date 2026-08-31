package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// ConditionalAccessHandler exposes the conditional access policy CRUD
// surface to admins. The login-flow evaluator is invoked directly by
// the auth handler; this handler only deals with policy management.
//
// Route surface (registered via routes_conditional_access.go):
//
//	GET    /api/v1/conditional-access/policies
//	POST   /api/v1/conditional-access/policies
//	GET    /api/v1/conditional-access/policies/:id
//	PUT    /api/v1/conditional-access/policies/:id
//	DELETE /api/v1/conditional-access/policies/:id
//
// All endpoints are tenant-scoped — the handler reads TenantID from
// the gin context (set by the auth middleware).
type ConditionalAccessHandler struct {
	svc *service.ConditionalAccessService
}

// NewConditionalAccessHandler is the DI constructor.
func NewConditionalAccessHandler(svc *service.ConditionalAccessService) *ConditionalAccessHandler {
	return &ConditionalAccessHandler{svc: svc}
}

// ConditionalAccessPolicyRequest is the wire body for POST / PUT. The
// Conditions field is a free-form PolicyConditions JSON object; we
// trust it here because the service validates each populated field
// when MatchConditions runs.
type ConditionalAccessPolicyRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Conditions  types.PolicyConditions `json:"conditions"`
	Action      types.PolicyAction     `json:"action" binding:"required"`
	Priority    int                    `json:"priority"`
	Description string                 `json:"description"`
	Enabled     *bool                  `json:"enabled"`
}

// ListPolicies handles the admin list endpoint.
func (h *ConditionalAccessHandler) ListPolicies(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := h.svc.ListPolicies(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		writeConditionalAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total})
}

// CreatePolicy handles POST.
func (h *ConditionalAccessHandler) CreatePolicy(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	var req ConditionalAccessPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := c.GetString("user_id")
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	policy := &types.ConditionalAccessPolicy{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Conditions:  req.Conditions,
		Action:      req.Action,
		Priority:    req.Priority,
		Enabled:     enabled,
		CreatedBy:   userID,
	}
	out, err := h.svc.CreatePolicy(c.Request.Context(), policy)
	if err != nil {
		writeConditionalAccessError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// GetPolicy handles GET-by-id.
func (h *ConditionalAccessHandler) GetPolicy(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	row, err := h.svc.GetPolicy(c.Request.Context(), tenantID, id)
	if err != nil {
		writeConditionalAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

// UpdatePolicy handles PUT.
func (h *ConditionalAccessHandler) UpdatePolicy(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req ConditionalAccessPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	policy := &types.ConditionalAccessPolicy{
		ID:          id,
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Conditions:  req.Conditions,
		Action:      req.Action,
		Priority:    req.Priority,
		Enabled:     enabled,
	}
	out, err := h.svc.UpdatePolicy(c.Request.Context(), policy)
	if err != nil {
		writeConditionalAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// DeletePolicy handles DELETE. Idempotent — 204 either way.
func (h *ConditionalAccessHandler) DeletePolicy(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeletePolicy(c.Request.Context(), tenantID, id); err != nil {
		writeConditionalAccessError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writeConditionalAccessError is the single place that maps service /
// repository sentinels to HTTP status codes.
//
// Sentinel → status:
//   ErrConditionalAccessPolicyNotFound        → 404
//   ErrConditionalAccessPolicyExists          → 409
//   ErrConditionalAccessInvalidRequest        → 400
//   service.ErrConditionalAccessInvalidRequest → 400
//   anything else                             → 500
func writeConditionalAccessError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrConditionalAccessPolicyNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrConditionalAccessPolicyExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrConditionalAccessInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
