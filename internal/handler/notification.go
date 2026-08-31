package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NotificationHandler is the HTTP entry point for the notification
// center. The bell dropdown in the top nav calls the routes here.
// All routes are tenant-scoped and require an authenticated user; the
// auth middleware fills tenant_id and user_id into the gin context.
type NotificationHandler struct {
	svc interfaces.NotificationService
}

// NewNotificationHandler wires the handler against the supplied
// service. Container.go owns the lifecycle.
func NewNotificationHandler(svc interfaces.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// List handles GET /notifications. Query parameters:
//
//	page        1-based page number (default 1)
//	page_size   items per page (default 20, max 100)
//	status      optional filter: unread | read | dismissed
//	kind        optional filter: one of the closed Kind set
//	since_days  optional: only rows newer than N days
func (h *NotificationHandler) List(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	q := types.NotificationListQuery{
		TenantID: tenantID,
		UserID:   userID,
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
	}
	if s := c.Query("status"); s != "" {
		st := types.NotificationStatus(s)
		if !st.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		q.Status = &st
	}
	if k := c.Query("kind"); k != "" {
		kd := types.NotificationKind(k)
		if !kd.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid kind"})
			return
		}
		q.Kind = &kd
	}
	if sd := c.Query("since_days"); sd != "" {
		if v, err := strconv.Atoi(sd); err == nil {
			q.SinceDays = v
		}
	}
	res, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		logger.Errorf(c.Request.Context(), "notification List failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// UnreadCount handles GET /notifications/unread-count. The bell polls
// this every 30s. The response is intentionally tiny so the polling
// cost is negligible even with thousands of active users.
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	count, err := h.svc.UnreadCount(c.Request.Context(), tenantID, userID)
	if err != nil {
		logger.Errorf(c.Request.Context(), "notification UnreadCount failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, types.NotificationUnreadCount{Count: count})
}

// MarkRead handles POST /notifications/:id/read.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), tenantID, userID, id); err != nil {
		writeNotificationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkDismissed handles POST /notifications/:id/dismiss.
func (h *NotificationHandler) MarkDismissed(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.MarkDismissed(c.Request.Context(), tenantID, userID, id); err != nil {
		writeNotificationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkAllRead handles POST /notifications/read-all.
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	count, err := h.svc.MarkAllRead(c.Request.Context(), tenantID, userID)
	if err != nil {
		logger.Errorf(c.Request.Context(), "notification MarkAllRead failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": count})
}

// DeleteHard handles DELETE /notifications/:id. Reserved for admin
// tooling / GDPR right-to-erasure; the bell dropdown does NOT call
// this endpoint (it uses MarkDismissed so the audit trail stays).
func (h *NotificationHandler) DeleteHard(c *gin.Context) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.DeleteHard(c.Request.Context(), tenantID, userID, id); err != nil {
		writeNotificationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writeNotificationError maps the service sentinel errors to HTTP
// status codes. The mapping is intentionally explicit so a new
// sentinel error fails loudly rather than falling through to 500.
func writeNotificationError(c *gin.Context, err error) {
	switch {
	case errorsIs(err, types.ErrNotificationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errorsIs(err, types.ErrNotificationForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errorsIs(err, types.ErrInvalidNotificationKind),
		errorsIs(err, types.ErrInvalidNotificationStatus),
		errorsIs(err, types.ErrInvalidNotificationTitle),
		errorsIs(err, types.ErrInvalidTenant),
		errorsIs(err, types.ErrInvalidUser):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		logger.Errorf(c.Request.Context(), "notification handler unexpected error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// atoiDefault parses s as int, returning def when empty or invalid.
// Kept local so the handler stays free of strconv import noise.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// errorsIs is a tiny shim around errors.Is so we do not need to
// import "errors" in this file just for the switch arms above.
func errorsIs(err, target error) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
