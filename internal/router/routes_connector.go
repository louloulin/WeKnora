package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterConnectorRoutes wires the v0.7.24 AI Connector framework
// endpoints. Eight REST calls cover the full lifecycle:
//
//   POST   /api/v1/connectors                — register new connector
//   GET    /api/v1/connectors                — list + kinds
//   GET    /api/v1/connectors/jobs           — all jobs for tenant
//   GET    /api/v1/connectors/:id            — one connector
//   PATCH  /api/v1/connectors/:id            — update
//   DELETE /api/v1/connectors/:id            — soft delete
//   POST   /api/v1/connectors/:id/trigger    — sync now
//   GET    /api/v1/connectors/:id/jobs       — jobs for one connector
//
// Tenant and user are read from gin context by the upstream auth
// middleware — never from URL/body.
func RegisterConnectorRoutes(rg *gin.RouterGroup, h *handler.ConnectorHandler) {
	if h == nil {
		return
	}
	h.Mount(rg)
}
