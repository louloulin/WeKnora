// Package router — Build #44 Slide routes.
//
// Wires SlideHandler into the v1 group.
package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterSlideRoutes mounts the Slide REST surface under /api/v1.
//
// Routes (Build #44):
//   POST   /api/v1/slides
//   GET    /api/v1/slides
//   POST   /api/v1/slides/auto-generate
//   GET    /api/v1/slides/:id
//   PATCH  /api/v1/slides/:id
//   DELETE /api/v1/slides/:id
//   GET    /api/v1/slides/:id/slides
//   POST   /api/v1/slides/:id/slides
//   PATCH  /api/v1/slides/:id/slides/:slideID
//   DELETE /api/v1/slides/:id/slides/:slideID
//   GET    /api/v1/slides/:id/export
func RegisterSlideRoutes(rg *gin.RouterGroup, h *handler.SlideHandler) {
	if h == nil {
		return
	}
	h.Mount(rg)
}
