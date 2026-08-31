package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// InlineAIHandler exposes the v0.7.25 Build #23 paragraph-level AI
// endpoint. The constructor takes the InlineAIService interface so
// tests can wire a fake.
type InlineAIHandler struct {
	svc interfaces.InlineAIService
}

// NewInlineAIHandler wires the handler. A nil service is rejected so a
// missing DI wiring fails loudly at startup.
func NewInlineAIHandler(svc interfaces.InlineAIService) *InlineAIHandler {
	if svc == nil {
		panic("handler.NewInlineAIHandler: svc is required")
	}
	return &InlineAIHandler{svc: svc}
}

// Run — POST /api/v1/ai/inline
//
// Body: { action, text, model_id?, instruction?, target_language?, max_tokens? }
// Returns: { action, model, result, input_tokens?, output_tokens? }
//
// Errors:
//   400 — bad input (invalid action, empty text, > 16 KB)
//   401 — missing authentication
//   502 — chat model unavailable or LLM error
//   500 — unexpected
func (h *InlineAIHandler) Run(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req types.InlineAIRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.svc.Run(c.Request.Context(), tenantID, req)
	if err != nil {
		switch {
		case errors.Is(err, types.ErrInlineAIBadInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, interfaces.ErrInlineAIUnavailable):
			c.JSON(http.StatusBadGateway, gin.H{"error": "no chat model configured for this tenant"})
		default:
			logger.Errorf(c.Request.Context(), "inline ai run failed: tenant=%d user=%s action=%s err=%v",
				tenantID, userID, req.Action, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "inline ai: model call failed"})
		}
		return
	}

	// Record the inline AI invocation in the audit log so admins can
	// attribute model usage to a specific user / tenant. The audit log
	// middleware reads gin.Context keys; the simplest path is to
	// publish an event here.
	if c.GetString("audit_log_enabled") == "1" {
		logger.Infof(c.Request.Context(),
			"audit inline_ai: tenant=%d user=%s action=%s model=%s in_tokens=%d out_tokens=%d",
			tenantID, userID, resp.Action, resp.Model, resp.InputTokens, resp.OutputTokens)
	}

	c.JSON(http.StatusOK, resp)
}

