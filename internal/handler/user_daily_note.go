package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// UserDailyNoteHandler exposes the Build #45.a Daily Note 默认页 REST
// surface. Every endpoint derives (user_id, tenant_id) from the
// auth context; the request body never carries them so a token
// can't be used to read or write another user's note.
//
// Endpoints (mounted at /api/v1/user/daily-notes):
//
//   GET    /today?kb_id=...           → today's note (GetOrCreate)
//   GET    /today?date=YYYY-MM-DD     → date-pinned variant
//   GET    /?kb_id=&from=&to=         → range listing
//   PATCH  /:id                       → title + content + summary
//   GET    /count?kb_id=&from=&to=    → row count (dashboard widget)
type UserDailyNoteHandler struct {
	svc *service.UserDailyNoteService
}

// NewUserDailyNoteHandler wires the handler against the service.
func NewUserDailyNoteHandler(svc *service.UserDailyNoteService) *UserDailyNoteHandler {
	return &UserDailyNoteHandler{svc: svc}
}

// GetOrCreateToday godoc
// @Summary      Get or create today's daily note
// @Description  Build #45.a. Returns the user's note for the current UTC
// @Description  calendar date in the given KB, creating an empty stub
// @Description  on the first call of the day.
// @Tags         UserDailyNote
// @Produce      json
// @Param        kb_id  query  string  true  "Knowledge base ID"
// @Success      200  {object}  types.UserDailyNote
// @Security     Bearer
// @Router       /user/daily-notes/today [get]
func (h *UserDailyNoteHandler) GetOrCreateToday(c *gin.Context) {
	kbID := c.Query("kb_id")
	tenantID, userID, ok := h.resolveCaller(c)
	if !ok {
		return
	}
	note, err := h.svc.GetOrCreateToday(c.Request.Context(), tenantID, userID, kbID)
	if err != nil {
		writeDailyNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, note)
}

// GetOrCreateDate godoc
// @Summary      Get or create a date-pinned daily note
// @Tags         UserDailyNote
// @Produce      json
// @Param        kb_id  query  string  true  "Knowledge base ID"
// @Param        date   query  string  true  "Calendar date (YYYY-MM-DD)"
// @Success      200  {object}  types.UserDailyNote
// @Security     Bearer
// @Router       /user/daily-notes/date [get]
func (h *UserDailyNoteHandler) GetOrCreateDate(c *gin.Context) {
	kbID := c.Query("kb_id")
	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required (YYYY-MM-DD)"})
		return
	}
	day, perr := time.Parse("2006-01-02", dateStr)
	if perr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}
	tenantID, userID, ok := h.resolveCaller(c)
	if !ok {
		return
	}
	note, err := h.svc.GetOrCreateDate(c.Request.Context(), tenantID, userID, kbID, day)
	if err != nil {
		writeDailyNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, note)
}

// ListRange godoc
// @Summary      List daily notes in a date range
// @Tags         UserDailyNote
// @Produce      json
// @Param        kb_id  query  string  true  "Knowledge base ID"
// @Param        from   query  string  true  "Start date (YYYY-MM-DD, inclusive)"
// @Param        to     query  string  true  "End date (YYYY-MM-DD, inclusive)"
// @Param        limit  query  int     false "Max rows (default 30, max 365)"
// @Success      200  {object}  types.DailyNoteListResponse
// @Security     Bearer
// @Router       /user/daily-notes [get]
func (h *UserDailyNoteHandler) ListRange(c *gin.Context) {
	kbID := c.Query("kb_id")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			limit = parsed
		}
	}
	from, ferr := time.Parse("2006-01-02", fromStr)
	if ferr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
		return
	}
	to, terr := time.Parse("2006-01-02", toStr)
	if terr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
		return
	}
	_, userID, ok := h.resolveCaller(c)
	if !ok {
		return
	}
	notes, err := h.svc.ListRange(c.Request.Context(), userID, kbID, from, to, limit)
	if err != nil {
		writeDailyNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.DailyNoteListResponse{Notes: notes, Total: int64(len(notes))})
}

// CountRange godoc
// @Summary      Count daily notes in a date range
// @Tags         UserDailyNote
// @Produce      json
// @Param        kb_id  query  string  true  "Knowledge base ID"
// @Param        from   query  string  true  "Start date (YYYY-MM-DD)"
// @Param        to     query  string  true  "End date (YYYY-MM-DD)"
// @Success      200  {object}  object
// @Security     Bearer
// @Router       /user/daily-notes/count [get]
func (h *UserDailyNoteHandler) CountRange(c *gin.Context) {
	kbID := c.Query("kb_id")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	from, ferr := time.Parse("2006-01-02", fromStr)
	if ferr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from must be YYYY-MM-DD"})
		return
	}
	to, terr := time.Parse("2006-01-02", toStr)
	if terr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to must be YYYY-MM-DD"})
		return
	}
	_, userID, ok := h.resolveCaller(c)
	if !ok {
		return
	}
	n, err := h.svc.CountRange(c.Request.Context(), userID, kbID, from, to)
	if err != nil {
		writeDailyNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"kb_id": kbID, "from": fromStr, "to": toStr, "count": n})
}

// UpdateNoteRequest is the PATCH body. All three fields are required
// to keep the front-end patch surface small — the request is a full
// replacement of the mutable triple, not a partial merge.
type UpdateNoteRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content"`
	Summary string `json:"summary"`
}

// UpdateContent godoc
// @Summary      Update a daily note's title / content / summary
// @Tags         UserDailyNote
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Note ID"
// @Param        body  body  UpdateNoteRequest  true  "Updated fields"
// @Success      200  {object}  types.UserDailyNote
// @Security     Bearer
// @Router       /user/daily-notes/{id} [patch]
func (h *UserDailyNoteHandler) UpdateContent(c *gin.Context) {
	noteID := c.Param("id")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, userID, ok := h.resolveCaller(c)
	if !ok {
		return
	}
	note, err := h.svc.UpdateContent(c.Request.Context(), userID, noteID, req.Title, req.Content, req.Summary)
	if err != nil {
		writeDailyNoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, note)
}

// resolveCaller pulls the tenant id and user id from the gin context.
// Writes 401 if either is missing — daily notes are scoped per user
// and per tenant so an unauthenticated request can't sneak through.
func (h *UserDailyNoteHandler) resolveCaller(c *gin.Context) (uint64, string, bool) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID, ok := types.UserIDFromContext(c.Request.Context())
	if !ok || userID == "" || tenantID == 0 {
		// Fall back to X-User-ID / X-Tenant-ID for the smoke scripts
		// that bypass the auth middleware.
		if v := c.GetHeader("X-User-ID"); v != "" && userID == "" {
			userID = v
		}
		if v := c.GetHeader("X-Tenant-ID"); v != "" && tenantID == 0 {
			if parsed, perr := strconv.ParseUint(v, 10, 64); perr == nil {
				tenantID = parsed
			}
		}
	}
	if userID == "" || tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user and tenant required"})
		return 0, "", false
	}
	return tenantID, userID, true
}

// writeDailyNoteError maps the service / repo error vocabulary to
// HTTP status codes. Centralised so the four handler methods stay
// one-liner happy-paths.
func writeDailyNoteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrUserDailyNoteNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "daily note not found"})
	case errors.Is(err, service.ErrUserDailyNoteKBRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrUserDailyNoteRangeInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
