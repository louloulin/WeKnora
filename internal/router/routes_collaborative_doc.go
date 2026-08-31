// Package router — v0.7.26 collaborative_docs routes.
//
// Wire surface:
//   POST   /api/v1/collaborative-docs                  — create
//   GET    /api/v1/collaborative-docs                  — list
//   GET    /api/v1/collaborative-docs/:id              — metadata
//   PATCH  /api/v1/collaborative-docs/:id              — update
//   POST   /api/v1/collaborative-docs/:id/archive      — soft delete
//   DELETE /api/v1/collaborative-docs/:id              — hard delete
//   GET    /api/v1/collaborative-docs/:id/presence     — live presence
//   GET    /api/v1/collaborative-docs/:id/export       — markdown export
//   POST   /api/v1/collaborative-docs/:id/upload       — v0.7.26 binary upload
//   GET    /api/v1/collaborative-docs/:id/download     — v0.7.26 latest bytes
//   GET    /api/v1/collaborative-docs/:id/download/:v  — v0.7.26 historical version
//   GET    /api/v1/collaborative-docs/:id/realtime     — Yjs WS upgrade
//   POST   /api/v1/collaborative-docs/:id/comments     — v0.7.29 add comment
//   GET    /api/v1/collaborative-docs/:id/comments     — v0.7.29 list comments
//   PATCH  /api/v1/collaborative-docs/:id/comments/:commentID — v0.7.29 edit
//   DELETE /api/v1/collaborative-docs/:id/comments/:commentID — v0.7.29 remove
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterCollabDocRoutes wires the collaborative_docs surface.
func RegisterCollabDocRoutes(
	rg *gin.RouterGroup,
	h *handler.CollabDocHandler,
	bytesH *handler.CollabDocBytesHandler,
	ws *handler.CollabDocRealtimeWSHandler,
	commentH *handler.CollabDocCommentHandler,
	auditH *handler.CollabDocAuditHandler,
	g *rbacGuards,
) {
	if h != nil {
		h.Mount(rg)
	}
	if auditH != nil {
		auditH.Register(rg)
	}
	if bytesH != nil {
		// Binary upload/download need write access. Read enforcement is
		// performed inside the handler so we can return 404 for missing
		// docs rather than 403 for ACL.
		bytesH.Mount(rg)
	}
	if ws != nil {
		// WS upgrade: write access is enforced inside the handler so we can
		// return a 101 + immediate close-frame rejection rather than 403.
		rg.GET("/collaborative-docs/:id/realtime",
			g.OwnedWikiKBOrAdmin(),
			g.KBAccessWrite("id"),
			ws.Handle,
		)
	}
	if commentH != nil {
		commentH.Register(rg)
	}
}
