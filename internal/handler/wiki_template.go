package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiTemplateHandler exposes the 2 REST endpoints documented in
// docs/comet/changes/weknora-template-skeleton/spec.md §2.1. The
// constructor takes the WikiTemplateService interface so tests can wire
// a stub.
//
// Build #18.
type WikiTemplateHandler struct {
	tplSvc interfaces.WikiTemplateService
}

// NewWikiTemplateHandler constructs the handler. The service is
// required — a nil service would panic on the first request, so we
// refuse to construct rather than silently misbehave.
func NewWikiTemplateHandler(tplSvc interfaces.WikiTemplateService) *WikiTemplateHandler {
	if tplSvc == nil {
		panic("handler.NewWikiTemplateHandler: tplSvc is required")
	}
	return &WikiTemplateHandler{tplSvc: tplSvc}
}

// validateKB extracts the kb_id path param. Returns the kbID and
// whether the request should continue. Mirrors the convention used
// by wiki_tag.go / wiki_acl.go.
func (h *WikiTemplateHandler) validateKB(c *gin.Context) (string, bool) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return "", false
	}
	return kbID, true
}

// ApplyTemplate — POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/apply-template
//
// Body: types.WikiApplyTemplateRequest
// Response: types.WikiApplyTemplateResult (200) or error JSON.
func (h *WikiTemplateHandler) ApplyTemplate(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	var req types.WikiApplyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	result, err := h.tplSvc.ApplyTemplate(c.Request.Context(), kbID, slug, req)
	if err != nil {
		h.writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// PreviewTemplate — POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/preview-template
//
// Same body as ApplyTemplate. Returns the same result shape but with
// no DB writes — used by the dialog's "预览" button. Mirrors the
// Build #16 batch-preview-* guard.
func (h *WikiTemplateHandler) PreviewTemplate(c *gin.Context) {
	kbID, ok := h.validateKB(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	var req types.WikiApplyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	result, err := h.tplSvc.PreviewSkeleton(c.Request.Context(), kbID, slug, req)
	if err != nil {
		h.writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// writeTemplateError translates the service sentinels into stable
// HTTP status codes. Mirrors the convention used by wiki_tag.go and
// wiki_acl.go.
func (h *WikiTemplateHandler) writeTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, types.ErrWikiTemplateEmptySkeleton):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "empty_skeleton"})
	case errors.Is(err, types.ErrWikiTemplateOversizeSkeleton):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "oversize_skeleton"})
	case errors.Is(err, repository.ErrWikiPageNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		logger.Errorf(c.Request.Context(), "wiki template handler error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}