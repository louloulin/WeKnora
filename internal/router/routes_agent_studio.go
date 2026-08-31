package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterAgentStudioRoutes wires the v0.7.21 Custom Agent Studio
// endpoints under the KB surface so the upstream rbacGuards stack
// enforces KB-Viewer / KB-Editor.
//
//   POST   /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/triggers
//   GET    /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/triggers
//   POST   /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/triggers/:trigger_id/pause
//   POST   /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/triggers/:trigger_id/resume
//   DELETE /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/triggers/:trigger_id
//   POST   /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/run
//   GET    /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/runs
//   GET    /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/runs/:run_id
//   POST   /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/credentials
//   GET    /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/credentials
//   DELETE /api/v1/knowledgebase/:kb_id/agents/:agent_id/studio/credentials/:name
func RegisterAgentStudioRoutes(r *gin.RouterGroup, h *handler.AgentStudioHandler) {
	g := r.Group("/knowledgebase/:kb_id/agents")
	h.Mount(g)
}
