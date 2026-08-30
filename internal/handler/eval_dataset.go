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

// Build #31 — Eval dataset handler.
//
// Maps HTTP routes for /api/v1/eval/datasets:
//   - POST   /api/v1/eval/datasets        → CreateDataset (Admin+)
//   - GET    /api/v1/eval/datasets        → ListDatasets   (Viewer+)
//   - GET    /api/v1/eval/datasets/:id    → GetDatasetByID (Viewer+)
//   - PUT    /api/v1/eval/datasets/:id    → UpdateDataset  (Admin+)
//   - DELETE /api/v1/eval/datasets/:id    → DeleteDataset  (Admin+)
//   - PUT    /api/v1/eval/datasets/:id/qa → ReplaceQAList  (Admin+)
//   - POST   /api/v1/eval/datasets/import → ImportJSON     (Admin+)
//
// All handlers return JSON; errors carry a typed code in the body so
// the frontend can surface the right toast (cap reached vs not found
// vs invalid body).

// EvalDatasetHandler is the typed HTTP surface for dataset CRUD.
type EvalDatasetHandler struct {
	svc interfaces.EvalDatasetService
}

// NewEvalDatasetHandler wires the handler with its single dependency.
// svc may be nil in test wiring — handlers degrade to 503 rather than
// panicking.
func NewEvalDatasetHandler(svc interfaces.EvalDatasetService) *EvalDatasetHandler {
	return &EvalDatasetHandler{svc: svc}
}

// CreateDatasetRequest is the POST body.
type CreateDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateDataset persists a new dataset and returns the id.
func (h *EvalDatasetHandler) CreateDataset(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
		return
	}
	var req CreateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())

	ds := &types.EvalDataset{
		ID:            "evaldataset-" + req.Name, // overwritten by service GenerateID
		TenantID:      tenantID,
		Name:          req.Name,
		Description:   req.Description,
		SchemaVersion: 1,
		CreatedBy:     actorID,
	}
	if err := h.svc.CreateDataset(c.Request.Context(), ds); err != nil {
		switch {
		case errors.Is(err, service.ErrDatasetCapReached):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			if isValidationErr(err) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			} else {
				logger.Errorf(c.Request.Context(), "[eval_dataset] create failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "create dataset failed"})
			}
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":          ds.ID,
		"name":        ds.Name,
		"description": ds.Description,
		"qa_count":    ds.QACount,
		"created_at":  ds.CreatedAt,
	})
}

// ListDatasets returns metadata rows for the tenant.
func (h *EvalDatasetHandler) ListDatasets(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.svc.ListDatasets(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		logger.Errorf(c.Request.Context(), "[eval_dataset] list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list datasets failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  rows,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetDatasetByID returns metadata + QA list.
func (h *EvalDatasetHandler) GetDatasetByID(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	ds, qas, err := h.svc.GetDatasetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrDatasetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset not found"})
			return
		}
		logger.Errorf(c.Request.Context(), "[eval_dataset] get failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get dataset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"dataset": ds,
		"qa":      qas,
	})
}

// UpdateDatasetRequest is the PUT body.
type UpdateDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateDataset mutates metadata (not QA — that goes through
// /datasets/:id/qa).
func (h *EvalDatasetHandler) UpdateDataset(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
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
	var req UpdateDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	ds := &types.EvalDataset{
		ID:          id,
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
	}
	if err := h.svc.UpdateDataset(c.Request.Context(), ds); err != nil {
		if errors.Is(err, service.ErrDatasetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset not found"})
			return
		}
		logger.Errorf(c.Request.Context(), "[eval_dataset] update failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update dataset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated", "id": id})
}

// DeleteDataset cascades to QA + runs via FK constraints.
func (h *EvalDatasetHandler) DeleteDataset(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
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
	if err := h.svc.DeleteDataset(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, service.ErrDatasetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "dataset not found"})
			return
		}
		logger.Errorf(c.Request.Context(), "[eval_dataset] delete failed id=%s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete dataset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

// ReplaceQAListRequest is the PUT body for /datasets/:id/qa.
type ReplaceQAListRequest struct {
	QA []types.EvalDatasetQA `json:"qa"`
}

// ReplaceQAList swaps the QA list in one transaction.
func (h *EvalDatasetHandler) ReplaceQAList(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var req ReplaceQAListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if err := h.svc.ReplaceQAList(c.Request.Context(), id, req.QA); err != nil {
		switch {
		case errors.Is(err, service.ErrQACapReached):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "[eval_dataset] replace QA failed id=%s: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "replace QA failed"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "replaced", "id": id, "qa_count": len(req.QA)})
}

// ImportJSONRequest is the POST body for /datasets/import.
type ImportJSONRequest struct {
	interfaces.EvalDatasetJSONPayload
}

// ImportJSON creates a dataset + QA rows in one call.
func (h *EvalDatasetHandler) ImportJSON(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "eval dataset service unavailable"})
		return
	}
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())

	var req ImportJSONRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}
	datasetID, err := h.svc.ImportJSON(c.Request.Context(), tenantID, actorID, &req.EvalDatasetJSONPayload)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDatasetCapReached),
			errors.Is(err, service.ErrQACapReached):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			if isValidationErr(err) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			} else {
				logger.Errorf(c.Request.Context(), "[eval_dataset] import failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "import dataset failed"})
			}
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": datasetID})
}

// isValidationErr returns true when the wrapped error is a
// user-input validation message ("X is required"). Used by handlers
// to map to 400 instead of 500.
func isValidationErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, m := range []string{"is required", "must be", "invalid"} {
		if contains(msg, m) {
			return true
		}
	}
	return false
}

// contains is a no-allocation strings.Contains shim so we don't have
// to import strings just for one predicate.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
