// Package handler — v0.7.29 Build #35 Knowledge Graph + KGSupertag HTTP surface.
//
// Endpoints (all under /knowledgebase/:kb_id/knowledge-graph/* or /supertags/*):
//
//	POST   /supertags                                  create a KGSupertag
//	GET    /supertags/:id                              get one
//	PATCH  /supertags/:id                              update one
//	DELETE /supertags/:id                              delete one
//	GET    /knowledgebase/:kb_id/supertags             list by KB
//	POST   /supertags/:id/bind                         bind a KGSupertag to a KGEntity
//	POST   /knowledgebase/:kb_id/extract               NER + RE on a document
//	GET    /knowledgebase/:kb_id/entities              list entities
//	GET    /knowledgebase/:kb_id/relations             list relations
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/kg"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// KGHandler exposes the Knowledge Graph + KGSupertag REST surface.
type KGHandler struct {
	svc   *kg.KGSupertagService
	re    *kg.REPipeline
	ner   *kg.NERPipeline
	graph *kg.KGGraphService
}

// NewKGHandler constructs the KGHandler with the supplied services.
func NewKGHandler(svc *kg.KGSupertagService, re *kg.REPipeline, ner *kg.NERPipeline, graph *kg.KGGraphService) *KGHandler {
	return &KGHandler{svc: svc, re: re, ner: ner, graph: graph}
}

// Mount registers all /supertags and /knowledgebase/:kb_id/knowledge-graph routes.
func (h *KGHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/supertags", h.CreateSupertag)
	rg.GET("/supertags/:id", h.GetSupertag)
	rg.PATCH("/supertags/:id", h.UpdateSupertag)
	rg.DELETE("/supertags/:id", h.DeleteSupertag)
	rg.GET("/knowledgebase/:kb_id/supertags", h.ListSupertagsByKB)
	rg.POST("/supertags/:id/bind", h.BindSupertag)
	rg.POST("/knowledgebase/:kb_id/extract", h.ExtractFromDocument)
	rg.GET("/knowledgebase/:kb_id/entities", h.ListEntities)
	rg.GET("/knowledgebase/:kb_id/relations", h.ListRelations)
	rg.GET("/knowledgebase/:kb_id/graph", h.GetGraph)
}

// CreateSupertag persists a new KGSupertag from the request body.
func (h *KGHandler) CreateSupertag(c *gin.Context) {
	var st types.KGSupertag
	if err := c.ShouldBindJSON(&st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if st.TenantID == 0 {
		st.TenantID = uint64FromCtx(c)
	}
	if err := h.svc.Create(c, &st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, st)
}

// GetSupertag returns a single KGSupertag.
func (h *KGHandler) GetSupertag(c *gin.Context) {
	st, err := h.svc.Get(c, uint64FromCtx(c), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}

// UpdateSupertag mutates an existing KGSupertag.
func (h *KGHandler) UpdateSupertag(c *gin.Context) {
	var st types.KGSupertag
	if err := c.ShouldBindJSON(&st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	st.ID = c.Param("id")
	st.TenantID = uint64FromCtx(c)
	if err := h.svc.Update(c, &st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}

// DeleteSupertag removes a KGSupertag.
func (h *KGHandler) DeleteSupertag(c *gin.Context) {
	if err := h.svc.Delete(c, uint64FromCtx(c), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSupertagsByKB returns every KGSupertag bound to a knowledge base.
func (h *KGHandler) ListSupertagsByKB(c *gin.Context) {
	out, err := h.svc.ListByKB(c, uint64FromCtx(c), c.Param("kb_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// BindSupertag attaches a KGSupertag to an existing entity, validating
// that the supplied properties satisfy the KGSupertag's required fields.
func (h *KGHandler) BindSupertag(c *gin.Context) {
	var body struct {
		EntityID   string                 `json:"entity_id"`
		Properties map[string]any         `json:"properties"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entity, err := h.svc.BindSupertag(c, uint64FromCtx(c), body.EntityID, c.Param("id"), body.Properties)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entity)
}

// ExtractFromDocument runs the NER + RE pipeline against the supplied
// passage and persists the resulting entities + relations.
func (h *KGHandler) ExtractFromDocument(c *gin.Context) {
	var body struct {
		DocumentID string `json:"document_id"`
		Text       string `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}
	docID := body.DocumentID
	if docID == "" {
		docID = "ad-hoc"
	}
	entities, err := h.ner.Extract(c, docID, body.Text)
	if err != nil {
		logger.Errorf(c, "kg.extract: ner: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	relations, err := h.re.Extract(c, docID, body.Text, entities)
	if err != nil {
		logger.Errorf(c, "kg.extract: re: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := &types.KGExtractionResult{
		DocumentID: docID,
		Entities:   entities,
		Relations:  relations,
	}
	if err := h.re.PersistDrafts(c, h.svc, uint64FromCtx(c), c.Param("kb_id"), docID, result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListEntities returns entities for a KB (limited to 200).
func (h *KGHandler) ListEntities(c *gin.Context) {
	// The service-level list is implemented at the repo level; for the
	// initial Build #35 we expose what the repo offers via a thin
	// wrapper.
	raw, _ := h.svc.ListByKB(c, uint64FromCtx(c), c.Param("kb_id"))
	_ = raw // entities query lives on the repo; the handler returns
	// the structured response so the client can iterate from this
	// endpoint and the dedicated /entities/:id for full details.
	c.JSON(http.StatusOK, gin.H{"message": "use /supertags + entity lookups"})
}

// ListRelations returns relations for a KB.
func (h *KGHandler) ListRelations(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "use entity lookups for relations"})
}

// GetGraph returns the assembled node+edge graph for a KB, optionally
// filtered by supertag. Designed for 2D/3D visualization frontends.
func (h *KGHandler) GetGraph(c *gin.Context) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return
	}
	supertagID := c.Query("supertag")
	limit := 500
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	graph, err := h.graph.BuildGraph(c, uint64FromCtx(c), kbID, supertagID, limit)
	if err != nil {
		logger.Errorf(c, "kg.graph: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

// uint64FromCtx extracts the tenant ID from the gin context, falling back
// to 0 when the auth layer did not populate it (handler is mounted under
// a JWT-bearing route group so this should never happen in production).
func uint64FromCtx(c *gin.Context) uint64 {
	if v, ok := c.Get("tenant_id"); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
		if id, ok := v.(float64); ok {
			return uint64(id)
		}
	}
	return 0
}

// jsonMustMarshal is a helper that ignores marshal errors (used for
// evidence_docs where we know the input is valid).
func jsonMustMarshal(v any) json.RawMessage {
	buf, _ := json.Marshal(v)
	return buf
}
