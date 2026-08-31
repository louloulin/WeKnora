package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterAuthZAdminRoutes wires the admin AuthZ tuple CRUD +
// debug check endpoints onto the SystemAdmin group. These
// endpoints are Phase-3 P0 — they expose the persistent
// authorization tuple store + a debug check that mirrors the
// runtime decision engine.
//
// All routes inherit the SystemAdmin guard from the parent
// group; adding new endpoints cannot accidentally drop the gate.
func RegisterAuthZAdminRoutes(
	adminGroup *gin.RouterGroup,
	h *handler.AuthZHandler,
	apiKeyGuards *rbacGuards,
) {
	if h == nil || adminGroup == nil {
		return
	}
	// Phase-3: only SystemAdmin human operators can manage
	// tuples. The platform API key policy mirrors /settings so
	// automation (e.g. SCIM-driven provisioning) can use the
	// same endpoints with the right scope.
	if apiKeyGuards == nil {
		adminGroup.POST("/authz/tuples", h.CreateTuple)
		adminGroup.GET("/authz/tuples", h.ListTuples)
		adminGroup.DELETE("/authz/tuples/:id", h.RevokeTuple)
		adminGroup.POST("/authz/check", h.CheckTuple)
		return
	}
	apiKeyGuards.apiKeyRoute(adminGroup, http.MethodPost, "/authz/tuples",
		apiKeyPlatform(), h.CreateTuple)
	apiKeyGuards.apiKeyRoute(adminGroup, http.MethodGet, "/authz/tuples",
		apiKeyPlatform(), h.ListTuples)
	apiKeyGuards.apiKeyRoute(adminGroup, http.MethodDelete, "/authz/tuples/:id",
		apiKeyPlatform(), h.RevokeTuple)
	apiKeyGuards.apiKeyRoute(adminGroup, http.MethodPost, "/authz/check",
		apiKeyPlatform(), h.CheckTuple)
}
