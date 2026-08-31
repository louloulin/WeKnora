package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterDLPAuthZRoutes wires the v0.7.22 DLP + AuthZ Admin UI routes
// under the v1 group. The upstream auth middleware populates
// tenant_id / user_id in the gin context — the handler trusts those
// and never reads tenant_id from the URL.
//
//   DLP surface (per tenant):
//     POST   /api/v1/dlp/policies
//     GET    /api/v1/dlp/policies
//     GET    /api/v1/dlp/policies/:policy_id
//     POST   /api/v1/dlp/policies/:policy_id/activate
//     POST   /api/v1/dlp/policies/:policy_id/rules
//     GET    /api/v1/dlp/policies/:policy_id/rules
//     DELETE /api/v1/dlp/rules/:rule_id
//     POST   /api/v1/dlp/scan
//     GET    /api/v1/dlp/violations
//
//   AuthZ admin surface (per tenant):
//     POST   /api/v1/authz/policies
//     GET    /api/v1/authz/policies
//     GET    /api/v1/authz/policies/:policy_key
//     GET    /api/v1/authz/policies/:policy_key/versions
//     GET    /api/v1/authz/versions/:version_id
//     POST   /api/v1/authz/policies/:policy_key/rollback
//     POST   /api/v1/authz/simulate
func RegisterDLPAuthZRoutes(r *gin.RouterGroup, h *handler.DLPAuthZHandler) {
	h.Mount(r)
}
