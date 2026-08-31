package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerConditionalAccessRoutes wires the conditional access admin
// CRUD surface. The login flow evaluator is invoked directly by the
// auth handler — no hot-path endpoint needed here.
//
// Routes sit behind the admin group so only Admin+ can manage them.
func registerConditionalAccessRoutes(adminGroup *gin.RouterGroup, h *handler.ConditionalAccessHandler) {
	if h == nil || adminGroup == nil {
		return
	}
	policies := adminGroup.Group("/conditional-access/policies")
	{
		policies.GET("", h.ListPolicies)
		policies.POST("", h.CreatePolicy)
		policies.GET("/:id", h.GetPolicy)
		policies.PUT("/:id", h.UpdatePolicy)
		policies.DELETE("/:id", h.DeletePolicy)
	}
}
