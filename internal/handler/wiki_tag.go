package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiTagHandler exposes the 8 REST endpoints documented in
// docs/comet/changes/weknora-wiki-tags/spec.md §2.1. The constructor
// takes the WikiTagService interface so tests can wire a fake.
//
// Build #17.
type WikiTagHandler struct {
	tagSvc interfaces.WikiTagService
}

// NewWikiTagHandler constructs the handler. The service is required —
// a nil service would panic on the first request, so we refuse to
// construct rather than silently misbehave.
func NewWikiTagHandler(tagSvc interfaces.WikiTagService) *WikiTagHandler {
	if tagSvc == nil {
		panic("handler.NewWikiTagHandler: tagSvc is required")
	}
	return &WikiTagHandler{tagSvc: tagSvc}
}

// validateKB extracts the kb_id path param and checks it against the
// gin context's KB middleware. Returns the kbID and a ready-to-use
// child context — same convention as the rest of the wiki handlers.
func (h *WikiTagHandler) validateKB(c *gin.Context) (string, bool) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return "", false
	}
	return kbID, true
}

// ListTags — GET /api/v1/knowledge-bases/:kb_id/wiki/tags
func (h *WikiTagHandler) ListTags(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	tags, err := h.tagSvc.List(c.Request.Context(), kbID)
	if err != nil {
		logger.Errorf(c.Request.Context(), "wiki tag list failed: kb=%s err=%v", kbID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []types.WikiTagWithCount{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// CreateTag — POST /api/v1/knowledge-bases/:kb_id/wiki/tags
func (h *WikiTagHandler) CreateTag(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	var req types.WikiTagCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	tag, err := h.tagSvc.Create(c.Request.Context(), kbID, req.Name, req.Color)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	c.JSON(http.StatusCreated, tag)
}

// GetTag — GET /api/v1/knowledge-bases/:kb_id/wiki/tags/:tag_id
func (h *WikiTagHandler) GetTag(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	tagID := c.Param("tag_id")
	if tagID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id is required"})
		return
	}
	tag, err := h.tagSvc.Get(c.Request.Context(), kbID, tagID)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	c.JSON(http.StatusOK, tag)
}

// UpdateTag — PUT /api/v1/knowledge-bases/:kb_id/wiki/tags/:tag_id
func (h *WikiTagHandler) UpdateTag(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	tagID := c.Param("tag_id")
	if tagID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id is required"})
		return
	}
	var req types.WikiTagUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	tag, err := h.tagSvc.Update(c.Request.Context(), kbID, tagID, req)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	c.JSON(http.StatusOK, tag)
}

// DeleteTag — DELETE /api/v1/knowledge-bases/:kb_id/wiki/tags/:tag_id
func (h *WikiTagHandler) DeleteTag(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	tagID := c.Param("tag_id")
	if tagID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id is required"})
		return
	}
	if err := h.tagSvc.Delete(c.Request.Context(), kbID, tagID); err != nil {
		h.writeTagError(c, err)
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// GetPageTags — GET /api/v1/knowledge-bases/:kb_id/wiki/pages/:slug/tags
func (h *WikiTagHandler) GetPageTags(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	tags, err := h.tagSvc.GetPageTags(c.Request.Context(), kbID, slug)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	if tags == nil {
		tags = []types.WikiTag{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// SetPageTags — PUT /api/v1/knowledge-bases/:kb_id/wiki/pages/:slug/tags
func (h *WikiTagHandler) SetPageTags(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	var req types.WikiTagSetPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	tags, err := h.tagSvc.SetPageTags(c.Request.Context(), kbID, slug, req.TagIDs)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	if tags == nil {
		tags = []types.WikiTag{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// BatchTagPages — POST /api/v1/knowledge-bases/:kb_id/wiki/pages/batch-tag
//
// Mirrors the BatchMove / BatchDelete / BatchStatus dispatch: 200 with
// the per-row result for sync, 202 with the job envelope for async.
func (h *WikiTagHandler) BatchTagPages(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	var req types.WikiBatchTagBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	route, err := h.tagSvc.BatchTag(c.Request.Context(), kbID, req.Slugs, req.TagID, req.Op)
	if err != nil {
		h.writeTagError(c, err)
		return
	}
	switch route.Kind {
	case "sync":
		c.JSON(http.StatusOK, route.Result)
	case "job":
		c.JSON(http.StatusAccepted, gin.H{"job": route.Job, "kind": "job"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unknown batch route kind: " + route.Kind})
	}
}

// writeTagError translates the service sentinels into stable HTTP
// status codes. Mirrors the convention used by wiki_acl.go / wiki_page.go.
func (h *WikiTagHandler) writeTagError(c *gin.Context, err error) {
	switch {
	case types.IsWikiTagNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case types.IsWikiTagConflict(err):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case types.IsWikiTagLimitExceeded(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "tag_limit_exceeded"})
	case errors.Is(err, types.ErrWikiTagInvalidName),
		errors.Is(err, types.ErrWikiTagInvalidColor):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		logger.Errorf(c.Request.Context(), "wiki tag handler error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}