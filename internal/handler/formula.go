// Package handler provides REST endpoints for the v0.7.26 Build #31
// formula / rollup / linked-record engine. The handler is a thin
// shell: validation lives in the formula package, persistence
// lives in the database service.
package handler

import (
	"context"
	"errors"
	"net/http"

		"github.com/gin-gonic/gin"

	formulasvc "github.com/Tencent/WeKnora/internal/application/service/formula"
	"github.com/Tencent/WeKnora/internal/application/service/database"
	"github.com/Tencent/WeKnora/internal/formula"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// FormulaHandler wires the formula service to the HTTP surface.
type FormulaHandler struct {
	svc      *formulasvc.Service
	fieldSrc FieldSource
}

// FieldSource abstracts the database-side lookup needed by the
// SetFormula endpoint. The full database service implements it.
type FieldSource interface {
	GetFieldByID(ctx context.Context, tenantID uint64, fieldID string) (*types.DatabaseField, error)
}

// NewFormulaHandler constructs a FormulaHandler.
func NewFormulaHandler(svc *formulasvc.Service, src FieldSource) *FormulaHandler {
	return &FormulaHandler{svc: svc, fieldSrc: src}
}

// Register attaches the routes under the provided group.
func (h *FormulaHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/knowledgebase/:kb_id/databases/:database_id/formulas", h.Set)
	rg.POST("/knowledgebase/:kb_id/formulas/evaluate", h.Evaluate)
	rg.POST("/knowledgebase/:kb_id/formulas/deps", h.ExtractDeps)
}

// SetRequest is the body for Set.
type SetRequest struct {
	TenantID   uint64 `json:"tenant_id"`
	FieldID    string `json:"field_id"`
	Expression string `json:"expression"`
}

// Set installs or updates a formula expression on a field.
func (h *FormulaHandler) Set(c *gin.Context) {
	var req SetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	field, err := h.fieldSrc.GetFieldByID(c.Request.Context(), req.TenantID, req.FieldID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetFormula(c.Request.Context(), field, req.Expression); err != nil {
		if errors.Is(err, formulasvc.ErrInvalidExpression) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, formulasvc.ErrCyclicDependency) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		logger.Errorf(c.Request.Context(), "formula set: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"field_id": field.ID, "expression": req.Expression})
}

// EvaluateRequest is the body for Evaluate.
type EvaluateRequest struct {
	Expression string                     `json:"expression"`
	Fields     map[string]formula.Value   `json:"fields"`
}

// Evaluate computes the result of an expression with caller-provided
// field values. The endpoint does not persist anything.
func (h *FormulaHandler) Evaluate(c *gin.Context) {
	var req EvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.svc.EvaluateFormula(c.Request.Context(), req.Expression, req.Fields)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": v, "string": v.AsString()})
}

// ExtractDepsRequest is the body for ExtractDeps.
type ExtractDepsRequest struct {
	Expression string `json:"expression"`
}

// ExtractDeps returns the column names the expression depends on.
func (h *FormulaHandler) ExtractDeps(c *gin.Context) {
	var req ExtractDepsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deps := formula.ExtractFieldRefs(req.Expression)
	c.JSON(http.StatusOK, gin.H{"deps": deps})
}

// NewDatabaseFieldSource returns a FieldSource backed by the given
// database service. We scan the field ID by walking every database
// the service can see — fine for the small per-tenant workloads we
// target today and avoids a new repository method.
func NewDatabaseFieldSource(svc *database.Service) FieldSource {
	return &databaseFieldSource{svc: svc}
}

type databaseFieldSource struct {
	svc *database.Service
}

// GetFieldByID satisfies FieldSource.
func (a *databaseFieldSource) GetFieldByID(ctx context.Context, tenantID uint64, fieldID string) (*types.DatabaseField, error) {
	if a == nil || a.svc == nil {
		return nil, errors.New("formula: nil field source")
	}
	detail, err := a.svc.GetDetail(ctx, tenantID, "")
	if err != nil || detail == nil {
		return nil, err
	}
	for _, f := range detail.Fields {
		if f.ID == fieldID {
			return f, nil
		}
	}
	return nil, errors.New("formula: field not found")
}
