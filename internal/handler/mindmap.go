// Package handler — Build #43 MindMap REST surface.
//
// All routes are mounted under the API v1 group and require auth middleware.
//
//   POST   /api/v1/mindmaps                          create a mindmap
//   GET    /api/v1/mindmaps/:id                      read
//   PATCH  /api/v1/mindmaps/:id                      update
//   DELETE /api/v1/mindmaps/:id                      delete (with nodes)
//   GET    /api/v1/mindmaps                          list with filters
//   POST   /api/v1/mindmaps/:id/nodes                create a node
//   PATCH  /api/v1/mindmaps/:id/nodes/:nodeID        update a node
//   DELETE /api/v1/mindmaps/:id/nodes/:nodeID        delete a node
//   GET    /api/v1/mindmaps/:id/nodes                list nodes
//   POST   /api/v1/mindmaps/:id/auto-layout          run auto-layout
//   GET    /api/v1/mindmaps/:id/export?format=...    export Markdown / OPML / XMind
package handler

import (
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service/mindmap"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// MindMapHandler exposes the MindMap REST surface.
type MindMapHandler struct {
	svc *mindmap.MindMapService
}

// NewMindMapHandler constructs the handler.
func NewMindMapHandler(svc *mindmap.MindMapService) *MindMapHandler {
	return &MindMapHandler{svc: svc}
}

// Mount registers the MindMap routes on the supplied router group.
func (h *MindMapHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/mindmaps", h.Create)
	rg.GET("/mindmaps", h.List)
	rg.GET("/mindmaps/:id", h.Get)
	rg.PATCH("/mindmaps/:id", h.Update)
	rg.DELETE("/mindmaps/:id", h.Delete)
	rg.POST("/mindmaps/:id/nodes", h.CreateNode)
	rg.GET("/mindmaps/:id/nodes", h.ListNodes)
	rg.PATCH("/mindmaps/:id/nodes/:nodeID", h.UpdateNode)
	rg.DELETE("/mindmaps/:id/nodes/:nodeID", h.DeleteNode)
	rg.POST("/mindmaps/:id/auto-layout", h.AutoLayout)
	rg.GET("/mindmaps/:id/export", h.Export)
}

// Create handles POST /mindmaps.
func (h *MindMapHandler) Create(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.CreateMindMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.CreateMindMap(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

// Get handles GET /mindmaps/:id.
func (h *MindMapHandler) Get(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	m, err := h.svc.GetMindMap(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mindmap not found"})
		return
	}
	c.JSON(http.StatusOK, m)
}

// Update handles PATCH /mindmaps/:id.
func (h *MindMapHandler) Update(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.UpdateMindMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	m, err := h.svc.UpdateMindMap(c.Request.Context(), tenantID, userID, c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mindmap not found"})
		return
	}
	c.JSON(http.StatusOK, m)
}

// Delete handles DELETE /mindmaps/:id.
func (h *MindMapHandler) Delete(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteMindMap(c.Request.Context(), tenantID, userID, c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// List handles GET /mindmaps.
func (h *MindMapHandler) List(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	filter := types.ListMindMapsFilter{
		KBID:       c.Query("kb_id"),
		Visibility: c.Query("visibility"),
	}
	if v := c.Query("owner_user_id"); v != "" {
		var u uint64
		_, err := fmt.Sscanf(v, "%d", &u)
		if err == nil {
			filter.OwnerUserID = u
		}
	}
	filter.Limit = 100
	filter.Offset = 0
	maps, err := h.svc.ListMindMaps(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	count, err := h.svc.CountMindMaps(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": maps, "total": count})
}

// CreateNode handles POST /mindmaps/:id/nodes.
func (h *MindMapHandler) CreateNode(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.CreateMindMapNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n, err := h.svc.CreateNode(c.Request.Context(), tenantID, userID, c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, n)
}

// ListNodes handles GET /mindmaps/:id/nodes.
func (h *MindMapHandler) ListNodes(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	nodes, err := h.svc.ListNodes(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": nodes})
}

// UpdateNode handles PATCH /mindmaps/:id/nodes/:nodeID.
func (h *MindMapHandler) UpdateNode(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.UpdateMindMapNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n, err := h.svc.UpdateNode(c.Request.Context(), tenantID, userID, c.Param("id"), c.Param("nodeID"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if n == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}
	c.JSON(http.StatusOK, n)
}

// DeleteNode handles DELETE /mindmaps/:id/nodes/:nodeID.
func (h *MindMapHandler) DeleteNode(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteNode(c.Request.Context(), tenantID, userID, c.Param("id"), c.Param("nodeID")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// AutoLayout handles POST /mindmaps/:id/auto-layout.
func (h *MindMapHandler) AutoLayout(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.AutoLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	nodes, err := h.svc.AutoLayout(c.Request.Context(), tenantID, userID, c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": nodes})
}

// Export handles GET /mindmaps/:id/export?format=markdown|opml|xmind.
func (h *MindMapHandler) Export(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	format := types.ExportFormat(c.Query("format"))
	if format == "" {
		format = types.ExportFormatMD
	}
	if !types.ValidExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format is invalid (markdown|opml|xmind|png|svg)"})
		return
	}
	mapID := c.Param("id")
	var (
		body    string
		err     error
		content string
	)
	switch format {
	case types.ExportFormatMD:
		content, err = h.svc.ExportMarkdown(c.Request.Context(), tenantID, mapID)
		body = "text/markdown; charset=utf-8"
	case types.ExportFormatOPML:
		content, err = h.svc.ExportOPML(c.Request.Context(), tenantID, mapID)
		body = "application/xml; charset=utf-8"
	case types.ExportFormatXMIND:
		content, err = h.svc.ExportXMind(c.Request.Context(), tenantID, mapID)
		body = "application/json; charset=utf-8"
	default:
		// PNG / SVG are rendered by the client; we just emit a hint.
		content = fmt.Sprintf(`{"format":"%s","note":"render client-side","map_id":"%s"}`, format, mapID)
		body = "application/json; charset=utf-8"
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logger.Debugf(c, "[mindmap] export format=%s map=%s bytes=%d", format, mapID, len(content))
	c.Data(http.StatusOK, body, []byte(content))
}

// tenantAndUser extracts tenant_id + user_id from the gin context. Returns
// ok=false if either is missing (handler short-circuits with 401).
func (h *MindMapHandler) tenantAndUser(c *gin.Context) (uint64, uint64, bool) {
	tv, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant_id"})
		return 0, 0, false
	}
	uv, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user_id"})
		return 0, 0, false
	}
	tenantID, _ := tv.(uint64)
	userID, _ := uv.(uint64)
	if tenantID == 0 || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth context"})
		return 0, 0, false
	}
	return tenantID, userID, true
}
