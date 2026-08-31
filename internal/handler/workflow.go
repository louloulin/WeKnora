// Package handler — v0.7.31 Build #37 AI Workflow Builder HTTP surface.
//
// Endpoints (all under /knowledgebase/:kb_id/workflows):
//
//	POST   /workflows                                  create workflow
//	GET    /workflows                                  list workflows in KB
//	GET    /workflows/:id                              get workflow
//	PATCH  /workflows/:id                              update workflow
//	DELETE /workflows/:id                              delete workflow
//	POST   /workflows/:id/run                          start a workflow run
//	GET    /workflows/:id/runs                         list runs
//	GET    /workflow-runs/:id                          get run with node runs
package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service/workflow"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// WorkflowHandler exposes the AI Workflow Builder REST surface.
type WorkflowHandler struct {
	svc *workflow.Service
}

// NewWorkflowHandler constructs a WorkflowHandler.
func NewWorkflowHandler(svc *workflow.Service) *WorkflowHandler {
	return &WorkflowHandler{svc: svc}
}

// Mount attaches all /workflows and /workflow-runs routes.
func (h *WorkflowHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/knowledgebase/:kb_id/workflows", h.Create)
	rg.GET("/knowledgebase/:kb_id/workflows", h.List)
	rg.GET("/knowledgebase/:kb_id/workflows/:id", h.Get)
	rg.PATCH("/knowledgebase/:kb_id/workflows/:id", h.Update)
	rg.DELETE("/knowledgebase/:kb_id/workflows/:id", h.Delete)
	rg.POST("/knowledgebase/:kb_id/workflows/:id/run", h.Run)
	rg.GET("/knowledgebase/:kb_id/workflows/:id/runs", h.ListRuns)
	rg.GET("/workflow-runs/:id", h.GetRun)
}

// Create persists a new workflow after DAG validation.
func (h *WorkflowHandler) Create(c *gin.Context) {
	var in types.WorkflowInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	w := &types.Workflow{
		Name:    in.Name,
		KBID:    c.Param("kb_id"),
		Nodes:   in.Nodes,
		Edges:   in.Edges,
		Enabled: in.Enabled,
	}
	if err := h.svc.Create(c, w); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, w)
}

// List returns every workflow in a knowledge base.
func (h *WorkflowHandler) List(c *gin.Context) {
	out, err := h.svc.ListByKB(c, uint64FromCtx(c), c.Param("kb_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// Get retrieves a workflow by ID.
func (h *WorkflowHandler) Get(c *gin.Context) {
	w, err := h.svc.Get(c, uint64FromCtx(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, w)
}

// Update mutates an existing workflow.
func (h *WorkflowHandler) Update(c *gin.Context) {
	var in types.WorkflowInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	w := &types.Workflow{
		ID:      c.Param("id"),
		Name:    in.Name,
		Nodes:   in.Nodes,
		Edges:   in.Edges,
		Enabled: in.Enabled,
	}
	if err := h.svc.Update(c, w); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, w)
}

// Delete removes a workflow.
func (h *WorkflowHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c, uint64FromCtx(c), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// Run starts a workflow run.
func (h *WorkflowHandler) Run(c *gin.Context) {
	var body map[string]any
	_ = c.ShouldBindJSON(&body)
	run, err := h.svc.Run(c, uint64FromCtx(c), c.Param("id"), "manual", body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}

// ListRuns returns the latest runs for a workflow.
func (h *WorkflowHandler) ListRuns(c *gin.Context) {
	out, err := h.svc.ListRuns(c, c.Param("id"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// GetRun returns a workflow run with its per-node runs.
func (h *WorkflowHandler) GetRun(c *gin.Context) {
	run, err := h.svc.GetRun(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, run)
}
