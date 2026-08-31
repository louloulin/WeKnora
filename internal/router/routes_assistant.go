package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerAssistantRoutes wires the AI Assistant Q&A surface behind
// the same JWT auth middleware the rest of the v1 user surface
// uses. The panel is a per-user panel — every endpoint relies on
// the tenant_id / user_id that the auth middleware put on the gin
// context; we never accept them from the URL or body (IDOR guard).
//
//	POST /api/v1/assistant/ask                — ask a question (JSON)
//	POST /api/v1/assistant/ask?stream=1       — same as above, SSE output
//	GET  /api/v1/assistant/conversations      — paginated audit of the user's asks
//	GET  /api/v1/assistant/conversations/:id  — all turns of one thread
func registerAssistantRoutes(group *gin.RouterGroup, h *handler.AssistantHandler) {
	if h == nil || group == nil {
		return
	}
	g := group.Group("/assistant")
	g.POST("/ask", h.Ask)
	g.GET("/conversations", h.ListConversations)
	g.GET("/conversations/:id", h.GetConversation)
}
