package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/doc_integration"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// DocIntegrationHandler serves the Build #42 docs × KB integration
// API. Endpoints (all under /v1):
//
//	POST   /docs/:source_type/:source_id/kg-links         bulk-write doc↔KG relations
//	GET    /docs/:source_type/:source_id/kg-links         list relations for a doc
//	DELETE /docs/:source_type/:source_id/kg-links         clear relations for a doc
//	GET    /kg/:target_type/:target_id/docs               list docs that point at a KG node
//
//	POST   /kb/:chunk_id/wiki-refs                       bulk-write KB→wiki refs
//	GET    /kb/:chunk_id/wiki-refs                       list wiki pages citing a chunk
//	GET    /wiki/:page_id/kb-refs                        list KB chunks cited by a page
//
//	POST   /wiki/:page_id/inline-kb                      add one inline KB citation
//	GET    /wiki/:page_id/inline-kb                      list inline KB citations
//	PUT    /wiki/:page_id/inline-kb                      replace all inline KB citations
//
//	POST   /assistant                                    run the AI Assistant Panel (chat / search / create)
type DocIntegrationHandler struct {
	svc *doc_integration.Service
}

// NewDocIntegrationHandler constructs a DocIntegrationHandler.
func NewDocIntegrationHandler(svc *doc_integration.Service) *DocIntegrationHandler {
	return &DocIntegrationHandler{svc: svc}
}

// Mount attaches all /docs, /kb, /wiki, and /assistant routes.
func (h *DocIntegrationHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/docs/:source_type/:source_id/kg-links", h.BulkLinkDocKg)
	rg.GET("/docs/:source_type/:source_id/kg-links", h.ListDocKgBySource)
	rg.DELETE("/docs/:source_type/:source_id/kg-links", h.ClearDocKgBySource)
	rg.GET("/kg/:target_type/:target_id/docs", h.ListDocKgByTarget)

	rg.POST("/kb/:chunk_id/wiki-refs", h.BulkLinkKbWiki)
	rg.GET("/kb/:chunk_id/wiki-refs", h.ListKbWikiByChunk)
	rg.GET("/wiki/:page_id/kb-refs", h.ListKbWikiByPage)

	rg.POST("/wiki/:page_id/inline-kb", h.AddInlineKB)
	rg.GET("/wiki/:page_id/inline-kb", h.ListInlineKB)
	rg.PUT("/wiki/:page_id/inline-kb", h.ResetInlineKB)

	rg.POST("/assistant", h.Assistant)
}

// --- Doc ↔ KG ---

type docKgBulkInput struct {
	Links []types.DocKgRelation `json:"links"`
}

func (h *DocIntegrationHandler) BulkLinkDocKg(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant"})
		return
	}
	sourceType := c.Param("source_type")
	sourceID := c.Param("source_id")
	var in docKgBulkInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := range in.Links {
		in.Links[i].TenantID = tenantID
		in.Links[i].SourceType = sourceType
		in.Links[i].SourceID = sourceID
		if err := h.svc.LinkDocToKG(c, &in.Links[i]); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"linked": len(in.Links)})
}

func (h *DocIntegrationHandler) ListDocKgBySource(c *gin.Context) {
	out, err := h.svc.ListDocKgRelationsBySource(c, c.Param("source_type"), c.Param("source_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *DocIntegrationHandler) ClearDocKgBySource(c *gin.Context) {
	if err := h.svc.DeleteDocKgRelationsBySource(c, c.Param("source_type"), c.Param("source_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DocIntegrationHandler) ListDocKgByTarget(c *gin.Context) {
	out, err := h.svc.ListDocKgRelationsByTarget(c, c.Param("target_type"), c.Param("target_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// --- KB → wiki ---

type kbWikiBulkInput struct {
	Refs []types.KbWikiReference `json:"refs"`
}

func (h *DocIntegrationHandler) BulkLinkKbWiki(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant"})
		return
	}
	chunkID := c.Param("chunk_id")
	var in kbWikiBulkInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := range in.Refs {
		in.Refs[i].TenantID = tenantID
		in.Refs[i].KBChunkID = chunkID
		if err := h.svc.LinkKbToWiki(c, &in.Refs[i]); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"linked": len(in.Refs)})
}

func (h *DocIntegrationHandler) ListKbWikiByChunk(c *gin.Context) {
	out, err := h.svc.ListKbWikiReferencesByChunk(c, c.Param("chunk_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *DocIntegrationHandler) ListKbWikiByPage(c *gin.Context) {
	out, err := h.svc.ListKbWikiReferencesByPage(c, c.Param("page_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// --- Inline KB ---

func (h *DocIntegrationHandler) AddInlineKB(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant"})
		return
	}
	var in types.InlineKBRef
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.TenantID = tenantID
	in.WikiPageID = c.Param("page_id")
	if err := h.svc.AddInlineKBRef(c, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, in)
}

func (h *DocIntegrationHandler) ListInlineKB(c *gin.Context) {
	out, err := h.svc.ListInlineKBRefsByPage(c, c.Param("page_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

type inlineKBResetInput struct {
	Refs []types.InlineKBRef `json:"refs"`
}

func (h *DocIntegrationHandler) ResetInlineKB(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant"})
		return
	}
	var in inlineKBResetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := range in.Refs {
		in.Refs[i].TenantID = tenantID
	}
	refs := make([]*types.InlineKBRef, len(in.Refs))
	for i := range in.Refs {
		refs[i] = &in.Refs[i]
	}
	if err := h.svc.ResetInlineKBRefs(c, c.Param("page_id"), refs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reset": len(in.Refs)})
}

// --- AI Assistant Panel ---

func (h *DocIntegrationHandler) Assistant(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	var in types.DocAssistantRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.UserID == "" {
		if v, ok := c.Get("user_id"); ok {
			in.UserID, _ = v.(string)
		}
	}
	in.TenantID = tenantID
	resp, err := h.svc.Assistant(c, &in)
	if err != nil {
		status := http.StatusBadRequest
		if err == doc_integration.ErrUnknownMode || err == doc_integration.ErrInvalidRequest {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// silence unused-import warning if strconv gets dropped.
var _ = strconv.Itoa
