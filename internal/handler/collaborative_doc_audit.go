// Package handler - v0.7.30 collab_doc_audit_log REST endpoints.
//
// Audit log read-only API: list + summary. Writes are not exposed over HTTP
// — handlers and middleware call service.RecordAudit directly so the audit
// trail is never user-controllable.
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// CollabDocAuditHandler exposes the audit-log read API.
type CollabDocAuditHandler struct {
	svc *service.CollabDocService
}

// NewCollabDocAuditHandler wires the audit handler.
func NewCollabDocAuditHandler(svc *service.CollabDocService) *CollabDocAuditHandler {
	return &CollabDocAuditHandler{svc: svc}
}

// Register attaches the audit routes under the existing collab-docs router.
// Caller must pass the auth middleware (Bearer token → user id).
func (h *CollabDocAuditHandler) Register(rg *gin.RouterGroup) {
	// Per-doc timeline (powers the doc detail "history" panel).
	rg.GET("/collaborative-docs/:id/audit", h.ListForDoc)
	// Tenant-wide summary (powers the tenant "activity" tab).
	rg.GET("/collaborative-docs/audit/summary", h.Summary)
}

// ListForDoc returns paginated audit entries for a single doc.
// Query params:
//
//	actor   uint64 — narrow to a single actor
//	action  string — narrow to a single action enum
//	since   RFC3339 — lower bound on created_at
//	until   RFC3339 — upper bound on created_at
//	limit   int    — page size (default 100, max 500)
//	offset  int    — pagination offset
func (h *CollabDocAuditHandler) ListForDoc(c *gin.Context) {
	tenantID, userID, ok := collabAuditCaller(c)
	if !ok {
		return
	}
	docID := c.Param("id")
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doc id is required"})
		return
	}
	filter := types.ListCollabDocAuditFilter{DocID: docID}
	if v := c.Query("actor"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			filter.ActorUserID = n
		}
	}
	if v := c.Query("action"); v != "" {
		filter.Action = types.CollabDocAuditAction(v)
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
		}
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
	_ = userID // user identity is implicit in the auth context for the future RBAC check
	entries, err := h.svc.ListAudit(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

// Summary returns rolled-up audit counts for the tenant (or for a single
// doc when `?doc=...` is supplied). Powers the activity chart.
func (h *CollabDocAuditHandler) Summary(c *gin.Context) {
	tenantID, _, ok := collabAuditCaller(c)
	if !ok {
		return
	}
	filter := types.ListCollabDocAuditFilter{}
	if v := c.Query("doc"); v != "" {
		filter.DocID = v
	}
	if v := c.Query("action"); v != "" {
		filter.Action = types.CollabDocAuditAction(v)
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
		}
	}
	out, err := h.svc.AuditSummary(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// collabAuditCaller mirrors the comment helper. Both are tiny enough that
// keeping one per handler avoids an import cycle.
func collabAuditCaller(c *gin.Context) (uint64, uint64, bool) {
	tenantRaw, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return 0, 0, false
	}
	tenantID, ok := tenantRaw.(uint64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id invalid"})
		return 0, 0, false
	}
	userID := uint64(0)
	if u, exists := c.Get("user_id"); exists {
		if uid, ok := u.(uint64); ok {
			userID = uid
		}
	}
	return tenantID, userID, true
}
