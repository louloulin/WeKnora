package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerSAMLRoutes wires the SAML 2.0 endpoints onto the public
// auth router group. Metadata + login + ACS are public; the IdP
// CRUD endpoints sit behind the Admin role guard.
func registerSAMLRoutes(publicGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, h *handler.SAMLHandler) {
	if h == nil {
		return
	}
	// Public SAML SSO flow.
	publicGroup.GET("/auth/saml/metadata", h.SAMLMetadata)
	publicGroup.GET("/auth/saml/login", h.SAMLLogin)
	publicGroup.POST("/auth/saml/acs", h.SAMLACS)
	// Admin: tenant IdP CRUD. Admin group is expected to already
	// enforce the tenant scope and Admin role.
	if adminGroup != nil {
		adminGroup.GET("/auth/saml/idp", h.SAMLGetIdP)
		adminGroup.POST("/auth/saml/idp", h.SAMLCreateIdP)
		adminGroup.PUT("/auth/saml/idp", h.SAMLUpdateIdP)
		adminGroup.DELETE("/auth/saml/idp", h.SAMLDeleteIdP)
	}
}
