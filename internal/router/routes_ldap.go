package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerLDAPRoutes wires the directory integration endpoints onto
// the public auth router group and the admin router group. Mirrors
// the SAML route layout so the frontend code is symmetric:
//
//	POST /auth/ldap/login       — public auth group
//	GET    /ldap/configs
//	POST   /ldap/configs
//	GET    /ldap/configs/:id
//	PUT    /ldap/configs/:id
//	DELETE /ldap/configs/:id
//	POST   /ldap/configs/:id/test
//	GET    /system/admin/ldap/configs   — admin group
func registerLDAPRoutes(publicGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, h *handler.LDAPHandler) {
	if h == nil {
		return
	}
	if publicGroup != nil {
		publicGroup.POST("/auth/ldap/login", h.Login)
		ldap := publicGroup.Group("/ldap")
		ldap.GET("/configs", h.GetConfigForTenant)
		ldap.POST("/configs", h.CreateConfig)
		ldap.GET("/configs/:id", h.GetConfig)
		ldap.PUT("/configs/:id", h.UpdateConfig)
		ldap.DELETE("/configs/:id", h.DeleteConfig)
		ldap.POST("/configs/:id/test", h.TestConnection)
	}
	if adminGroup != nil {
		adminGroup.GET("/system/admin/ldap/configs", h.ListConfigs)
	}
}
