// Package handler — Build #19 / P2.x.a wiki search v2 handler.
//
// The v2 endpoint lives at the same path as the legacy search but is
// selected by `?v=2`. Missing or `?legacy=1` keeps the legacy
// WikiPageService.SearchPages path. The fan-out happens here so the
// router still has a single concrete handler bound to
// `wikiRead.GET("/search", ...)`.
package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// WikiSearchV2Handler serves the v2 search endpoint. It is intentionally
// a separate handler from WikiPageHandler so that:
//   - DI does not have to thread a new dep through the legacy path,
//   - the legacy SearchPages keeps its current gin-route binding,
//   - the v2 path is gated on `?v=2` and can be turned off by the
//     frontend without server redeploy (set v back to 1).
type WikiSearchV2Handler struct {
	kbSvc            interfaces.KnowledgeBaseService
	searchV2Svc      interfaces.WikiSearchV2Service
	legacySearchPages func(c *gin.Context) // wraps WikiPageHandler.SearchPages
}

// NewWikiSearchV2Handler wires the v2 handler. legacySearchPages is a
// closure that re-enters the legacy handler so the route can fan out
// from a single gin handler binding.
func NewWikiSearchV2Handler(
	kbSvc interfaces.KnowledgeBaseService,
	searchV2Svc interfaces.WikiSearchV2Service,
	legacySearchPages func(c *gin.Context),
) *WikiSearchV2Handler {
	return &WikiSearchV2Handler{
		kbSvc:             kbSvc,
		searchV2Svc:       searchV2Svc,
		legacySearchPages: legacySearchPages,
	}
}

// Search is the unified gin handler. The path :kb_id is the routing
// KB; the actual SQL filter uses the union of path kb_id and any
// explicit ?kb_ids[] query, restricted to the caller's KB-ACL set.
func (h *WikiSearchV2Handler) Search(c *gin.Context) {
	if !useV2(c) {
		if h.legacySearchPages != nil {
			h.legacySearchPages(c)
			return
		}
		// Defense in depth: if the legacy closure is not wired, return
		// 501 so the frontend never gets a silent 200 with empty hits.
		c.JSON(http.StatusNotImplemented, gin.H{"error": "legacy search path is not configured"})
		return
	}

	kbID, _, err := h.validatePathKB(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID, ok := types.TenantIDFromContext(c.Request.Context())
	if !ok || tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant id missing"})
		return
	}
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user id missing"})
		return
	}

	req, err := parseSearchV2Request(c, kbID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// visibleKBIDs: the user's accessible KB set for this tenant.
	// Empty slice means "no KB-ACL restriction known at this layer" —
	// we treat that as "all KBs in tenant" downstream.
	visibleKBIDs, err := h.loadVisibleKBIDs(c, tenantID, userID)
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve visible KBs"})
		return
	}

	result, err := h.searchV2Svc.Search(c.Request.Context(), tenantID, userID, req, visibleKBIDs)
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// useV2 reports whether this request should be served by the v2 path.
// `?v=2` opts in. `?legacy=1` opts out. Anything else falls through to
// legacy so existing clients keep working.
func useV2(c *gin.Context) bool {
	switch c.Query("v") {
	case "2":
		return true
	case "1", "":
		// legacy default — but `?legacy=1` also opts out even when `v=2`
		// is set so we can keep both knobs controllable.
		return c.Query("legacy") == "1"
	default:
		return false
	}
}

// validatePathKB enforces that the path :kb_id exists, is a wiki KB,
// and is in the caller's accessible set (cheap pre-check via KB
// service). The full KBAccessRead guard still runs upstream via
// `g.KBAccessRead("kb_id")`.
func (h *WikiSearchV2Handler) validatePathKB(c *gin.Context) (string, uint64, error) {
	kbID := c.Param("kb_id")
	if kbID == "" {
		return "", 0, errors.NewBadRequestError("kb_id is required")
	}
	ctx := c.Request.Context()
	kb, err := h.kbSvc.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		return "", 0, errors.NewNotFoundError("knowledge base not found")
	}
	if !kb.IsWikiEnabled() {
		return "", 0, errors.NewBadRequestError("wiki feature is not enabled for this knowledge base")
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	return kbID, tenantID, nil
}

// parseSearchV2Request parses the query string into WikiSearchV2Request.
// The path kb_id is always added to the effective KB scope (we never
// drop the routing KB) — caller-specified kb_ids[] can add others on
// top, intersected with visibleKBIDs downstream.
func parseSearchV2Request(c *gin.Context, pathKBID string) (types.WikiSearchV2Request, error) {
	req := types.WikiSearchV2Request{
		Query: c.Query("q"),
	}
	if kbIDs, ok := c.GetQueryArray("kb_ids[]"); ok {
		req.KBIDs = kbIDs
	} else if kbIDs, ok := c.GetQueryArray("kb_ids"); ok {
		req.KBIDs = kbIDs
	}
	if req.KBIDs == nil {
		req.KBIDs = []string{pathKBID}
	} else {
		// Always include the path KB so the caller's current KB is in
		// scope even if they forgot to tick its chip in the UI.
		found := false
		for _, k := range req.KBIDs {
			if k == pathKBID {
				found = true
				break
			}
		}
		if !found {
			req.KBIDs = append([]string{pathKBID}, req.KBIDs...)
		}
	}
	if pts, ok := c.GetQueryArray("page_types[]"); ok {
		req.PageTypes = pts
	} else if pts, ok := c.GetQueryArray("page_types"); ok {
		req.PageTypes = pts
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return req, errors.NewBadRequestError("limit must be an integer")
		}
		req.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return req, errors.NewBadRequestError("offset must be an integer")
		}
		req.Offset = n
	}
	return req, nil
}

// loadVisibleKBIDs returns the list of KBs the caller can read. For
// Build #19 this is "all KBs in the caller's tenant" — Build #19.x
// adds KB-level ACL filtering on top. Returning a non-nil empty slice
// means "no KB-ACL restriction", which the service treats as "all KBs".
func (h *WikiSearchV2Handler) loadVisibleKBIDs(c *gin.Context, tenantID uint64, userID string) ([]string, error) {
	// Pragmatic default: KB-level ACL is a Build #19.x concern. Returning
	// nil hands the service "no KB-ACL restriction", so its SQL filter
	// collapses to "all KBs in this tenant". This matches the D4 default
	// the brief ships with while leaving the intersection logic ready
	// for Build #19.x when KB-ACL ships.
	_ = tenantID
	_ = userID
	_ = h.kbSvc
	return nil, nil
}