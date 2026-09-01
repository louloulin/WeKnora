// Package router — v0.7.26 collaborative_docs routes.
//
// Wire surface (v0.7.38):
//
//	POST   /api/v1/collaborative-docs                  — create
//	GET    /api/v1/collaborative-docs                  — list
//	GET    /api/v1/collaborative-docs/:id              — metadata
//	PATCH  /api/v1/collaborative-docs/:id              — update
//	POST   /api/v1/collaborative-docs/:id/archive      — soft delete
//	DELETE /api/v1/collaborative-docs/:id              — hard delete
//	GET    /api/v1/collaborative-docs/:id/presence     — live presence
//	GET    /api/v1/collaborative-docs/:id/export       — markdown export
//	POST   /api/v1/collaborative-docs/:id/upload       — v0.7.26 binary upload
//	GET    /api/v1/collaborative-docs/:id/download     — v0.7.26 latest bytes
//	GET    /api/v1/collaborative-docs/:id/download/:v  — v0.7.26 historical version
//	GET    /api/v1/collaborative-docs/:id/realtime     — Yjs WS upgrade
//	POST   /api/v1/collaborative-docs/:id/comments     — v0.7.29 add comment
//	GET    /api/v1/collaborative-docs/:id/comments     — v0.7.29 list comments
//	PATCH  /api/v1/collaborative-docs/:id/comments/:commentID — v0.7.29 edit
//	DELETE /api/v1/collaborative-docs/:id/comments/:commentID — v0.7.29 remove
//	GET    /api/v1/collaborative-docs/audit            — v0.7.30 list audit
//	GET    /api/v1/collaborative-docs/audit/summary   — v0.7.30 summary
//	POST   /api/v1/collaborative-docs/:id/responses      — v0.7.90 submit (public via share_token)
//	GET    /api/v1/collaborative-docs/:id/responses      — v0.7.90 list (owner)
//	GET    /api/v1/collaborative-docs/:id/responses/summary — v0.7.90 aggregate (owner)
//	GET    /api/v1/collaborative-docs/:id/responses/export.csv — v0.7.90 export (owner)
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterCollabDocRoutes wires the collaborative_docs surface with
// per-IP / per-tenant / per-doc rate limits. The IP limiter is the
// first line of defense against unauthenticated enumeration; the
// tenant + doc limiters come in after authentication so they can
// read the tenant and :id route params.
func RegisterCollabDocRoutes(
	rg *gin.RouterGroup,
	h *handler.CollabDocHandler,
	bytesH *handler.CollabDocBytesHandler,
	ws *handler.CollabDocRealtimeWSHandler,
	commentH *handler.CollabDocCommentHandler,
	auditH *handler.CollabDocAuditHandler,
	formRespH *handler.CollabDocFormResponseHandler,
	g *rbacGuards,
	redisClient *redis.Client,
) {
	// IP fallback covers pre-auth and unauthenticated paths.
	rg.Use(middleware.CollabIPRateLimit(redisClient))

	if h != nil {
		// Tenant-level cap covers all authenticated handlers below.
		rg.Use(middleware.CollabTenantRateLimit(redisClient))
		h.Mount(rg)
	}
	if auditH != nil {
		auditH.Register(rg)
	}
	if bytesH != nil {
		// Binary upload/download need write access. Read enforcement is
		// performed inside the handler so we can return 404 for missing
		// docs rather than 403 for ACL.
		rg.Use(middleware.CollabDocRateLimit(redisClient))
		bytesH.Mount(rg)
	}
	if ws != nil {
		// WS upgrade: write access is enforced inside the handler so we can
		// return a 101 + immediate close-frame rejection rather than 403.
		rg.GET("/collaborative-docs/:id/realtime/*room",
			ws.Handle,
		)
	}
	if commentH != nil {
		commentH.Register(rg)
	}
	if formRespH != nil {
		formRespH.Register(rg)
	}
}
