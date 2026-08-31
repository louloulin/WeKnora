package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterDatabaseRoutes wires the Build #26 (G06) multi-view database
// REST surface. Read paths use OwnedKBOrAdmin (creator or tenant admin);
// write paths use Admin() (tenant admin) since database mutation can
// change schema that affects all viewers.
func RegisterDatabaseRoutes(
	v1 *gin.RouterGroup,
	h *handler.DatabaseHandler,
	g *rbacGuards,
) {
	// Top-level list/create under a knowledge base.
	v1.GET("/knowledge-bases/:kb_id/databases", g.OwnedKBOrAdminFromKbIDParam(), h.List)
	v1.POST("/knowledge-bases/:kb_id/databases", g.Admin(), h.Create)

	// Database-scoped reads.
	v1.GET("/databases/:id", g.OwnedKBOrAdmin(), h.Get)
	v1.GET("/databases/:id/rows", g.OwnedKBOrAdmin(), h.ListRows)
	v1.GET("/databases/:id/views", g.OwnedKBOrAdmin(), h.ListViews)

	// Database-scoped writes.
	v1.PATCH("/databases/:id", g.Admin(), h.Update)
	v1.DELETE("/databases/:id", g.Admin(), h.Delete)

	// Fields.
	v1.POST("/databases/:id/fields", g.Admin(), h.AddField)
	v1.PATCH("/databases/:id/fields/:field_id", g.Admin(), h.UpdateField)
	v1.DELETE("/databases/:id/fields/:field_id", g.Admin(), h.DeleteField)

	// Rows.
	v1.POST("/databases/:id/rows", g.Admin(), h.AddRow)
	v1.PATCH("/databases/:id/rows/:row_id", g.Admin(), h.UpdateRow)
	v1.POST("/databases/:id/rows/reorder", g.Admin(), h.ReorderRows)
	v1.DELETE("/databases/:id/rows/:row_id", g.Admin(), h.DeleteRow)

	// Views.
	v1.POST("/databases/:id/views", g.Admin(), h.AddView)
	v1.PATCH("/databases/:id/views/:view_id", g.Admin(), h.UpdateView)
	v1.DELETE("/databases/:id/views/:view_id", g.Admin(), h.DeleteView)
}
