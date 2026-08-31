package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/connector"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// ConnectorHandler wires the v0.7.24 AI Connector framework REST API.
//
//   POST   /api/v1/connectors                          — register
//   GET    /api/v1/connectors                          — list
//   GET    /api/v1/connectors/:id                      — get
//   PATCH  /api/v1/connectors/:id                      — update
//   DELETE /api/v1/connectors/:id                      — disable
//   POST   /api/v1/connectors/:id/trigger              — sync now
//   GET    /api/v1/connectors/:id/jobs                 — jobs for connector
//   GET    /api/v1/connectors/jobs                     — all jobs for tenant
//
// Tenant and user are read from gin context, never from URL/body.
type ConnectorHandler struct {
	svc *connector.Service
}

// NewConnectorHandler wires the handler.
func NewConnectorHandler(svc *connector.Service) *ConnectorHandler {
	return &ConnectorHandler{svc: svc}
}

// Mount attaches the routes to an authenticated v1 group.
func (h *ConnectorHandler) Mount(rg *gin.RouterGroup) {
	g := rg.Group("/connectors")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/jobs", h.ListTenantJobs)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/trigger", h.Trigger)
	g.GET("/:id/jobs", h.ListJobs)
}

// --- connectors ---

type createConnectorBody struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Config          string `json:"config"` // raw JSON
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Enabled         *bool  `json:"enabled"`
}

func (h *ConnectorHandler) Create(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body createConnectorBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	conn := &types.IngestConnector{
		TenantID:        tenantID,
		Name:            body.Name,
		Kind:            types.ConnectorKind(body.Kind),
		Config:          body.Config,
		KnowledgeBaseID: body.KnowledgeBaseID,
		Enabled:         enabled,
		CreatedBy:       userID,
	}
	if err := h.svc.Create(c.Request.Context(), conn); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, conn)
}

func (h *ConnectorHandler) List(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.List(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connectors": rows, "total": total, "kinds": h.svc.Kinds()})
}

func (h *ConnectorHandler) Get(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	conn, err := h.svc.Get(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if conn == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connector not found"})
		return
	}
	c.JSON(http.StatusOK, conn)
}

func (h *ConnectorHandler) Update(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body createConnectorBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	conn := &types.IngestConnector{
		ID:              id,
		TenantID:        tenantID,
		Name:            body.Name,
		Kind:            types.ConnectorKind(body.Kind),
		Config:          body.Config,
		KnowledgeBaseID: body.KnowledgeBaseID,
		Enabled:         enabled,
	}
	if err := h.svc.Update(c.Request.Context(), conn); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, conn)
}

func (h *ConnectorHandler) Delete(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *ConnectorHandler) Trigger(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	job, err := h.svc.Trigger(c.Request.Context(), tenantID, id)
	if err != nil {
		// Even on Trigger error, we may have a job row for partial runs.
		if job != nil {
			c.JSON(http.StatusOK, job)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *ConnectorHandler) ListJobs(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.ListJobs(c.Request.Context(), tenantID, id, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": rows, "total": total})
}

func (h *ConnectorHandler) ListTenantJobs(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.ListTenantJobs(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": rows, "total": total})
}

// --- context helpers ---

func (h *ConnectorHandler) resolveCtx(c *gin.Context) (tenantID, userID string, ok bool) {
	tv, _ := c.Get("tenant_id")
	uv, _ := c.Get("user_id")
	tid, ok1 := toString(tv)
	uid, _ := toString(uv)
	if !ok1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return "", "", false
	}
	return tid, uid, true
}
