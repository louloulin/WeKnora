package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterAuditExportRoutes wires the v0.7.25 Build #24 audit export
// + compliance summary endpoints under /api/v1/audit/*. The whole
// group is gated by g.Admin() so only Tenant Owners / Admins can
// pull the audit trail.
func RegisterAuditExportRoutes(r *gin.RouterGroup, h *handler.AuditExportHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	audit := r.Group("/audit", g.Admin())
	audit.POST("/exports", h.CreateAndDownload)
	audit.GET("/exports", h.List)
	audit.GET("/exports/:id", h.Get)
	audit.GET("/report", h.ComplianceSummary)
}
