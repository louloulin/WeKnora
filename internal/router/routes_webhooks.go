// Package router — Build #46.x Webhook routes.
//
// Wires WebhookHandler into the v1 group. Subscription CRUD lives
// here; event emission is fired from inside the collab / slide
// service layers via interfaces.WebhookService.PublishEvent so the
// dispatcher goroutine does not block user requests.
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterWebhookRoutes(rg *gin.RouterGroup, h *handler.WebhookHandler) {
	if h == nil {
		return
	}
	h.Mount(rg)
}
