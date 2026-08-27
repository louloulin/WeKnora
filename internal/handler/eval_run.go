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

// Build #31 — Eval run handler.
//
// Maps HTTP routes for /api/v1/eval/runs:
//   - POST /api/v1/eval/runs             → StartRun      (Admin+)
//   - GET  /api/v1/eval/runs             → ListRuns      (Viewer+)
//   - GET  /api/v1/eval/runs/:id         → GetRun        (Viewer+)
//   - GET  /api/v1/eval/runs/:id/results → ListResults   (Viewer+)
//   - POST /api/v1/eval/runs/:id/cancel  → CancelRun     (Admin+)
//
// StartRun returns 202 with the new run id; the actual run happens
// in a goroutine so the HTTP request never blocks the chat pipeline.

type EvalRunHandler struct {
	svc interfaces.EvalRunService
}

func NewEvalRunHandler(svc interfaces.EvalRunService) *EvalRunHandler {
	return &EvalRunHandler{svc: svc}
}

// StartRunRequest is the POST body. JudgeModelID is optional.
type StartRunRequest struct {
	interfaces.EvalRunStartRequest
}

// StartRun kicks off a background run.
func (h *EvalRunHandler) StartRun(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval run service unavailable"})
		return
	}
	var req StartRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if req.DatasetID == "" || req.ChatModelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dataset_id and chat_model_id are required"})
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	req.CreatedBy = actorID

	runID, err := h.svc.StartRun(c.Request.Context(), &req.EvalRunStartRequest)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_run] start failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "start run failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"status": "accepted",
		"run_id": runID,
	})
}

// ListRuns newest-first, optional dataset_id filter.
func (h *EvalRunHandler) ListRuns(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval run service unavailable"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	datasetID := c.Query("dataset_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.ListRuns(c.Request.Context(), tenantID, datasetID, limit, offset)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_run] list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list runs failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  rows,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetRun loads one run by id.
func (h *EvalRunHandler) GetRun(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval run service unavailable"})
		return
	}
	id := c.Param("id")
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	run, err := h.svc.GetRun(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, service.ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		logger.Errorf(c.Request.Context(), "[eval_run] get failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get run failed"})
		return
	}
	c.JSON(http.StatusOK, run)
}

// ListResults returns per-QA rows for a run.
func (h *EvalRunHandler) ListResults(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval run service unavailable"})
		return
	}
	id := c.Param("id")
	rows, err := h.svc.ListResults(c.Request.Context(), id)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_run] list results failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list results failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"run_id": id,
		"items":  rows,
	})
}

// CancelRun marks a run canceled. A second cancel call on an
// already-terminal run returns 409.
func (h *EvalRunHandler) CancelRun(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval run service unavailable"})
		return
	}
	id := c.Param("id")
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	if err := h.svc.CancelRun(c.Request.Context(), tenantID, id); err != nil {
		switch {
		case errors.Is(err, service.ErrRunNotCancelable):
			c.JSON(http.StatusConflict, gin.H{"error": "run is already in a terminal state"})
		case errors.Is(err, service.ErrRunNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		default:
			logger.Errorf(c.Request.Context(), "[eval_run] cancel failed id=%s: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel run failed"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled", "run_id": id})
}