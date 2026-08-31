// Package handler - v0.7.35 collab_doc_comments REST endpoints.
//
// Threaded comments for DOC / PPT / SHEET. A thread is anchored to a
// position in the document (paragraph index, shape id, cell ref) and
// carries one or more replies. Endpoints mirror the wiki comment API
// style (POST create, GET list, PATCH edit, DELETE remove).
package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// CollabDocCommentHandler exposes the comment REST surface.
type CollabDocCommentHandler struct {
	svc *service.CollabDocService
}

// NewCollabDocCommentHandler wires the comment handler.
func NewCollabDocCommentHandler(svc *service.CollabDocService) *CollabDocCommentHandler {
	return &CollabDocCommentHandler{svc: svc}
}

// Register attaches the comment routes under the existing collab-docs router.
// Caller must pass the auth middleware (Bearer token → user id) so we can
// stamp author_user_id on each comment.
func (h *CollabDocCommentHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/collaborative-docs/:id/comments", h.Create)
	rg.GET("/collaborative-docs/:id/comments", h.List)
	rg.PATCH("/collaborative-docs/:id/comments/:commentID", h.Update)
	rg.DELETE("/collaborative-docs/:id/comments/:commentID", h.Delete)
}

// helper: extract tenant + user from the gin context (set by the auth
// middleware earlier in the chain).
func collabCommentCaller(c *gin.Context) (uint64, uint64, bool) {
	tenantVal, tenantOK := c.Get("tenant_id")
	userVal, userOK := c.Get("user_id")
	if !tenantOK || !userOK {
		return 0, 0, false
	}
	tenantID, _ := tenantVal.(uint64)
	userID, _ := userVal.(uint64)
	return tenantID, userID, tenantID > 0 && userID > 0
}

// helper: extract display name + color from the auth middleware. The
// middleware writes these when it stamps the request context.
func collabCommentAuthor(c *gin.Context) (string, string) {
	name, _ := c.Get("user_name")
	color, _ := c.Get("user_color")
	nm, _ := name.(string)
	cl, _ := color.(string)
	if cl == "" {
		cl = "#58a6ff"
	}
	return nm, cl
}

func (h *CollabDocCommentHandler) Create(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	docID := c.Param("id")
	var req types.CreateCollabDocCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, color := collabCommentAuthor(c)
	comment, err := h.svc.AddComment(c.Request.Context(), tenantID, userID, docID, name, color, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, comment)
}

func (h *CollabDocCommentHandler) List(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	docID := c.Param("id")
	filter := types.ListCollabDocCommentsFilter{
		ThreadID: c.Query("thread_id"),
	}
	if r := c.Query("resolved"); r != "" {
		if r == "true" {
			t := true
			filter.Resolved = &t
		} else if r == "false" {
			f := false
			filter.Resolved = &f
		}
	}
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			filter.Limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			filter.Offset = v
		}
	}
	rows, err := h.svc.ListComments(c.Request.Context(), tenantID, userID, docID, filter)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comments": rows})
}

func (h *CollabDocCommentHandler) Update(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	docID := c.Param("id")
	commentID, err := strconv.ParseUint(c.Param("commentID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commentID invalid"})
		return
	}
	var req types.UpdateCollabDocCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comment, err := h.svc.UpdateComment(c.Request.Context(), tenantID, userID, docID, commentID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, comment)
}

func (h *CollabDocCommentHandler) Delete(c *gin.Context) {
	tenantID, userID, ok := collabCommentCaller(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	docID := c.Param("id")
	commentID, err := strconv.ParseUint(c.Param("commentID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commentID invalid"})
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), tenantID, userID, docID, commentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}
