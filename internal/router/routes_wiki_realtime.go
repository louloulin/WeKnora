package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterWikiRealtimeRoutes wires the v0.7.19 Yjs realtime collaboration
// routes. The WebSocket endpoint is gated by the same Viewer / KB-write
// guard stack as the rest of the Wiki surface; AuthZ phase-3 is consulted
// once on upgrade (see handler.ValidateWriteAccess).
//
// Path shape (matches the existing wiki URL surface):
//
//	GET /api/v1/knowledgebase/:kb_id/wiki/realtime/:page_id             (WS upgrade)
//	GET /api/v1/knowledgebase/:kb_id/wiki/realtime/:page_id/presence    (presence snapshot)
//	GET /api/v1/knowledgebase/_realtime/_stats                          (admin stats)
//
// The kb_id is captured in the URL so we can hand it to the handler's
// ValidateWriteAccess call alongside page_id; this keeps the surface
// consistent with the existing /knowledgebase/:kb_id/wiki/* routes.
func RegisterWikiRealtimeRoutes(r *gin.RouterGroup, h *handler.WikiRealtimeWSHandler, g *rbacGuards) {
	// Read-side routes — Viewer + KB read.
	wikiRead := g.apiKeyGroup(r.Group("/knowledgebase/:kb_id/wiki/realtime"), apiKeyRetrieve(apiKeyFullAccess()))
	{
		// GET /api/v1/knowledgebase/:kb_id/wiki/realtime/:page_id/presence
		wikiRead.GET("/:page_id/presence", g.Viewer(), g.KBAccessRead("kb_id"), h.HandlePresence)

		// GET /api/v1/knowledgebase/:kb_id/wiki/realtime/:page_id
		// WebSocket upgrade; write access is enforced inside the handler
		// so we can return a 101 upgrade + immediate close-frame rejection
		// rather than a flat 403 (better client UX).
		wikiRead.GET("/:page_id", g.OwnedWikiKBOrAdmin(), g.KBAccessWrite("kb_id"), h.Handle)
	}

	// Admin stats — top-level so it doesn't require a kb_id.
	stats := r.Group("/_realtime")
	{
		stats.GET("/_stats", h.HandleStats)
	}
}
