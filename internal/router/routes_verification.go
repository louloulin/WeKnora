package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterVerificationRoutes wires both Build #29 (AI scanner) and
// Build #48 (Verified Knowledge Engine) routes under the
// authenticated v1 group. The two layers share the same KBAccessRead
// guard the wiki page routes use so any KB member can introspect
// their own pages without Admin.
//
// AI scanner (Build #29, GET only):
//
//   /knowledgebase/:kb_id/wiki/pages/:slug/verification
//     → GET  — single-page verification report
//   /knowledgebase/:kb_id/wiki/verification?limit=N
//     → GET  — per-KB scan + summary rollup (limit cap 1000)
//
// Verified Knowledge (Build #48, POST only):
//
//   /knowledgebase/:kb_id/wiki/pages/:slug/verification/verify
//     → POST — stamp verified_at = now, verified_by = caller,
//              advance review_due_at by DefaultReviewInterval.
//   /knowledgebase/:kb_id/wiki/pages/:slug/verification/review-due
//     → POST — set review_owner + review_due_at in one shot.
//
// Both Verify* routes require KB write access; the Read routes only
// need KB read. The handler is nil-tolerant so legacy deployments
// without Build #48 still boot, just without the new endpoints.
func RegisterVerificationRoutes(rg *gin.RouterGroup, h *handler.VerificationHandler, wvh *handler.WikiVerificationHandler, g *rbacGuards) {
	wikiRead := rg.Group("/knowledgebase/:kb_id/wiki", g.OwnedKBOrAdminFromKbIDParam(), g.KBAccessRead("kb_id"))
	if h != nil {
		wikiRead.GET("/pages/:slug/verification", h.RunForPage)
		wikiRead.GET("/verification", h.RunForKB)
	}
	if wvh != nil {
		// Verified Knowledge Engine mutations live under the same
		// :slug-scoped URL prefix but need write access — any KB
		// member can verify, the same as the wiki page update path.
		wikiWrite := rg.Group("/knowledgebase/:kb_id/wiki", g.OwnedKBOrAdminFromKbIDParam(), g.KBAccessWrite("kb_id"))
		wikiWrite.POST("/pages/:slug/verification/verify", wvh.VerifyPage)
		wikiWrite.POST("/pages/:slug/verification/review-due", wvh.SetReviewSchedule)
	}
}
