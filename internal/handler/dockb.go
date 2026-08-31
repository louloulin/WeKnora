package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/dockb"
	"github.com/gin-gonic/gin"
)

// DockbHandler wires the v0.7.23 Doc ↔ KB AI Bridge endpoints.
// Tenant and user are read from the gin context by the upstream
// auth middleware — never from URL/body.
//
//   /api/v1/dockb/summaries/:knowledge_id       — list summaries
//   /api/v1/dockb/chunks/:knowledge_id/:chunk_id — get or upsert summary
//   /api/v1/dockb/summaries/:id                — delete one summary
type DockbHandler struct {
	summariser *dockb.SummariserService
}

// NewDockbHandler wires the handler.
func NewDockbHandler(s *dockb.SummariserService) *DockbHandler {
	return &DockbHandler{summariser: s}
}

// Mount attaches the routes onto an authenticated v1 group.
func (h *DockbHandler) Mount(rg *gin.RouterGroup) {
	dg := rg.Group("/dockb")
	dg.GET("/summaries/:knowledge_id", h.ListSummaries)
	dg.GET("/chunks/:knowledge_id/:chunk_id", h.GetSummary)
	dg.PUT("/chunks/:knowledge_id/:chunk_id", h.UpsertSummary)
	dg.DELETE("/summaries/:id", h.DeleteSummary)
}

// --- Doc KB Summaries ---

// resolveCtx reads the tenant and user from the gin context. Returns
// ok=false (and writes a 401) when either is missing — the handler
// short-circuits on ok=false.
func (h *DockbHandler) resolveCtx(c *gin.Context) (string, string, bool) {
	tenantID := c.GetString("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth context"})
		return "", "", false
	}
	return tenantID, userID, true
}

func (h *DockbHandler) ListSummaries(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	rows, err := h.summariser.ListByKnowledge(c.Request.Context(), tenantID, knowledgeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summaries": rows, "total": len(rows)})
}

func (h *DockbHandler) GetSummary(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	chunkID := c.Param("chunk_id")
	row, err := h.summariser.GetByChunk(c.Request.Context(), tenantID, knowledgeID, chunkID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "summary not found"})
		return
	}
	c.JSON(http.StatusOK, row)
}

type upsertSummaryBody struct {
	Text      string `json:"text"`
	ModelName string `json:"model_name"`
}

func (h *DockbHandler) UpsertSummary(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	chunkID := c.Param("chunk_id")
	var body upsertSummaryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := h.summariser.SummariseChunk(
		c.Request.Context(), tenantID, knowledgeID, chunkID, body.Text, body.ModelName,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = userID // reserved for future audit wiring
	c.JSON(http.StatusOK, row)
}

func (h *DockbHandler) DeleteSummary(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.summariser.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
