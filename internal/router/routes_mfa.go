package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerMFARoutes wires the per-user MFA endpoints behind the
// same JWT auth middleware the rest of the user surface uses.
//
//	POST   /api/v1/mfa/enroll
//	POST   /api/v1/mfa/verify
//	GET    /api/v1/mfa
//	DELETE /api/v1/mfa/:id
//
// The /verify endpoint is the building block the login flow will
// call once the SAML / LDAP / password credentials have been
// validated. Adding that hookup is a follow-up; the primitives
// shipped here are sufficient to wire it.
func registerMFARoutes(group *gin.RouterGroup, h *handler.MFAHandler) {
	if h == nil || group == nil {
		return
	}
	// Engine-level middleware.Auth already protects v1, which is
	// where this group lives; users manage their own MFA so no
	// per-route role guard is needed.
	g := group.Group("/mfa")
	g.POST("/enroll", h.Enroll)
	g.POST("/verify", h.Verify)
	g.GET("", h.List)
	g.DELETE("/:id", h.Disable)
}
