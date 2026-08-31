package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// registerWikiKBReferenceRoutes wires the doc + KB integration endpoints
// onto the authenticated wiki group. Both directions live under the same
// handler so the auth contract is identical:
//
//   - GET    /wiki/pages/:id/references/kb            (page → many KB)
//
//   - POST   /wiki/pages/:id/references/kb            (page → add one KB)
//
//   - GET    /wiki/pages/:id/references/kb/:kbId      (page → one KB)
//
//   - DELETE /wiki/pages/:id/references/kb/:kbId      (page → drop one KB)
//
//   - GET    /knowledge/:id/references/wiki           (KB → many pages)
//
// The function is defensive: a nil handler is allowed so unit tests
// that wire only a subset of routes do not have to provide a stub
// handler.
func registerWikiKBReferenceRoutes(wikiGroup *gin.RouterGroup, knowledgeGroup *gin.RouterGroup, h *handler.WikiKBReferenceHandler) {
	if h == nil {
		return
	}
	if wikiGroup != nil {
		wikiGroup.GET("/pages/:id/references/kb", h.ListForWikiPage)
		wikiGroup.POST("/pages/:id/references/kb", h.AddReference)
		wikiGroup.GET("/pages/:id/references/kb/:kbId", h.ResolveReference)
		wikiGroup.DELETE("/pages/:id/references/kb/:kbId", h.RemoveReference)
	}
	if knowledgeGroup != nil {
		knowledgeGroup.GET("/:id/references/wiki", h.ListForKnowledge)
	}
}
