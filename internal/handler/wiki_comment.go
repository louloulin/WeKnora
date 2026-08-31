package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// WikiCommentHandler exposes the wiki-page-comment HTTP endpoints.
// Thin layer that pulls ids off the URL, hands them to the service, and
// maps service errors to HTTP status codes. Same conventions as
// WikiPageHandler / WikiAclHandler.
type WikiCommentHandler struct {
	svc *service.WikiPageCommentService
}

// NewWikiCommentHandler wires the handler.
func NewWikiCommentHandler(svc *service.WikiPageCommentService) *WikiCommentHandler {
	return &WikiCommentHandler{svc: svc}
}

// commentActorIDs resolves (tenantID, userID, isAdmin) from the gin
// context using the auth middleware conventions already used by
// WikiPageHandler. Returns "" if auth context is missing.
func commentActorIDs(c *gin.Context) (tenantID uint64, userID string, isAdmin bool) {
	if v, ok := c.Get("tenant_id"); ok {
		if n, ok := v.(uint64); ok {
			tenantID = n
		}
	}
	if v, ok := c.Get("user_id"); ok {
		if s, ok := v.(string); ok {
			userID = s
		}
	}
	if v, ok := c.Get("is_admin"); ok {
		if b, ok := v.(bool); ok {
			isAdmin = b
		}
	}
	return
}

// List handles GET /knowledgebase/:kb_id/wiki/pages/:page_id/comments
func (h *WikiCommentHandler) List(c *gin.Context) {
	kbID := c.Param("kb_id")
	pageID := c.Param("slug")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	comments, total, err := h.svc.ListByPage(c.Request.Context(), pageID, limit, offset)
	if err != nil {
		logger.Errorf(c.Request.Context(), "list wiki comments failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": comments,
		"total": total,
		"limit": limit,
		"offset": offset,
		"kb_id": kbID,
		"page_id": pageID,
	})
}

// Create handles POST /knowledgebase/:kb_id/wiki/pages/:page_id/comments
func (h *WikiCommentHandler) Create(c *gin.Context) {
	kbID := c.Param("kb_id")
	pageID := c.Param("slug")
	tenantID, userID, _ := commentActorIDs(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req types.CreateWikiCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.Create(c.Request.Context(), kbID, pageID, userID, tenantID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": comment})
}

// Update handles PATCH /knowledgebase/:kb_id/wiki/comments/:comment_id
func (h *WikiCommentHandler) Update(c *gin.Context) {
	commentID := c.Param("comment_id")
	_, userID, isAdmin := commentActorIDs(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req types.UpdateWikiCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.Update(c.Request.Context(), commentID, userID, isAdmin, req.Body)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, service.ErrCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author or a KB admin can edit"})
		default:
			logger.Errorf(c.Request.Context(), "update wiki comment failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update comment"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": comment})
}

// SetResolved handles PATCH /knowledgebase/:kb_id/wiki/comments/:comment_id/resolve
func (h *WikiCommentHandler) SetResolved(c *gin.Context) {
	commentID := c.Param("comment_id")
	_, userID, isAdmin := commentActorIDs(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req types.ResolveWikiCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.SetResolved(c.Request.Context(), commentID, userID, isAdmin, req.Resolved)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, service.ErrCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author or a KB admin can resolve"})
		default:
			logger.Errorf(c.Request.Context(), "resolve wiki comment failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve comment"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": comment})
}

// Delete handles DELETE /knowledgebase/:kb_id/wiki/comments/:comment_id
func (h *WikiCommentHandler) Delete(c *gin.Context) {
	commentID := c.Param("comment_id")
	_, userID, isAdmin := commentActorIDs(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	err := h.svc.Delete(c.Request.Context(), commentID, userID, isAdmin)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, service.ErrCommentForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author or a KB admin can delete"})
		default:
			logger.Errorf(c.Request.Context(), "delete wiki comment failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete comment"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
