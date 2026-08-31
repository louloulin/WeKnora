// Package handler — automation / button REST endpoints (Build #33).
// The handler is intentionally thin: validation lives in the
// automation package, persistence lives in the repo.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	autosvc "github.com/Tencent/WeKnora/internal/application/service/automation"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// AutomationHandler wires the automation service to HTTP.
type AutomationHandler struct {
	svc *autosvc.Service
}

// NewAutomationHandler constructs an AutomationHandler.
func NewAutomationHandler(svc *autosvc.Service) *AutomationHandler {
	return &AutomationHandler{svc: svc}
}

// Register attaches the routes under rg. Webhook triggers post to
// a separate endpoint that does not require the caller to be a
// signed-in user.
func (h *AutomationHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/knowledgebase/:kb_id/databases/:database_id/automations", h.Create)
	rg.GET("/knowledgebase/:kb_id/automations/:id", h.Get)
	rg.PUT("/knowledgebase/:kb_id/automations/:id", h.Update)
	rg.DELETE("/knowledgebase/:kb_id/automations/:id", h.Delete)
	rg.GET("/knowledgebase/:kb_id/databases/:database_id/automations", h.List)
	rg.POST("/knowledgebase/:kb_id/automations/:id/run", h.Run)
	rg.GET("/knowledgebase/:kb_id/automations/:id/runs", h.ListRuns)
	rg.POST("/webhooks/automations/:id", h.Webhook)
}

// Create persists a new automation. The request body is the
// Automation JSON.
func (h *AutomationHandler) Create(c *gin.Context) {
	var a types.Automation
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Create(c.Request.Context(), &a); err != nil {
		status := http.StatusBadRequest
		if err == autosvc.ErrNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

// Get returns the automation by id.
func (h *AutomationHandler) Get(c *gin.Context) {
	id := c.Param("id")
	a, err := h.svc.Get(c.Request.Context(), 0, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// Update mutates an existing automation.
func (h *AutomationHandler) Update(c *gin.Context) {
	var a types.Automation
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a.ID = c.Param("id")
	if err := h.svc.Update(c.Request.Context(), &a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// Delete soft-deletes an automation.
func (h *AutomationHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.Delete(c.Request.Context(), 0, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// List lists automations for a database.
func (h *AutomationHandler) List(c *gin.Context) {
	databaseID := c.Param("database_id")
	out, err := h.svc.ListByDatabase(c.Request.Context(), 0, databaseID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"automations": out})
}

// Run executes an automation synchronously and returns the run.
func (h *AutomationHandler) Run(c *gin.Context) {
	id := c.Param("id")
	a, err := h.svc.Get(c.Request.Context(), 0, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var inputs types.AutomationRunInputs
	_ = c.ShouldBindJSON(&inputs)
	run, runErr := h.svc.Run(c.Request.Context(), a, &inputs)
	if runErr != nil {
		logger.Warnf(c.Request.Context(), "automation run: %v", runErr)
	}
	status := http.StatusOK
	if runErr != nil {
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, run)
}

// ListRuns returns the most recent runs for an automation.
func (h *AutomationHandler) ListRuns(c *gin.Context) {
	// Stub: in a real implementation we'd call svc.repo.ListRunsByAutomation.
	// For now we just return an empty list so the route is wired.
	_ = c.Param("id")
	c.JSON(http.StatusOK, gin.H{"runs": []any{}})
}

// Webhook is the inbound trigger for AutomationTriggerWebhook
// automations. The token comes from the URL query string so a
// caller does not need to know the secret header conventions of
// the rest of the system.
func (h *AutomationHandler) Webhook(c *gin.Context) {
	id := c.Param("id")
	a, err := h.svc.Get(c.Request.Context(), 0, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if a.TriggerType != types.AutomationTriggerWebhook {
		c.JSON(http.StatusBadRequest, gin.H{"error": "automation is not webhook-triggered"})
		return
	}
	cfg, _ := types.ParseTriggerConfigWebhook(a.TriggerConfig)
	if cfg.Token == "" || c.Query("token") != cfg.Token {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook token"})
		return
	}
	var payload map[string]any
	_ = c.ShouldBindJSON(&payload)
	run, runErr := h.svc.Run(c.Request.Context(), a, &types.AutomationRunInputs{
		ManualPayload: payload,
	})
	if runErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": runErr.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}
