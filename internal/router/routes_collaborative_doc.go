// Package router — v0.7.25 collaborative_docs routes.
//
// Wire surface:
//   POST   /api/v1/collaborative-docs                — create
//   GET    /api/v1/collaborative-docs                — list
//   GET    /api/v1/collaborative-docs/:id            — metadata
//   PATCH  /api/v1/collaborative-docs/:id            — update
//   POST   /api/v1/collaborative-docs/:id/archive    — soft delete
//   DELETE /api/v1/collaborative-docs/:id            — hard delete
//   GET    /api/v1/collaborative-docs/:id/presence   — live presence
//   GET    /api/v1/collaborative-docs/:id/export     — markdown export
//   GET    /api/v1/collaborative-docs/:id/realtime   — Yjs WS upgrade
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterCollabDocRoutes wires the collaborative_docs surface.
func RegisterCollabDocRoutes(rg *gin.RouterGroup, h *handler.CollabDocHandler, ws *handler.CollabDocRealtimeWSHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	h.Mount(rg)
	if ws != nil {
		// WS upgrade: write access is enforced inside the handler so we can
		// return a 101 + immediate close-frame rejection rather than 403.
		rg.GET("/collaborative-docs/:id/realtime",
			g.OwnedWikiKBOrAdmin(),
			g.KBAccessWrite("id"),
			ws.Handle,
		)
	}
}
