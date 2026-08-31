package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterVerificationRoutes wires the Build #29 AI Verification routes
// under the authenticated v1 group. The two endpoints share the same
// KBAccessRead guard the wiki page routes use so any KB member can
// introspect their own pages without Admin.
//
//   /knowledgebase/:kb_id/wiki/pages/:slug/verification
//     → GET  — single-page verification report
//   /knowledgebase/:kb_id/wiki/verification?limit=N
//     → GET  — per-KB scan + summary rollup (limit cap 1000)
//
// When the VerificationHandler is nil (legacy wiring without the
// scanner) the routes simply aren't registered — the Build #29
// feature is opt-in by container presence.
func RegisterVerificationRoutes(rg *gin.RouterGroup, h *handler.VerificationHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	wikiRead := rg.Group("/knowledgebase/:kb_id/wiki", g.OwnedKBOrAdminFromKbIDParam(), g.KBAccessRead("kb_id"))
	wikiRead.GET("/pages/:slug/verification", h.RunForPage)
	wikiRead.GET("/verification", h.RunForKB)
}
