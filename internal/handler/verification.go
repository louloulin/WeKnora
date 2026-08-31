package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/application/service/verification"
	"github.com/gin-gonic/gin"
)

// VerificationHandler exposes the Build #29 AI Verification scanner
// behind two REST endpoints: a per-page check and a per-KB rollup.
type VerificationHandler struct {
	svc *verification.Service
}

// NewVerificationHandler wires the handler.
func NewVerificationHandler(svc *verification.Service) *VerificationHandler {
	return &VerificationHandler{svc: svc}
}

// RunForPage godoc
// @Summary      Run AI verification on a wiki page
// @Description  Build #29. Runs the freshness / contradiction / link
// @Description  health / trust-score checks on the given slug and returns
// @Description  the per-page verification report.
// @Tags         WikiVerification
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        slug   path  string  true  "Page slug"
// @Success      200  {object}  types.VerificationReport
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/pages/{slug}/verification [get]
func (h *VerificationHandler) RunForPage(c *gin.Context) {
	kbID := c.Param("kb_id")
	slug := c.Param("slug")
	if kbID == "" || slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id and slug are required"})
		return
	}
	report, err := h.svc.RunForPage(c.Request.Context(), kbID, slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

// RunForKB godoc
// @Summary      Run AI verification on every page in a KB
// @Description  Build #29. Scans every slug in the KB and returns the
// @Description  per-page reports plus a summary rollup. `limit` is a
// @Description  safety cap (default 100, max 1000) for callers that
// @Description  want to throttle the run.
// @Tags         WikiVerification
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Param        limit  query  int     false "Max pages to scan"
// @Success      200  {object}  verification.KBSummaryResponse
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/wiki/verification [get]
func (h *VerificationHandler) RunForKB(c *gin.Context) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id is required"})
		return
	}
	limit := 100
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	reports, summary, err := h.svc.RunForKB(c.Request.Context(), kbID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"reports": reports,
		"summary": summary,
	})
}

var _ = types.VerificationStatusOK
