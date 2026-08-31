package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/authzadmin"
	"github.com/Tencent/WeKnora/internal/application/service/dlp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// DLPAuthZHandler exposes the v0.7.22 DLP + AuthZ Admin REST API. Two
// sub-routers are mounted:
//
//   /api/v1/dlp/...     — policies, rules, scan, violations
//   /api/v1/authz/...   — policy versions, simulator, diff
//
// All routes are tenant-scoped via the gin context (set by upstream
// auth middleware); we never read tenant_id from URL/body.
type DLPAuthZHandler struct {
	dlp   *dlp.DLPScanner
	authz *authzadmin.AuthZAdmin
}

// NewDLPAuthZHandler wires the handler to the services.
func NewDLPAuthZHandler(dlpSvc *dlp.DLPScanner, authzSvc *authzadmin.AuthZAdmin) *DLPAuthZHandler {
	return &DLPAuthZHandler{dlp: dlpSvc, authz: authzSvc}
}

// Mount attaches routes to the v1 group. Caller owns the prefix.
func (h *DLPAuthZHandler) Mount(rg *gin.RouterGroup) {
	// DLP sub-router
	dlp := rg.Group("/dlp")
	dlp.POST("/policies", h.CreateDLPPolicy)
	dlp.GET("/policies", h.ListDLPPolicies)
	dlp.GET("/policies/:policy_id", h.GetDLPPolicy)
	dlp.POST("/policies/:policy_id/activate", h.ActivateDLPPolicy)
	dlp.POST("/policies/:policy_id/rules", h.AddDLPRule)
	dlp.GET("/policies/:policy_id/rules", h.ListDLPRules)
	dlp.DELETE("/rules/:rule_id", h.DeleteDLPRule)
	dlp.POST("/scan", h.ScanText)
	dlp.GET("/violations", h.ListViolations)

	// AuthZ sub-router
	authz := rg.Group("/authz")
	authz.POST("/policies", h.PublishAuthZPolicy)
	authz.GET("/policies", h.ListAuthZKeys)
	authz.GET("/policies/:policy_key", h.GetLatestAuthZ)
	authz.GET("/policies/:policy_key/versions", h.ListAuthZVersions)
	authz.GET("/versions/:version_id", h.GetAuthZVersion)
	authz.POST("/policies/:policy_key/rollback", h.RollbackAuthZ)
	authz.POST("/simulate", h.SimulateAuthZ)
}

// helper — extract tenant + user from gin context.
func (h *DLPAuthZHandler) resolveCtx(c *gin.Context) (tenantID, userID uint64, ok bool) {
	tv, _ := c.Get("tenant_id")
	uv, _ := c.Get("user_id")
	tid, ok1 := toUint64(tv)
	uid, ok2 := toUint64(uv)
	if !ok1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return 0, 0, false
	}
	if !ok2 {
		uid = 0
	}
	return tid, uid, true
}

// --- DLP policies ---

type createPolicyBody struct {
	Name          string `json:"name"`
	ResourceScope string `json:"resource_scope"`
	Severity      string `json:"severity"`
	Action        string `json:"action"`
	Description   string `json:"description"`
}

