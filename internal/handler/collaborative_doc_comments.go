// Package handler - v0.7.35 collab_doc_comments REST endpoints.
//
// Threaded comments for DOC / PPT / SHEET. A thread is anchored to a
// position in the document (paragraph index, shape id, cell ref) and
// carries one or more replies. Endpoints mirror the wiki comment API
// style (POST create, GET list, PATCH edit, DELETE remove).
package handler

import (
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"strings"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// CollabDocCommentHandler exposes the comment REST surface.
type CollabDocCommentHandler struct {
	svc       *service.CollabDocService
	notifSvc  interfaces.NotificationService
}

// NewCollabDocCommentHandler wires the comment handler.
// notifSvc is optional (may be nil in lite deployments) — when nil,
// @-mention notification fan-out is silently skipped.
func NewCollabDocCommentHandler(
	svc *service.CollabDocService,
	notifSvc interfaces.NotificationService,
) *CollabDocCommentHandler {
	return &CollabDocCommentHandler{svc: svc, notifSvc: notifSvc}
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
	tenantVal, tenantOK := c.Get(types.TenantIDContextKey.String())
	userVal, userOK := c.Get(types.UserIDContextKey.String())
	if !tenantOK || !userOK {
		return 0, 0, false
	}
	tenantID := collabCommentUint64(tenantVal)
	userID := collabCommentUint64(userVal)
	return tenantID, userID, tenantID > 0 && userID > 0
}

// The auth middleware stores tenant_id as uint64 and user_id as the user's
// UUID string. Comment endpoints must use the same stable numeric projection
// as the document and binary handlers when looking up ACL ownership.
func collabCommentUint64(value any) uint64 {
	switch typed := value.(type) {
	case uint64:
		return typed
	case uint:
		return uint64(typed)
	case uint32:
		return uint64(typed)
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case string:
		if typed == "" {
			return 0
		}
		hash := sha256.Sum256([]byte(typed))
		return binary.BigEndian.Uint64(hash[:8]) &^ (uint64(1) << 63)
	}
	return 0
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
	// v0.7.197 — @-mention fan-out: one notification per recipient.
	// Best-effort: failures here never fail the comment write itself.
	if h.notifSvc != nil && len(req.MentionedUserIDs) > 0 {
		authorIDStr := strconv.FormatUint(userID, 10)
		h.fanOutMentions(c, tenantID, authorIDStr, docID, name, comment, req.MentionedUserIDs)
	}
	c.JSON(http.StatusCreated, comment)
}

// fanOutMentions emits a single notification per @-mentioned recipient.
// It de-duplicates ids, skips the author themselves, and swallows each
// per-recipient error so one bad row cannot poison the rest. The
// notification kind is `wiki.mentioned` — reused for cross-product
// parity (wiki + collab docs share the same bell dropdown).
func (h *CollabDocCommentHandler) fanOutMentions(
	c *gin.Context,
	tenantID uint64,
	authorID string,
	docID string,
	authorName string,
	comment *types.CollabDocComment,
	rawIDs []string,
) {
	ctx := c.Request.Context()
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rid := range rawIDs {
		rid = strings.TrimSpace(rid)
		if rid == "" || rid == authorID {
			continue
		}
		if _, dup := seen[rid]; dup {
			continue
		}
		seen[rid] = struct{}{}

		body := comment.Body
		if len(body) > 140 {
			body = body[:140] + "…"
		}
		displayName := strings.TrimSpace(authorName)
		if displayName == "" {
			displayName = "某人"
		}
		title := displayName + " 在协作文档中提到了你"
		authorIDCopy := authorID
		n := &types.Notification{
			TenantID:        tenantID,
			RecipientUserID: rid,
			Kind:            "wiki.mentioned",
			Title:           title,
			Body:            body,
			ActorUserID:     &authorIDCopy,
			ResourceType:    "collab_doc",
			ResourceID:      docID,
			Status:          types.NotificationStatusUnread,
		}
		if err := h.notifSvc.Create(ctx, n); err != nil {
			logger.Warnf(ctx, "[collab_comment] mention fan-out failed: doc=%s recipient=%s err=%v",
				docID, rid, err)
		}
	}
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
