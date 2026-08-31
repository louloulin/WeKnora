package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterUserDailyNoteRoutes wires the Build #45.a Daily Note
// endpoints. Mounted under /api/v1/user/daily-notes. Authorization
// is the Viewer floor — daily notes are personal data, not
// KB-owned, so any authenticated user can read / write their own.
//
// Endpoints:
//
//   GET    /user/daily-notes/today?kb_id=...
//     → GetOrCreate today's note for the caller
//   GET    /user/daily-notes/date?kb_id=...&date=YYYY-MM-DD
//     → GetOrCreate a date-pinned note (jump-to-date UI)
//   GET    /user/daily-notes?kb_id=...&from=YYYY-MM-DD&to=YYYY-MM-DD&limit=N
//     → range list (dashboard widget)
//   PATCH  /user/daily-notes/:id
//     → title + content + summary
//   GET    /user/daily-notes/count?kb_id=...&from=...&to=...
//     → row count (dashboard "X notes this month" copy)
//
// The handler is nil-tolerant so legacy deployments without the
// service still boot, just without the new endpoints.
func RegisterUserDailyNoteRoutes(r *gin.RouterGroup, h *handler.UserDailyNoteHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	notes := r.Group("/user/daily-notes", g.Viewer())
	{
		notes.GET("/today", h.GetOrCreateToday)
		notes.GET("/date", h.GetOrCreateDate)
		notes.GET("", h.ListRange)
		notes.GET("/count", h.CountRange)
		notes.PATCH("/:id", h.UpdateContent)
	}
}
