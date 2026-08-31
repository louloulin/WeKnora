package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterWikiSyncBlockRoutes wires the v0.7.20 synced-block endpoints
// under /api/v1/knowledgebase/:kb_id/wiki/sync-blocks.
//
// The routes rely on the upstream rbacGuards stack already attached to
// the wiki surface (Viewer / KB-read for GET, KB-write for POST/PUT/DELETE)
// — we don't re-attach them here to avoid double-wrapping.
func RegisterWikiSyncBlockRoutes(r *gin.RouterGroup, h *handler.WikiSyncBlockHandler) {
	g := r.Group("/knowledgebase/:kb_id/wiki/sync-blocks")
	h.Mount(g)
}
