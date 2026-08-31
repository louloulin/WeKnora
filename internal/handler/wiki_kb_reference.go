package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiKBReferenceHandler exposes the doc + KB integration glue over HTTP.
//
// Route surface (registered via routes_wiki_kb_reference.go):
//
//	POST   /api/v1/wiki/pages/:id/references/kb         add KB reference
//	DELETE /api/v1/wiki/pages/:id/references/kb/:kbId   remove KB reference
//	GET    /api/v1/wiki/pages/:id/references/kb         list page references
//	GET    /api/v1/wiki/pages/:id/references/kb/:kbId   resolve one reference
//	GET    /api/v1/knowledge/:id/references/wiki        list wiki backlinks
//
// All endpoints are tenant-scoped — the handler reads TenantID from the
// gin context (set by the auth middleware) and never trusts the URL
// for tenant scoping, which would be an IDOR waiting to happen.
type WikiKBReferenceHandler struct {
	svc *service.KnowledgeReferenceService
}

// NewWikiKBReferenceHandler is the constructor wired into the container.
// The service is the only required dependency — every read goes through
// it so the soft-delete / status decoration lives in one place.
func NewWikiKBReferenceHandler(svc *service.KnowledgeReferenceService) *WikiKBReferenceHandler {
	return &WikiKBReferenceHandler{svc: svc}
}

// WikiKBReferenceRequest is the wire body for POST add. The author can
// include a human-readable label (the text between the [[kb:id]]
// delimiters) so the audit log keeps what they actually typed.
type WikiKBReferenceRequest struct {
	KnowledgeID    string `json:"knowledge_id" binding:"required"`
	ReferenceLabel string `json:"reference_label"`
}

// AddReference handles POST /api/v1/wiki/pages/:id/references/kb.
// Idempotent: a second POST with the same knowledge_id refreshes
// reference_label and bumps updated_at.
func (h *WikiKBReferenceHandler) AddReference(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	wikiPageID := c.Param("id")
	var req WikiKBReferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := c.GetString("user_id")
	ref, err := h.svc.AddReference(c.Request.Context(),
		tenantID, wikiPageID, req.KnowledgeID, req.ReferenceLabel, userID)
	if err != nil {
		writeKnowledgeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ref)
}

// RemoveReference handles DELETE. Soft-delete on the row, idempotent.
func (h *WikiKBReferenceHandler) RemoveReference(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	wikiPageID := c.Param("id")
	kbID := c.Param("kbId")
	if err := h.svc.RemoveReference(c.Request.Context(), tenantID, wikiPageID, kbID); err != nil {
		writeKnowledgeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ListForWikiPage handles GET. Supports limit/offset via query string.
func (h *WikiKBReferenceHandler) ListForWikiPage(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	wikiPageID := c.Param("id")
	limit, offset := parseKnowledgeReferencePagination(c)
	rows, err := h.svc.ListForWikiPage(c.Request.Context(), tenantID, wikiPageID, limit, offset)
	if err != nil {
		writeKnowledgeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": len(rows)})
}

// ListForKnowledge handles the inverse GET — the KB document viewer's
// "Mentioned in Wiki Pages" sidebar.
func (h *WikiKBReferenceHandler) ListForKnowledge(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	kbID := c.Param("id")
	limit, offset := parseKnowledgeReferencePagination(c)
	rows, err := h.svc.ListForKnowledge(c.Request.Context(), tenantID, kbID, limit, offset)
	if err != nil {
		writeKnowledgeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": len(rows)})
}

// ResolveReference handles GET-by-id. The renderer calls this when
// the wiki page contains a single [[kb:id]] mention and needs to know
// the KB title / snippet for an inline card.
func (h *WikiKBReferenceHandler) ResolveReference(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing"})
		return
	}
	wikiPageID := c.Param("id")
	kbID := c.Param("kbId")
	row, err := h.svc.ResolveReference(c.Request.Context(), tenantID, wikiPageID, kbID)
	if err != nil {
		writeKnowledgeReferenceError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

// writeKnowledgeReferenceError is the single place that maps service /
// repository sentinels to HTTP status codes. Keeping the mapping here
// (rather than scattered in each handler) means adding a new error is
// a one-line change.
//
// Sentinel → status:
//
//	ErrKnowledgeReferenceNotFound → 404
//	ErrWikiKBReferenceNotFound    → 404
//	anything else                 → 500
func writeKnowledgeReferenceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrKnowledgeReferenceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, interfaces.ErrWikiKBReferenceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "reference not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// parsePagination reads limit / offset from the gin context with sane
// defaults: limit=50, offset=0. Returning 0/0 is also acceptable (the
// service treats 0 as "no cap"); we keep the parse simple and reject
// negatives rather than silently flipping them, because the KB viewer
// sends very small offsets in practice.
func parseKnowledgeReferencePagination(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// Compile-time guard: keep the types package import live in case a
// future refactor drops the only reference. Cheap insurance against
// the "unused import" footgun when the handler is trimmed.
var _ = types.WikiKBReference{}
