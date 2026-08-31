package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/database"
	"github.com/Tencent/WeKnora/internal/application/service/dockb"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// DockbHandler wires the v0.7.23 Doc ↔ KB AI Bridge + Database /
// 多维表 endpoints. Two sub-resources live on the same handler so
// the auth contract is identical — both read tenant_id from the gin
// context, never from the URL or body.
//
//   /api/v1/dockb/summaries/:knowledge_id       — list summaries
//   /api/v1/dockb/chunks/:knowledge_id/:chunk_id — get or upsert summary
//   /api/v1/dockb/summaries/:id                — delete one summary
//
//   /api/v1/databases                          — list / create
//   /api/v1/databases/:id                      — get / update / delete
//   /api/v1/databases/:id/rows                 — list / insert rows
//   /api/v1/databases/:id/rows/:row_id         — get / update / delete row
type DockbHandler struct {
	summariser *dockb.SummariserService
	databases  *database.Service
}

// NewDockbHandler wires the handler.
func NewDockbHandler(s *dockb.SummariserService, d *database.Service) *DockbHandler {
	return &DockbHandler{summariser: s, databases: d}
}

// Mount attaches the routes onto an authenticated v1 group.
func (h *DockbHandler) Mount(rg *gin.RouterGroup) {
	dg := rg.Group("/dockb")
	dg.GET("/summaries/:knowledge_id", h.ListSummaries)
	dg.GET("/chunks/:knowledge_id/:chunk_id", h.GetSummary)
	dg.PUT("/chunks/:knowledge_id/:chunk_id", h.UpsertSummary)
	dg.DELETE("/summaries/:id", h.DeleteSummary)

	dbg := rg.Group("/databases")
	dbg.GET("", h.ListDatabases)
	dbg.POST("", h.CreateDatabase)
	dbg.GET("/:id", h.GetDatabase)
	dbg.PATCH("/:id", h.UpdateDatabase)
	dbg.DELETE("/:id", h.DeleteDatabase)

	rg.GET("/databases/:id/rows", h.ListRows)
	rg.POST("/databases/:id/rows", h.InsertRow)
	rg.GET("/databases/:id/rows/:row_id", h.GetRow)
	rg.PATCH("/databases/:id/rows/:row_id", h.UpdateRow)
	rg.DELETE("/databases/:id/rows/:row_id", h.DeleteRow)
}

// --- Doc KB Summaries ---

func (h *DockbHandler) ListSummaries(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	rows, err := h.summariser.ListByKnowledge(c.Request.Context(), tenantID, knowledgeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summaries": rows, "total": len(rows)})
}

func (h *DockbHandler) GetSummary(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	chunkID := c.Param("chunk_id")
	row, err := h.summariser.GetByChunk(c.Request.Context(), tenantID, knowledgeID, chunkID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "summary not found"})
		return
	}
	c.JSON(http.StatusOK, row)
}

type upsertSummaryBody struct {
	Text      string `json:"text"`
	ModelName string `json:"model_name"`
}

func (h *DockbHandler) UpsertSummary(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	knowledgeID := c.Param("knowledge_id")
	chunkID := c.Param("chunk_id")
	var body upsertSummaryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := h.summariser.SummariseChunk(
		c.Request.Context(), tenantID, knowledgeID, chunkID, body.Text, body.ModelName,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = userID // reserved for future audit wiring
	c.JSON(http.StatusOK, row)
}

func (h *DockbHandler) DeleteSummary(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.summariser.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// --- Databases ---

func (h *DockbHandler) CreateDatabase(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	var body struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Schema      []types.DatabaseField  `json:"schema"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := &types.WKDatabase{
		TenantID:    tenantID,
		Name:        body.Name,
		Description: body.Description,
		Schema:      body.Schema,
		CreatedBy:   userID,
	}
	if err := h.databases.Create(c.Request.Context(), db); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, db)
}

func (h *DockbHandler) ListDatabases(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.databases.List(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"databases": rows, "total": total})
}

func (h *DockbHandler) GetDatabase(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	db, err := h.databases.Get(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if db == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
		return
	}
	c.JSON(http.StatusOK, db)
}

func (h *DockbHandler) UpdateDatabase(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Schema      []types.DatabaseField `json:"schema"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := &types.WKDatabase{
		ID:          id,
		TenantID:    tenantID,
		Name:        body.Name,
		Description: body.Description,
		Schema:      body.Schema,
	}
	if err := h.databases.Update(c.Request.Context(), db); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, db)
}

func (h *DockbHandler) DeleteDatabase(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.databases.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *DockbHandler) InsertRow(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	databaseID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Values map[string]any `json:"values"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row := &types.WKDatabaseRow{
		TenantID:   tenantID,
		DatabaseID: databaseID,
		Values:     body.Values,
		CreatedBy:  userID,
	}
	if err := h.databases.InsertRow(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *DockbHandler) ListRows(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	databaseID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := h.databases.ListRows(c.Request.Context(), tenantID, databaseID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows, "total": total})
}

func (h *DockbHandler) GetRow(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	rowID, err := strconv.ParseUint(c.Param("row_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid row_id"})
		return
	}
	row, err := h.databases.GetRow(c.Request.Context(), tenantID, rowID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "row not found"})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *DockbHandler) UpdateRow(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	rowID, err := strconv.ParseUint(c.Param("row_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid row_id"})
		return
	}
	databaseID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Values map[string]any `json:"values"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row := &types.WKDatabaseRow{
		ID:         rowID,
		TenantID:   tenantID,
		DatabaseID: databaseID,
		Values:     body.Values,
	}
	if err := h.databases.UpdateRow(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (h *DockbHandler) DeleteRow(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	rowID, err := strconv.ParseUint(c.Param("row_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid row_id"})
		return
	}
	if err := h.databases.DeleteRow(c.Request.Context(), tenantID, rowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// helper — extract tenant + user from gin context.
func (h *DockbHandler) resolveCtx(c *gin.Context) (tenantID, userID string, ok bool) {
	tv, _ := c.Get("tenant_id")
	uv, _ := c.Get("user_id")
	tid, ok1 := toString(tv)
	uid, _ := toString(uv)
	if !ok1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return "", "", false
	}
	return tid, uid, true
}

func toString(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, x != ""
	case uint64:
		return strconv.FormatUint(x, 10), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case int:
		return strconv.Itoa(x), true
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1], true
	}
	return s, true
}
