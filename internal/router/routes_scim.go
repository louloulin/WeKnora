package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerSCIMRoutes wires the SCIM 2.0 protocol endpoints behind
// the bearer-token middleware. The discovery endpoints are at the
// top of /scim/v2 per RFC 7644 §3; the resource endpoints follow.
//
//	GET    /scim/v2/ServiceProviderConfig
//	GET    /scim/v2/ResourceTypes
//	GET    /scim/v2/Schemas            (deferred — the major IdPs
//	                                   do not require it for
//	                                   provisioning to work)
//	GET    /scim/v2/Users
//	POST   /scim/v2/Users
//	GET    /scim/v2/Users/:id
//	PUT    /scim/v2/Users/:id
//	PATCH  /scim/v2/Users/:id
//	DELETE /scim/v2/Users/:id
//	GET    /scim/v2/Groups
//	GET    /scim/v2/Groups/:id
//
// The bearer middleware is mandatory — public SCIM endpoints are
// not a thing and would leak the entire user directory.
func registerSCIMRoutes(group *gin.RouterGroup, h *handler.SCIMHandler, mw gin.HandlerFunc) {
	if h == nil || group == nil || mw == nil {
		return
	}
	g := group.Group("/scim/v2")
	g.Use(mw)
	g.GET("/ServiceProviderConfig", h.ServiceProviderConfig)
	g.GET("/ResourceTypes", h.ResourceTypes)
	g.GET("/Users", h.UsersList)
	g.POST("/Users", h.UserCreate)
	g.GET("/Users/:id", h.UserGet)
	g.PUT("/Users/:id", h.UserReplace)
	g.PATCH("/Users/:id", h.UserPatch)
	g.DELETE("/Users/:id", h.UserDelete)
	g.GET("/Groups", h.GroupsList)
	g.GET("/Groups/:id", h.GroupGet)
}
