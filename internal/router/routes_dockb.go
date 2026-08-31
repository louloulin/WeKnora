package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterDockbRoutes wires the v0.7.23 Doc ↔ KB Bridge + WeKnora
// Base / Database endpoints. Both sub-routers live under the
// authenticated v1 group.
//
//   Doc ↔ KB AI Bridge:
//     GET    /api/v1/dockb/summaries/:knowledge_id
//     GET    /api/v1/dockb/chunks/:knowledge_id/:chunk_id
//     PUT    /api/v1/dockb/chunks/:knowledge_id/:chunk_id
//     DELETE /api/v1/dockb/summaries/:id
//
//   WeKnora Base / Database:
//     GET    /api/v1/databases
//     POST   /api/v1/databases
//     GET    /api/v1/databases/:id
//     PATCH  /api/v1/databases/:id
//     DELETE /api/v1/databases/:id
//     GET    /api/v1/databases/:id/rows
//     POST   /api/v1/databases/:id/rows
//     GET    /api/v1/databases/:id/rows/:row_id
//     PATCH  /api/v1/databases/:id/rows/:row_id
//     DELETE /api/v1/databases/:id/rows/:row_id
//
// Tenant and user are read from the gin context by the upstream auth
// middleware — never from URL/body.
func RegisterDockbRoutes(rg *gin.RouterGroup, h *handler.DockbHandler) {
	if h == nil {
		return
	}
	h.Mount(rg)
}
