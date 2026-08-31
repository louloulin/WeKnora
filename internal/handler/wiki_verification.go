package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// WikiVerificationHandler exposes the Build #48 Verified Knowledge
// Engine endpoints. These are layered on top of the Build #29 AI
// Verification scanner — the scanner reports WHAT is wrong, this
// handler records WHO verified and WHEN.
//
// Endpoints:
//
//   POST /knowledgebase/:kb_id/wiki/pages/:id/verification/verify
//     → stamps verified_at = now, verified_by = caller, advances
//       review_due_at by 90d unless a longer schedule is already set.
//
//   POST /knowledgebase/:kb_id/wiki/pages/:id/verification/review-due
//     → sets review_owner + review_due_at in one transaction.
//       Body: {"owner_id": "alice", "due_at": "2026-12-31T00:00:00Z"}
//
//   GET /knowledgebase/:kb_id/wiki/pages/:id/verification
//     → already handled by Build #29; we extend the report shape
//       with review_owner / verified_at / review_due_at so the UI
//       can render the VerifiedBadge without a second round-trip.
type WikiVerificationHandler struct {
	svc *service.WikiVerificationService
}

// NewWikiVerificationHandler wires the handler.
func NewWikiVerificationHandler(svc *service.WikiVerificationService) *WikiVerificationHandler {
	return &WikiVerificationHandler{svc: svc}
}

// VerifyPage godoc
// @Summary      Mark a wiki page as verified
// @Description  Build #48. Stamps verified_at = now and verified_by = caller.
// @Description  Advances review_due_at by 90 days (DefaultReviewInterval)
// @Description  unless a longer schedule is already pinned.
// @Tags         WikiVerification
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      200  {object}  types.WikiPage
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{id}/verification/verify [post]
func (h *WikiVerificationHandler) VerifyPage(c *gin.Context) {
	slug := c.Param("slug")
	kbID := c.Param("kb_id")
	if slug == "" || kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug and kb_id are required"})
		return
	}
	userID := callerUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user required"})
		return
	}
	if err := h.svc.MarkVerifiedBySlug(c.Request.Context(), kbID, slug, userID); err != nil {
		switch {
		case errors.Is(err, repository.ErrWikiPageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "wiki page not found"})
		case errors.Is(err, service.ErrWikiPageVerificationInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"verified_at": time.Now().UTC(), "verified_by": userID})
}

// SetReviewScheduleRequest is the request body for SetReviewSchedule.
type SetReviewScheduleRequest struct {
	OwnerID string    `json:"owner_id" binding:"required"`
	DueAt   time.Time `json:"due_at" binding:"required"`
	// ByUserID is optional; recorded as VerifiedBy on the page so the
	// audit trail captures who re-pinned the schedule.
	ByUserID string `json:"by_user_id,omitempty"`
}

// SetReviewSchedule godoc
// @Summary      Set or update a wiki page's review schedule
// @Description  Build #48. Writes review_owner + review_due_at in one shot.
// @Tags         WikiVerification
// @Accept       json
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Param        body   body  SetReviewScheduleRequest  true  "Schedule"
// @Success      200  {object}  types.WikiPage
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{id}/verification/review-due [post]
func (h *WikiVerificationHandler) SetReviewSchedule(c *gin.Context) {
	slug := c.Param("slug")
	kbID := c.Param("kb_id")
	if slug == "" || kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug and kb_id are required"})
		return
	}
	var req SetReviewScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// default ByUserID to the caller when not explicitly set
	if req.ByUserID == "" {
		req.ByUserID = callerUserID(c)
	}
	if err := h.svc.SetReviewScheduleBySlug(c.Request.Context(), kbID, slug, req.OwnerID, req.DueAt, req.ByUserID); err != nil {
		switch {
		case errors.Is(err, repository.ErrWikiPageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "wiki page not found"})
		case errors.Is(err, service.ErrWikiPageVerificationInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"review_owner": req.OwnerID, "review_due_at": req.DueAt})
}

// callerUserID extracts the user id from the request context, with
// the X-User-ID header as a test-harness fallback for end-to-end
// smoke scripts that bypass the auth middleware.
func callerUserID(c *gin.Context) string {
	if u, ok := types.UserIDFromContext(c.Request.Context()); ok && u != "" {
		return u
	}
	if v := c.GetHeader("X-User-ID"); v != "" {
		return v
	}
	return ""
}

