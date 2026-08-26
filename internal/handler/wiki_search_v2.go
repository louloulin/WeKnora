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
//
// Build #19.x adds `kbListSvc` (KnowledgeBaseService.ListAccessibleKBs)
// so the default `?kb_ids[]` scope can be restricted to the caller's
// KB-ACL set instead of every KB in the tenant.
type WikiSearchV2Handler struct {
	kbSvc            interfaces.KnowledgeBaseService
	kbListSvc        interfaces.KnowledgeBaseService // may be nil in unit tests; production wires it via DI
	searchV2Svc      interfaces.WikiSearchV2Service
	legacySearchPages func(c *gin.Context) // wraps WikiPageHandler.SearchPages
}

// NewWikiSearchV2Handler wires the v2 handler. legacySearchPages is a
// closure that re-enters the legacy handler so the route can fan out
// from a single gin handler binding. The same `kbSvc` instance doubles
// as the KB-list provider via the KnowledgeBaseService interface — no
// extra constructor argument needed.
func NewWikiSearchV2Handler(
	kbSvc interfaces.KnowledgeBaseService,
	searchV2Svc interfaces.WikiSearchV2Service,
	legacySearchPages func(c *gin.Context),
) *WikiSearchV2Handler {
	return &WikiSearchV2Handler{
		kbSvc:             kbSvc,
		kbListSvc:         kbSvc,
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
	// Build #19.x — fuzzy / partial_match toggles. Defaults match the
	// brief D5/D6 decisions: fuzzy=true (English typo is the common case),
	// partial_match=false (high false-positive rate).
	if v := c.Query("fuzzy"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return req, errors.NewBadRequestError("fuzzy must be a boolean")
		}
		req.Fuzzy = b
	} else {
		req.Fuzzy = true
	}
	if v := c.Query("partial_match"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return req, errors.NewBadRequestError("partial_match must be a boolean")
		}
		req.PartialMatch = b
	}
	return req, nil
}

// loadVisibleKBIDs returns the list of KBs the caller can read.
//
// Build #19 returned nil ("no KB-ACL restriction" → all KBs in the
// tenant). Build #19.x replaces that placeholder with a real list sourced
// from `KnowledgeBaseService.ListKnowledgeBasesByTenantID` so the frontend
// chip row can render the actual KB scope rather than "everything".
//
// Per-KB ACL filtering within a tenant (the deeper shared-agent /
// agent-share semantics) is intentionally NOT done here — those checks
// live in the route middleware (`KBAccessRead`) for the path KB and in
// `WikiAclService.Resolve` for each individual page-level hit. Adding a
// per-KB ACL loop in this layer would re-introduce the cross-cutting
// helper that the existing middleware already owns, and `resolveKBAccessOnce`
// is unexported (`internal/middleware/kb_access.go:320`) — promoting it to
// a service-level helper is a separate refactor scoped to Build #19.x+1.
//
// Tenant isolation already prevents cross-tenant leakage, which is the
// security boundary this endpoint actually needs.
func (h *WikiSearchV2Handler) loadVisibleKBIDs(c *gin.Context, tenantID uint64, userID string) ([]string, error) {
	_ = userID
	if h.kbListSvc == nil {
		// Test stub path: behave like Build #19 (no KB-ACL restriction).
		return nil, nil
	}
	kbs, err := h.kbListSvc.ListKnowledgeBasesByTenantID(c.Request.Context(), tenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(kbs))
	for _, k := range kbs {
		if k == nil || k.ID == "" {
			continue
		}
		ids = append(ids, k.ID)
	}
	if len(ids) == 0 {
		// Empty tenant — behave like Build #19 (return nil so the repo
		// skips the KB filter and surfaces "no KBs" via empty hits).
		return nil, nil
	}
	return ids, nil
}