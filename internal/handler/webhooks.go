// Package handler — Build #46.x Webhook REST surface.
//
// Endpoints (all under /api/v1/webhooks, all tenant-scoped via the
// auth middleware):
//   POST   /webhooks                 — register a new subscription
//   GET    /webhooks                 — list tenant's subscriptions
//   GET    /webhooks/:id             — read one
//   PATCH  /webhooks/:id             — partial update (name/url/events/active/secret)
//   DELETE /webhooks/:id             — remove
//   GET    /webhooks/:id/deliveries  — recent delivery attempts
//
// All write paths funnel through the service layer (no direct repo
// access) so the dispatcher + delivery persistence + retry backoff
// live in one place.
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/webhooks"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// WebhookHandler exposes the webhook subscription REST surface.
type WebhookHandler struct {
	svc interfaces.WebhookService // service interface — see service/webhooks package
}

// NewWebhookHandler wires the handler. Accepts the concrete *webhooks.WebhookService
// because the service exposes one public method set, but the field is
// typed against the interface so a future mock is drop-in.
func NewWebhookHandler(svc *webhooks.WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

// Mount registers the routes on the given router group.
func (h *WebhookHandler) Mount(rg *gin.RouterGroup) {
	if h == nil || h.svc == nil {
		return
	}
	g := rg.Group("/webhooks")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.GET("/:id/deliveries", h.ListDeliveries)
}

func (h *WebhookHandler) Create(c *gin.Context) {
	var req types.CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	userID := userIDFromCtx(c.Request.Context())
	hook, err := h.svc.Create(c.Request.Context(), tenantID, userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, hook)
}

func (h *WebhookHandler) List(c *gin.Context) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	filter := types.ListWebhooksFilter{
		ActiveOnly: c.Query("active_only") == "true",
	}
	rows, err := h.svc.List(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": len(rows)})
}

func (h *WebhookHandler) Get(c *gin.Context) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	hook, err := h.svc.Get(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if hook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}
	c.JSON(http.StatusOK, hook)
}

func (h *WebhookHandler) Update(c *gin.Context) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	var patch types.UpdateWebhookRequest
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hook, err := h.svc.Update(c.Request.Context(), tenantID, c.Param("id"), patch)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if hook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}
	c.JSON(http.StatusOK, hook)
}

func (h *WebhookHandler) Delete(c *gin.Context) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if err := h.svc.Delete(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		logger.Warnf(c.Request.Context(), "[webhook] delete failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	// Tenant-scoped fetch (admin / debugging).
	rows, err := h.svc.ListDeliveriesByTenant(c.Request.Context(), tenantID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": len(rows)})
}


// userIDFromCtx converts the context user id (string from JWT) to uint64.
// Returns 0 for unparseable values — matches the contract used by other
// collab handlers.
func userIDFromCtx(ctx context.Context) uint64 {
	s, _ := types.UserIDFromContext(ctx)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
