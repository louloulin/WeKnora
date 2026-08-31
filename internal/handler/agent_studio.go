package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service/agentstudio"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// AgentStudioHandler exposes the v0.7.21 Custom Agent Studio REST API.
// Eleven endpoints cover triggers, runs, credentials, and quota policies.
//
// Routes are wired under the KB surface — /api/v1/knowledgebase/:kb_id
// — so the upstream rbacGuards stack enforces KB-Viewer / KB-Editor.
// We never read tenant_id / user_id from URL/body (IDOR-safe).
type AgentStudioHandler struct {
	svc *agentstudio.AgentStudioService
}

// NewAgentStudioHandler wires the handler to the service.
func NewAgentStudioHandler(svc *agentstudio.AgentStudioService) *AgentStudioHandler {
	return &AgentStudioHandler{svc: svc}
}

// Mount attaches routes to a router group. Caller is responsible for
// prepending the KB scope prefix.
func (h *AgentStudioHandler) Mount(rg *gin.RouterGroup) {
	g := rg.Group("/agents/:agent_id/studio")
	g.POST("/triggers", h.CreateTrigger)
	g.GET("/triggers", h.ListTriggers)
	g.POST("/triggers/:trigger_id/pause", h.PauseTrigger)
	g.POST("/triggers/:trigger_id/resume", h.ResumeTrigger)
	g.DELETE("/triggers/:trigger_id", h.DeleteTrigger)
	g.POST("/run", h.Run)
	g.GET("/runs", h.ListRuns)
	g.GET("/runs/:run_id", h.GetRun)
	g.POST("/credentials", h.CreateCredential)
	g.GET("/credentials", h.ListCredentials)
	g.DELETE("/credentials/:name", h.DeleteCredential)
}

// resolveCtx pulls tenant + agent ids from the gin context.
func (h *AgentStudioHandler) resolveCtx(c *gin.Context) (tenantID, userID uint64, agentID string, ok bool) {
	tenantIDVal, _ := c.Get("tenant_id")
	tid, ok := toUint64(tenantIDVal)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return 0, 0, "", false
	}
	userIDVal, _ := c.Get("user_id")
	uid, ok := toUint64(userIDVal)
	if !ok {
		uid = 0 // system-triggered runs may have no user
	}
	agentID = c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing agent_id"})
		return 0, 0, "", false
	}
	return tid, uid, agentID, true
}

func parseUint64Param(c *gin.Context, name string) (uint64, bool) {
	raw := c.Param(name)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return v, true
}

// --- triggers ---

type createTriggerBody struct {
	Name            string `json:"name"`
	TriggerType    string `json:"trigger_type"`
	TriggerConfig  string `json:"trigger_config"`
	PayloadTemplate string `json:"payload_template"`
}

func (h *AgentStudioHandler) CreateTrigger(c *gin.Context) {
	tenantID, userID, agentID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body createTriggerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	trig, err := h.svc.Trigger().Create(c.Request.Context(), tenantID, userID,
		agentID, body.TriggerType, body.Name, body.TriggerConfig, body.PayloadTemplate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, trig)
}

func (h *AgentStudioHandler) ListTriggers(c *gin.Context) {
	tenantID, _, agentID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	out, err := h.svc.Trigger().ListByAgent(c.Request.Context(), tenantID, agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"triggers": out, "total": len(out)})
}

func (h *AgentStudioHandler) PauseTrigger(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, ok := parseUint64Param(c, "trigger_id")
	if !ok {
		return
	}
	if err := h.svc.Trigger().Pause(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "paused"})
}

func (h *AgentStudioHandler) ResumeTrigger(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, ok := parseUint64Param(c, "trigger_id")
	if !ok {
		return
	}
	if err := h.svc.Trigger().Resume(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "active"})
}

func (h *AgentStudioHandler) DeleteTrigger(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, ok := parseUint64Param(c, "trigger_id")
	if !ok {
		return
	}
	if err := h.svc.Trigger().Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- runs ---

type runBody struct {
	Input map[string]any `json:"input"`
}

func (h *AgentStudioHandler) Run(c *gin.Context) {
	tenantID, userID, agentID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body runBody
	_ = c.ShouldBindJSON(&body) // empty body is allowed (manual trigger)
	run, err := h.svc.Run(c.Request.Context(), agentstudio.RunOpts{
		TenantID:      tenantID,
		AgentID:       agentID,
		TriggeredBy:   types.AgentRunStatusRunning,
		TriggeredUser: &userID,
		Input:         body.Input,
	})
	if err != nil {
		// Quota-exceeded → 429; everything else → 500.
		if err == agentstudio.ErrorQuotaExceeded {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}

func (h *AgentStudioHandler) ListRuns(c *gin.Context) {
	tenantID, _, agentID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	runs, total, err := h.svc.ListRuns(c.Request.Context(), tenantID, agentID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs, "total": total})
}

func (h *AgentStudioHandler) GetRun(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, ok := parseUint64Param(c, "run_id")
	if !ok {
		return
	}
	run, err := h.svc.GetRun(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	c.JSON(http.StatusOK, run)
}

// --- credentials (vault) ---

type createCredentialBody struct {
	Name           string `json:"name"`
	CredentialType string `json:"credential_type"`
	Secret         string `json:"secret"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

func (h *AgentStudioHandler) CreateCredential(c *gin.Context) {
	tenantID, userID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body createCredentialBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cred, err := h.svc.Vault().Create(c.Request.Context(), tenantID, userID,
		body.Name, body.CredentialType, []byte(body.Secret), body.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":              cred.ID,
		"name":            cred.Name,
		"credential_type": cred.CredentialType,
		"expires_at":      cred.ExpiresAt,
		"created_at":      cred.CreatedAt,
	})
}

func (h *AgentStudioHandler) ListCredentials(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	out, err := h.svc.Vault().List(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Sanitize: drop ciphertext fields from the wire response.
	safe := make([]gin.H, 0, len(out))
	for _, c := range out {
		safe = append(safe, gin.H{
			"id":              c.ID,
			"name":            c.Name,
			"credential_type": c.CredentialType,
			"expires_at":      c.ExpiresAt,
			"last_used_at":    c.LastUsedAt,
			"created_at":      c.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"credentials": safe, "total": len(safe)})
}

func (h *AgentStudioHandler) DeleteCredential(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing name"})
		return
	}
	if err := h.svc.Vault().Delete(c.Request.Context(), tenantID, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// keep ctx import in case future handlers need it.
var _ = context.Background
var _ = fmt.Sprintf
var _ = json.RawMessage{}
