package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/gin-gonic/gin"
)

// WikiSyncBlockHandler exposes the synced-block REST surface. All routes
// are tenant-scoped via the wiki guards upstream; this handler trusts
// the (tenant_id, user_id) injected by middleware and never reads those
// values from URL/body (IDOR-safe).
type WikiSyncBlockHandler struct {
	svc *service.WikiSyncBlockService
}

// NewWikiSyncBlockHandler wires the handler to the service.
func NewWikiSyncBlockHandler(svc *service.WikiSyncBlockService) *WikiSyncBlockHandler {
	return &WikiSyncBlockHandler{svc: svc}
}

// Mount registers the routes onto the wiki realtime group shape:
//
//	POST   /api/v1/knowledgebase/:kb_id/wiki/sync-blocks                 (create)
//	GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks                 (list for picker)
//	GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (get canonical)
//	PUT    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (update canonical)
//	DELETE /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id       (delete, mode=cascade|unlink)
//	GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id/stats (fan-out stats)
//	GET    /api/v1/knowledgebase/:kb_id/wiki/sync-blocks/:block_id/refs  (refs list)
//
// The SyncPageRefs service method is exposed via the existing wiki page
// save flow rather than a dedicated endpoint — the page service calls
// svc.SyncPageRefs after every successful save.
func (h *WikiSyncBlockHandler) Mount(rg *gin.RouterGroup) {
	g := rg.Group("/sync-blocks")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:block_id", h.Get)
	g.PUT("/:block_id", h.Update)
	g.DELETE("/:block_id", h.Delete)
	g.GET("/:block_id/stats", h.Stats)
	g.GET("/:block_id/refs", h.ListRefs)
}

// Create inserts a new canonical synced block.
//
//	POST /sync-blocks
//	body: {"block_id": "uuid", "title": "...", "content_json": {...}, "content_md": "..."}
func (h *WikiSyncBlockHandler) Create(c *gin.Context) {
	tenantID, userID, kbID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body struct {
		BlockID     string          `json:"block_id"`
		Title       string          `json:"title"`
		ContentJSON json.RawMessage `json:"content_json"`
		ContentMD   string          `json:"content_md"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	raw, err := service.ValidateJSONContent(body.ContentJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := service.MakeSyncBlockUpsert(tenantID, kbID, body.BlockID, body.Title, raw, body.ContentMD, userID)
	row, err := h.svc.CreateCanonical(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

// List returns canonical blocks for a KB (picker UI).
//
//	GET /sync-blocks?limit=50&offset=0
func (h *WikiSyncBlockHandler) List(c *gin.Context) {
	tenantID, _, kbID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	limit := syncPageAtoiDefault(c.Query("limit"), 50)
	offset := syncPageAtoiDefault(c.Query("offset"), 0)
	rows, err := h.svc.ListForKB(c.Request.Context(), tenantID, kbID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"blocks": rows, "limit": limit, "offset": offset})
}

// Get returns one canonical block.
//
//	GET /sync-blocks/:block_id
func (h *WikiSyncBlockHandler) Get(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	blockID := c.Param("block_id")
	row, err := h.svc.GetCanonical(c.Request.Context(), tenantID, blockID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync block not found"})
		return
	}
	c.JSON(http.StatusOK, row)
}

// Update replaces content on an existing synced block, bumping version.
//
//	PUT /sync-blocks/:block_id
//	body: {"title": "...", "content_json": {...}, "content_md": "..."}
func (h *WikiSyncBlockHandler) Update(c *gin.Context) {
	tenantID, userID, kbID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	blockID := c.Param("block_id")
	var body struct {
		Title       string          `json:"title"`
		ContentJSON json.RawMessage `json:"content_json"`
		ContentMD   string          `json:"content_md"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	raw, err := service.ValidateJSONContent(body.ContentJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := service.MakeSyncBlockUpsert(tenantID, kbID, blockID, body.Title, raw, body.ContentMD, userID)
	row, err := h.svc.UpdateCanonical(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

// Delete removes a canonical block. mode=cascade|unlink controls ref fate.
//
//	DELETE /sync-blocks/:block_id?mode=cascade
func (h *WikiSyncBlockHandler) Delete(c *gin.Context) {
	tenantID, userID, kbID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	blockID := c.Param("block_id")
	mode := c.DefaultQuery("mode", "cascade")
	if err := h.svc.DeleteCanonical(c.Request.Context(), tenantID, userID, kbID, blockID, mode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "block_id": blockID, "mode": mode})
}

// Stats returns fan-out reach for the picker UI badge.
//
//	GET /sync-blocks/:block_id/stats
func (h *WikiSyncBlockHandler) Stats(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	blockID := c.Param("block_id")
	stats, err := h.svc.Stats(c.Request.Context(), tenantID, blockID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if stats == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync block not found"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// ListRefs returns every page that embeds the block.
//
//	GET /sync-blocks/:block_id/refs
func (h *WikiSyncBlockHandler) ListRefs(c *gin.Context) {
	tenantID, _, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	blockID := c.Param("block_id")
	refs, err := h.svc.ListRefsForBlock(c.Request.Context(), tenantID, blockID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"refs": refs})
}

// resolveCtx pulls tenant/user/kb IDs from the gin context. The middleware
// upstream is responsible for putting these keys; we don't read them
// from URL/body to avoid IDOR.
func (h *WikiSyncBlockHandler) resolveCtx(c *gin.Context) (tenantID, userID uint64, kbID string, ok bool) {
	v, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return 0, 0, "", false
	}
	tid, ok := toUint64(v)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return 0, 0, "", false
	}
	v, exists = c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return 0, 0, "", false
	}
	uid, ok := toUint64(v)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user context"})
		return 0, 0, "", false
	}
	kbID = c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing kb_id"})
		return 0, 0, "", false
	}
	return tid, uid, kbID, true
}

func syncPageAtoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
