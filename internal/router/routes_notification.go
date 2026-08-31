package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterNotificationRoutes mounts the notification center endpoints
// at /api/v1/notifications. All routes require an authenticated user
// (the auth middleware fills tenant_id + user_id into the gin
// context) and are tenant-scoped — there is no way to read or mutate
// another user's notifications.
//
// The bell dropdown in the top nav calls these routes:
//
//	GET    /notifications                  — page slice for the dropdown
//	GET    /notifications/unread-count     — polled every 30s for the badge
//	POST   /notifications/:id/read         — single-row read transition
//	POST   /notifications/:id/dismiss      — single-row dismiss transition
//	POST   /notifications/read-all         — bulk read transition
//	DELETE /notifications/:id              — admin-only hard delete (GDPR)
//
// The order of registration matters: /notifications/unread-count
// must be declared BEFORE /notifications/:id/* so gin does not treat
// "unread-count" as an :id value.
func RegisterNotificationRoutes(
	r *gin.RouterGroup,
	h *handler.NotificationHandler,
	g *rbacGuards,
) {
	notif := g.apiKeyGroup(r.Group("/notifications"), apiKeyFullAccess())

	notif.GET("", g.Viewer(), h.List)
	notif.GET("/unread-count", g.Viewer(), h.UnreadCount)
	notif.POST("/:id/read", g.Viewer(), h.MarkRead)
	notif.POST("/:id/dismiss", g.Viewer(), h.MarkDismissed)
	notif.POST("/read-all", g.Viewer(), h.MarkAllRead)
	notif.DELETE("/:id", g.Admin(), h.DeleteHard)
}
