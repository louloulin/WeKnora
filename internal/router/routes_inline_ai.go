package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterInlineAIRoutes wires the v0.7.25 Build #23 paragraph-level
// AI endpoint into the v1 API group. The route is intentionally
// unauthenticated at the gin level — the handler enforces tenant +
// user presence via gin.Context keys populated by the JWT middleware
// further up the chain.
func RegisterInlineAIRoutes(rg *gin.RouterGroup, h *handler.InlineAIHandler) {
	if h == nil {
		return
	}
	g := rg.Group("/ai")
	g.POST("/inline", h.Run)
}
