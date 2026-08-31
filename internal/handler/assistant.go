package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// AssistantHandler is the HTTP entrypoint for the AI Assistant Q&A
// backend. The handler is thin: it parses the wire body, reads the
// authenticated principal from the gin context, delegates to the
// service, and renders the response.
//
// Route surface (registered via routes_assistant.go):
//
//	POST /api/v1/assistant/ask
//	GET  /api/v1/assistant/conversations
//	GET  /api/v1/assistant/conversations/:id
type AssistantHandler struct {
	svc *service.AssistantService
}

// NewAssistantHandler is the DI constructor.
func NewAssistantHandler(svc *service.AssistantService) *AssistantHandler {
	return &AssistantHandler{svc: svc}
}

// Ask handles POST /api/v1/assistant/ask. The handler reads the
// tenant_id + user_id from the gin context (set by the auth
// middleware) and never trusts them from the URL or body — that
// would be an IDOR waiting to happen.
func (h *AssistantHandler) Ask(c *gin.Context) {
	tenantIDStr := c.GetString("tenant_id")
	if tenantIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	tenantID, err := strconv.ParseUint(tenantIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id not a uint64"})
		return
	}
	userID := c.GetString("user_id")

	var req types.AssistantAskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	opts := service.AssistantAskOptions{
		TenantID: tenantID,
		UserID:   userID,
		// VisibleKBIDs is filled by an optional future hook that
		// resolves the tenant-wide KB ACL envelope. For now we
		// pass an empty slice so the service falls back to
		// req.SourceKBIDs only.
	}

	resp, err := h.svc.Ask(c.Request.Context(), req, opts)
	if err != nil {
		writeAssistantError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ListConversations handles GET /api/v1/assistant/conversations.
func (h *AssistantHandler) ListConversations(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := h.svc.ListConversations(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		writeAssistantError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total})
}

// GetConversation handles GET /api/v1/assistant/conversations/:id
// (where :id is the conversation_id, not the row id).
func (h *AssistantHandler) GetConversation(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id missing"})
		return
	}
	rows, err := h.svc.GetConversation(c.Request.Context(), tenantID, conversationID)
	if err != nil {
		writeAssistantError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// writeAssistantError is the single place that maps service sentinels
// to HTTP status codes. Keeping the mapping here means adding a new
// error is a one-line change.
//
// Sentinel → status:
//
//	ErrAssistantInvalidRequest → 400
//	anything else              → 500
func writeAssistantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAssistantInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
