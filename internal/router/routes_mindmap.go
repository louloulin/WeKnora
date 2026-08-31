// Package router — Build #43 MindMap routes.
//
// Wires MindMapHandler into the v1 group. The caller passes the
// MindMapHandler as a constructor argument so it can be omitted in tests.
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterMindMapRoutes mounts the MindMap REST surface under /api/v1.
//
// Routes (Build #43):
//   POST   /api/v1/mindmaps
//   GET    /api/v1/mindmaps
//   GET    /api/v1/mindmaps/:id
//   PATCH  /api/v1/mindmaps/:id
//   DELETE /api/v1/mindmaps/:id
//   POST   /api/v1/mindmaps/:id/nodes
//   GET    /api/v1/mindmaps/:id/nodes
//   PATCH  /api/v1/mindmaps/:id/nodes/:nodeID
//   DELETE /api/v1/mindmaps/:id/nodes/:nodeID
//   POST   /api/v1/mindmaps/:id/auto-layout
//   GET    /api/v1/mindmaps/:id/export
func RegisterMindMapRoutes(rg *gin.RouterGroup, h *handler.MindMapHandler) {
	if h == nil {
		return
	}
	h.Mount(rg)
}