func (h *DLPAuthZHandler) CreateDLPPolicy(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body createPolicyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := h.dlp.CreatePolicy(c.Request.Context(), tenantID, userID,
		body.Name, body.ResourceScope, body.Severity, body.Action, body.Description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *DLPAuthZHandler) ListDLPPolicies(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	out, err := h.dlp.ListPolicies(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": out, "total": len(out)})
}

func (h *DLPAuthZHandler) GetDLPPolicy(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("policy_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
		return
	}
	p, err := h.dlp.GetPolicy(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *DLPAuthZHandler) ActivateDLPPolicy(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("policy_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
		return
	}
	if err := h.dlp.ActivatePolicy(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "active"})
}

// --- DLP rules ---

type addRuleBody struct {
	PatternType  string `json:"pattern_type"`
	PatternValue string `json:"pattern_value"`
	Severity     string `json:"severity"`
	Description  string `json:"description"`
}

func (h *DLPAuthZHandler) AddDLPRule(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	pid, err := strconv.ParseUint(c.Param("policy_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
		return
	}
	var body addRuleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.dlp.AddRule(c.Request.Context(), tenantID, pid,
		body.PatternType, body.PatternValue, body.Severity, body.Description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

func (h *DLPAuthZHandler) ListDLPRules(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	pid, err := strconv.ParseUint(c.Param("policy_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
		return
	}
	out, err := h.dlp.ListRules(c.Request.Context(), tenantID, pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": out, "total": len(out)})
}

func (h *DLPAuthZHandler) DeleteDLPRule(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule_id"})
		return
	}
	if err := h.dlp.DeleteRule(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- DLP scan ---

type scanBody struct {
	Resource   string         `json:"resource"`
	ResourceID string         `json:"resource_id"`
	ActorID    uint64         `json:"actor_id"`
	Text       string         `json:"text"`
}

func (h *DLPAuthZHandler) ScanText(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body scanBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.dlp.Scan(c.Request.Context(), dlp.ScanInput{
		TenantID:   tenantID,
		Resource:   body.Resource,
		ResourceID: body.ResourceID,
		ActorID:    body.ActorID,
		Text:       body.Text,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *DLPAuthZHandler) ListViolations(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	resource := c.Query("resource")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	out, total, err := h.dlp.ListViolations(c.Request.Context(), tenantID, resource, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"violations": out, "total": total})
}

// --- AuthZ policies ---

type publishPolicyBody struct {
	PolicyKey  string `json:"policy_key"`
	Expression string `json:"expression"`
	Decision   string `json:"decision"`
	Metadata   string `json:"metadata"`
}

func (h *DLPAuthZHandler) PublishAuthZPolicy(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body publishPolicyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.authz.PublishPolicy(c.Request.Context(), tenantID, userID,
		body.PolicyKey, body.Expression, body.Decision, body.Metadata)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *DLPAuthZHandler) ListAuthZKeys(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	keys, err := h.authz.ListKeys(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": keys, "total": len(keys)})
}

func (h *DLPAuthZHandler) GetLatestAuthZ(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	key := c.Param("policy_key")
	v, err := h.authz.GetLatest(c.Request.Context(), tenantID, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *DLPAuthZHandler) ListAuthZVersions(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	key := c.Param("policy_key")
	versions, err := h.authz.ListVersions(c.Request.Context(), tenantID, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions, "total": len(versions)})
}

func (h *DLPAuthZHandler) GetAuthZVersion(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("version_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version_id"})
		return
	}
	v, err := h.authz.GetVersion(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	c.JSON(http.StatusOK, v)
}

// RollbackAuthZ re-publishes an older version as the latest.
func (h *DLPAuthZHandler) RollbackAuthZ(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	key := c.Param("policy_key")
	var body struct {
		VersionID uint64 `json:"version_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	src, err := h.authz.GetVersion(c.Request.Context(), tenantID, body.VersionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if src == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	if src.PolicyKey != key {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version does not match policy_key"})
		return
	}
	v, err := h.authz.PublishPolicy(c.Request.Context(), tenantID, userID,
		src.PolicyKey, src.Expression, src.Decision, src.Metadata)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

type simulateBody struct {
	PolicyKey string         `json:"policy_key"`
	Actor     map[string]any `json:"actor"`
	Resource  map[string]any `json:"resource"`
	Action    string         `json:"action"`
}

func (h *DLPAuthZHandler) SimulateAuthZ(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body simulateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	decision, err := h.authz.Simulate(c.Request.Context(), tenantID, authzadmin.SimulateInput{
		PolicyKey: body.PolicyKey,
		Actor:     body.Actor,
		Resource:  body.Resource,
		Action:    body.Action,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"decision": decision})
}

// keep imports used if future edits trim them.
var _ = types.DLPActionLog
