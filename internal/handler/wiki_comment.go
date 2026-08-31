package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiCommentHandler exposes the 5 REST endpoints documented in
// docs/comet/changes/weknora-wiki-page-comments/spec.md §2.1. The
// constructor takes the WikiCommentService interface so tests can wire
// a fake.
//
// Build #22 (v0.7.25).
type WikiCommentHandler struct {
	svc interfaces.WikiCommentService
}

// NewWikiCommentHandler constructs the handler. A nil service is
// rejected so a missing DI wiring fails loudly at startup.
func NewWikiCommentHandler(svc interfaces.WikiCommentService) *WikiCommentHandler {
	if svc == nil {
		panic("handler.NewWikiCommentHandler: svc is required")
	}
	return &WikiCommentHandler{svc: svc}
}

// validateKB extracts the kb_id path param + tenant_id / user_id from
// the gin context, returning typed values ready for the service layer.
func (h *WikiCommentHandler) validate(c *gin.Context) (uint64, string, string, bool) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return 0, "", "", false
	}
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return 0, "", "", false
	}
	return tenantID, kbID, userID, true
}

// isOwnerOrAdmin inspects the gin context role marker set by the rbac
// guard. It is a best-effort check: production deployments should also
// pass through the wikiAclHandler.GetAcl response to confirm ownership.
func (h *WikiCommentHandler) isOwnerOrAdmin(c *gin.Context) bool {
	role := c.GetString("role")
	return role == "owner" || role == "admin" || role == "super_admin"
}

// List — GET /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments
func (h *WikiCommentHandler) List(c *gin.Context) {
	_, kbID, _, ok := h.validate(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	resp, err := h.svc.List(c.Request.Context(), 0, kbID, slug)
	if err != nil {
		logger.Errorf(c.Request.Context(), "wiki comment list failed: kb=%s slug=%s err=%v", kbID, slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Create — POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments
func (h *WikiCommentHandler) Create(c *gin.Context) {
	tenantID, kbID, userID, ok := h.validate(c)
	if !ok {
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}
	var req types.WikiCommentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.Create(
		c.Request.Context(),
		tenantID,
		kbID,
		slug,
		userID,
		c.GetString("user_name"),
		c.GetString("user_avatar_url"),
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, interfaces.ErrWikiCommentBadInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "wiki comment create failed: kb=%s slug=%s err=%v", kbID, slug, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, comment)
}

// Update — PUT /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments/:comment_id
func (h *WikiCommentHandler) Update(c *gin.Context) {
	_, kbID, userID, ok := h.validate(c)
	if !ok {
		return
	}
	commentID := c.Param("comment_id")
	if commentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment_id is required"})
		return
	}
	var req types.WikiCommentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.Update(c.Request.Context(), 0, kbID, commentID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, interfaces.ErrWikiCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, interfaces.ErrWikiCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, interfaces.ErrWikiCommentBadInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "wiki comment update failed: comment=%s err=%v", commentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, comment)
}

// SetResolved — POST /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments/:comment_id/resolve
func (h *WikiCommentHandler) SetResolved(c *gin.Context) {
	_, kbID, userID, ok := h.validate(c)
	if !ok {
		return
	}
	commentID := c.Param("comment_id")
	if commentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment_id is required"})
		return
	}
	var req types.WikiCommentResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.SetResolved(
		c.Request.Context(),
		0,
		kbID,
		commentID,
		userID,
		h.isOwnerOrAdmin(c),
		req.Resolved,
	)
	if err != nil {
		switch {
		case errors.Is(err, interfaces.ErrWikiCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, interfaces.ErrWikiCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "wiki comment resolve failed: comment=%s err=%v", commentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, comment)
}

// Delete — DELETE /api/v1/knowledgebase/:kb_id/wiki/pages/:slug/comments/:comment_id
func (h *WikiCommentHandler) Delete(c *gin.Context) {
	_, kbID, userID, ok := h.validate(c)
	if !ok {
		return
	}
	commentID := c.Param("comment_id")
	if commentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment_id is required"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), 0, kbID, commentID, userID, h.isOwnerOrAdmin(c)); err != nil {
		switch {
		case errors.Is(err, interfaces.ErrWikiCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, interfaces.ErrWikiCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "wiki comment delete failed: comment=%s err=%v", commentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
