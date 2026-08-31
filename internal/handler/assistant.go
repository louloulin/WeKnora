package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/llmstream"
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

	// ?stream=1 → SSE. Anything else (or missing) → JSON.
	if c.Query("stream") == "1" {
		h.askStream(c, req, opts)
		return
	}

	resp, err := h.svc.Ask(c.Request.Context(), req, opts)
	if err != nil {
		writeAssistantError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// askStream handles the SSE variant of POST /api/v1/assistant/ask.
// The response Content-Type is text/event-stream and the body is a
// sequence of frames written by llmstream.FormatSSEEvent.
//
// Headers set:
//
//	Content-Type: text/event-stream
//	Cache-Control: no-cache
//	X-Accel-Buffering: no    (disable nginx buffering)
//
// The first frame is always a "metadata" event that carries the
// conversation_id and answer_id so the client can correlate the
// stream with the audit row the backend persisted.
func (h *AssistantHandler) askStream(c *gin.Context, req types.AssistantAskRequest, opts service.AssistantAskOptions) {
	tenantIDStr := c.GetString("tenant_id")
	tenantID, _ := strconv.ParseUint(tenantIDStr, 10, 64)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		// Without a flusher the SSE protocol is broken; the
		// client will never see partial frames.
		errFrame, _ := llmstream.FormatSSEEvent(llmstream.Event{
			Type:  llmstream.EventError,
			Error: errors.New("streaming not supported by this transport"),
		})
		_, _ = c.Writer.Write(errFrame)
		return
	}

	// Metadata frame: lets the client correlate tokens back to the
	// assistant_conversations row.
	meta := fmt.Sprintf(
		"event: metadata\ndata: {\"conversation_id\":\"%s\",\"answer_id\":\"%s\",\"tenant_id\":%d}\n\n",
		req.ConversationID, "", tenantID,
	)
	_, _ = c.Writer.Write([]byte(meta))
	flusher.Flush()

	sink := llmstream.FuncSink(func(e llmstream.Event) error {
		frame, err := llmstream.FormatSSEEvent(e)
		if err != nil {
			return err
		}
		if _, err := c.Writer.Write(frame); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})

	err := h.svc.AskStream(c.Request.Context(), req, opts, sink)
	if err != nil {
		// transient → 503-ish via a final error frame, but the
		// connection is already 200 so the client decides what
		// to do. Permanent → same path; the message carries the
		// cause.
		if llmstream.IsTransient(err) {
			c.Writer.Header().Set("X-WeKnora-Stream-Status", "transient")
		} else {
			c.Writer.Header().Set("X-WeKnora-Stream-Status", "error")
		}
		return
	}
	c.Writer.Header().Set("X-WeKnora-Stream-Status", "ok")
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
