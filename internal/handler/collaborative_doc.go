// Package handler — v0.7.25 collaborative_docs HTTP handlers.
//
// REST surface (mirrors the connector / wiki APIs):
//   POST   /api/v1/collaborative-docs                — create
//   GET    /api/v1/collaborative-docs                — list (filter kb_id, kind)
//   GET    /api/v1/collaborative-docs/:id            — metadata
//   PATCH  /api/v1/collaborative-docs/:id            — rename / visibility
//   POST   /api/v1/collaborative-docs/:id/archive    — soft delete
//   DELETE /api/v1/collaborative-docs/:id            — hard delete (cascade)
//   GET    /api/v1/collaborative-docs/:id/presence   — live presence list
//   GET    /api/v1/collaborative-docs/:id/export     — markdown export
//
// WebSocket surface:
//   GET    /api/v1/collaborative-docs/:id/realtime   — Yjs y-websocket upgrade
package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// CollabDocHandler is the REST surface for collaborative documents.
type CollabDocHandler struct {
	svc *service.CollabDocService
}

// NewCollabDocHandler wires the REST handler.
func NewCollabDocHandler(svc *service.CollabDocService) *CollabDocHandler {
	return &CollabDocHandler{svc: svc}
}

// Mount attaches the REST routes onto an authenticated v1 group.
func (h *CollabDocHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/collaborative-docs", h.Create)
	rg.GET("/collaborative-docs", h.List)
	rg.GET("/collaborative-docs/:id", h.Get)
	rg.PATCH("/collaborative-docs/:id", h.Update)
	rg.POST("/collaborative-docs/:id/archive", h.Archive)
	rg.DELETE("/collaborative-docs/:id", h.Delete)
	rg.GET("/collaborative-docs/:id/presence", h.Presence)
	rg.GET("/collaborative-docs/:id/export", h.Export)
}

func (h *CollabDocHandler) tenantAndUser(c *gin.Context) (uint64, uint64, bool) {
	t := c.GetUint64(types.TenantIDContextKey.String())
	u := c.GetUint64(types.UserIDContextKey.String())
	if t == 0 || u == 0 {
		c.Error(errors.NewUnauthorizedError("missing tenant/user context"))
		return 0, 0, false
	}
	return t, u, true
}

// Create handles POST /collaborative-docs.
func (h *CollabDocHandler) Create(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.CreateCollaborativeDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if req.DocKind != "" {
		if _, err := types.ParseCollaborativeDocKind(string(req.DocKind)); err != nil {
			c.Error(errors.NewBadRequestError("invalid doc_kind"))
			return
		}
	}
	d, err := h.svc.CreateDoc(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	// v0.7.30 — audit
	h.svc.RecordAudit(c.Request.Context(), types.RecordAuditRequest{
		TenantID: tenantID, DocID: d.ID, ActorUserID: userID,
		Action: types.AuditActionCreate, Target: string(req.DocKind),
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": d})
}

// List handles GET /collaborative-docs.
func (h *CollabDocHandler) List(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	filter := types.ListCollaborativeDocsFilter{
		KBID:    c.Query("kb_id"),
		DocKind: types.CollaborativeDocKind(c.Query("doc_kind")),
		Limit:   parseIntDefault(c.Query("limit"), 50),
		Offset:  parseIntDefault(c.Query("offset"), 0),
	}
	if c.Query("archived") == "true" {
		filter.Archived = true
	}
	if filter.DocKind != "" {
		if _, err := types.ParseCollaborativeDocKind(string(filter.DocKind)); err != nil {
			c.Error(errors.NewBadRequestError("invalid doc_kind filter"))
			return
		}
	}
	items, total, err := h.svc.ListDocs(c.Request.Context(), tenantID, userID, filter)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"total":   total,
	})
}

// Get handles GET /collaborative-docs/:id.
func (h *CollabDocHandler) Get(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	d, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		c.Error(errors.NewNotFoundError(err.Error()))
		return
	}
	if d == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": d})
}

// Update handles PATCH /collaborative-docs/:id.
func (h *CollabDocHandler) Update(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.UpdateCollaborativeDocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	id := c.Param("id")
	d, err := h.svc.UpdateDoc(c.Request.Context(), tenantID, userID, id, req)
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if d == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": d})
}

// Archive handles POST /collaborative-docs/:id/archive.
func (h *CollabDocHandler) Archive(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := h.svc.ArchiveDoc(c.Request.Context(), tenantID, userID, id); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	// v0.7.30 — audit
	h.svc.RecordAudit(c.Request.Context(), types.RecordAuditRequest{
		TenantID: tenantID, DocID: id, ActorUserID: userID,
		Action: types.AuditActionArchive,
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Delete handles DELETE /collaborative-docs/:id.
func (h *CollabDocHandler) Delete(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := h.svc.DeleteDoc(c.Request.Context(), tenantID, userID, id); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	// v0.7.30 — audit
	h.svc.RecordAudit(c.Request.Context(), types.RecordAuditRequest{
		TenantID: tenantID, DocID: id, ActorUserID: userID,
		Action: types.AuditActionDelete,
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	c.Status(http.StatusNoContent)
}

// Presence handles GET /collaborative-docs/:id/presence.
func (h *CollabDocHandler) Presence(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	sessions, err := h.svc.ListSessions(c.Request.Context(), tenantID, id)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sessions})
}

// Export handles GET /collaborative-docs/:id/export.
//
// The MVP returns a short Markdown scaffold (title + doc_kind marker + the
// latest snapshot's mtime). Full TipTap→Markdown, Univer→Markdown table,
// and PptxGenJS→Markdown-per-slide converters land in the v0.7.26 follow-up;
// the route shape is finalized now so the client can integrate against a
// stable contract.
func (h *CollabDocHandler) Export(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	d, err := h.svc.GetDoc(c.Request.Context(), tenantID, userID, id)
	if err != nil || d == nil {
		c.Error(errors.NewNotFoundError("collab doc not found"))
		return
	}
	state, err := h.svc.LoadDocState(c.Request.Context(), tenantID, id)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	logger.Infof(c.Request.Context(), "[CollabDoc] export doc=%s kind=%s state_bytes=%d", id, d.DocKind, len(state))
	md := []byte("# " + d.Title + "\n\n<!-- collaborative_docs export -- kind: " + string(d.DocKind) + " -->\n\n")
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", md)
}

// parseIntDefault parses a non-negative integer from a query string with a fallback.
func parseIntDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}
