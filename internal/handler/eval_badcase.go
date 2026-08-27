package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// Build #31 — Eval badcase handler.
//
// Maps HTTP routes for /api/v1/eval/badcases:
//   - GET    /api/v1/eval/badcases                       → ListBadcases (Viewer+)
//   - POST   /api/v1/eval/badcases/promote               → Promote      (Admin+)
//   - POST   /api/v1/eval/badcases/:id/resolve           → Resolve      (Admin+)
//
// (FlagAuto is internal — only the EvalRunner calls it. There is no
// admin endpoint to manually flag; promotion is the public path.)

type EvalBadcaseHandler struct {
	svc interfaces.EvalBadcaseService
}

func NewEvalBadcaseHandler(svc interfaces.EvalBadcaseService) *EvalBadcaseHandler {
	return &EvalBadcaseHandler{svc: svc}
}

// ListBadcases returns rows newest-first.
func (h *EvalBadcaseHandler) ListBadcases(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval badcase service unavailable"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	filter := interfaces.EvalBadcaseFilter{
		Status:     c.Query("status"),
		Severity:   c.Query("severity"),
		FlagSource: c.Query("flag_source"),
	}
	filter.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "50"))
	filter.Offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.ListBadcases(c.Request.Context(), tenantID, filter)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_badcase] list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list badcases failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  rows,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// PromoteRequest is the POST body.
type PromoteRequest struct {
	RunID      string              `json:"run_id"`
	QID        int                 `json:"qid"`
	Severity   types.EvalSeverity  `json:"severity"`
	Notes      string              `json:"notes,omitempty"`
}

// Promote escalates an existing auto row, or creates a human-promote
// row when no auto row exists.
func (h *EvalBadcaseHandler) Promote(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval badcase service unavailable"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	if actorID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user id required"})
		return
	}
	var req PromoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if req.RunID == "" || req.QID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run_id and qid are required"})
		return
	}
	if req.Severity == "" {
		req.Severity = types.EvalSeverityMedium
	}
	row, err := h.svc.Promote(c.Request.Context(), tenantID, req.RunID, req.QID, req.Severity, req.Notes, actorID)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_badcase] promote failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "promote badcase failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

// ResolveRequest is the POST body.
type ResolveRequest struct {
	Notes string `json:"notes,omitempty"`
}

// Resolve stamps resolved_at and closes the row.
func (h *EvalBadcaseHandler) Resolve(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval badcase service unavailable"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	var req ResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		// Body is optional — only fail if JSON is malformed.
		if err.Error() != "EOF" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
			return
		}
	}
	if err := h.svc.Resolve(c.Request.Context(), tenantID, id, req.Notes); err != nil {
		if errors.Is(err, service.ErrBadcaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "badcase not found"})
			return
		}
		logger.Errorf(c.Request.Context(), "[eval_badcase] resolve failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resolve badcase failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "resolved", "id": id})
}