package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/database"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// DatabaseHandler exposes the Build #26 (G06) multi-view database
// REST surface. Routes:
//
//	GET    /knowledge-bases/:kb_id/databases                — list by KB
//	POST   /knowledge-bases/:kb_id/databases                — create
//	GET    /databases/:id                                  — get + fields + views
//	PATCH  /databases/:id                                  — update metadata
//	DELETE /databases/:id                                  — soft-delete
//
//	POST   /databases/:id/fields                           — add field
//	PATCH  /databases/:id/fields/:field_id                 — update field
//	DELETE /databases/:id/fields/:field_id                 — delete field
//
//	GET    /databases/:id/rows                             — list rows
//	POST   /databases/:id/rows                             — add row
//	PATCH  /databases/:id/rows/:row_id                     — update row
//	POST   /databases/:id/rows/reorder                     — bulk reorder
//	DELETE /databases/:id/rows/:row_id                     — soft-delete row
//
//	GET    /databases/:id/views                            — list views
//	POST   /databases/:id/views                            — add view
//	PATCH  /databases/:id/views/:view_id                   — update view
//	DELETE /databases/:id/views/:view_id                   — delete view
//
// Tenant and user are read from gin context, never from URL/body.
type DatabaseHandler struct {
	svc          *database.Service
	kbService    interfaces.KnowledgeBaseService
}

// NewDatabaseHandler wires the handler.
func NewDatabaseHandler(svc *database.Service, kbService interfaces.KnowledgeBaseService) *DatabaseHandler {
	return &DatabaseHandler{svc: svc, kbService: kbService}
}

// Mount attaches the routes to an authenticated v1 group.
func (h *DatabaseHandler) Mount(rg *gin.RouterGroup) {
	g := rg.Group("/knowledge-bases/:kb_id/databases")
	g.GET("", h.List)
	g.POST("", h.Create)

	d := rg.Group("/databases/:id")
	d.GET("", h.Get)
	d.PATCH("", h.Update)
	d.DELETE("", h.Delete)

	// Fields
	d.POST("/fields", h.AddField)
	d.PATCH("/fields/:field_id", h.UpdateField)
	d.DELETE("/fields/:field_id", h.DeleteField)

	// Rows
	d.GET("/rows", h.ListRows)
	d.POST("/rows", h.AddRow)
	d.PATCH("/rows/:row_id", h.UpdateRow)
	d.POST("/rows/reorder", h.ReorderRows)
	d.DELETE("/rows/:row_id", h.DeleteRow)

	// Views
	d.GET("/views", h.ListViews)
	d.POST("/views", h.AddView)
	d.PATCH("/views/:view_id", h.UpdateView)
	d.DELETE("/views/:view_id", h.DeleteView)
}

// --- helpers ---

func (h *DatabaseHandler) resolveCtx(c *gin.Context) (uint64, string, bool) {
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID := c.GetString(types.UserIDContextKey.String())
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return 0, "", false
	}
	return tenantID, userID, true
}

func (h *DatabaseHandler) resolveKB(c *gin.Context) (uint64, string, string, bool) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return 0, "", "", false
	}
	kbID := c.Param("kb_id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id required"})
		return 0, "", "", false
	}
	return tenantID, userID, kbID, true
}

// --- databases ---

type createDatabaseBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

func (h *DatabaseHandler) Create(c *gin.Context) {
	tenantID, userID, kbID, ok := h.resolveKB(c)
	if !ok {
		return
	}
	var body createDatabaseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db := &types.Database{
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		Name:            body.Name,
		Description:     body.Description,
		Icon:            body.Icon,
		CreatedBy:       userID,
	}
	if err := h.svc.Create(c.Request.Context(), db); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, db)
}

func (h *DatabaseHandler) List(c *gin.Context) {
	tenantID, _, kbID, ok := h.resolveKB(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, total, err := h.svc.ListByKB(c.Request.Context(), tenantID, kbID, limit, offset)
	if err != nil {
		logger.Errorf(c.Request.Context(), "database: list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total, "limit": limit, "offset": offset})
}

func (h *DatabaseHandler) Get(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	detail, err := h.svc.GetDetail(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

type updateDatabaseBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
}

func (h *DatabaseHandler) Update(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	existing, err := h.svc.GetDetail(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body updateDatabaseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name != nil {
		existing.Database.Name = *body.Name
	}
	if body.Description != nil {
		existing.Database.Description = *body.Description
	}
	if body.Icon != nil {
		existing.Database.Icon = *body.Icon
	}
	if err := h.svc.Update(c.Request.Context(), existing.Database); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing.Database)
}

func (h *DatabaseHandler) Delete(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := h.svc.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// --- fields ---

type fieldBody struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Options   json.RawMessage        `json:"options"`
	Width     int                    `json:"width"`
	SortOrder int                    `json:"sort_order"`
	IsPrimary bool                   `json:"is_primary"`
	Extra     map[string]interface{} `json:"-"`
}

func (h *DatabaseHandler) AddField(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body fieldBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Options == nil {
		body.Options = json.RawMessage(`{}`)
	}
	f := &types.DatabaseField{
		DatabaseID: id,
		Name:       body.Name,
		Type:       types.DatabaseFieldType(body.Type),
		Options:    types.JSON(body.Options),
		Width:      body.Width,
		SortOrder:  body.SortOrder,
		IsPrimary:  body.IsPrimary,
	}
	if err := h.svc.AddField(c.Request.Context(), f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, f)
}

func (h *DatabaseHandler) UpdateField(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	fieldID := c.Param("field_id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body fieldBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Options == nil {
		body.Options = json.RawMessage(`{}`)
	}
	f := &types.DatabaseField{
		ID:         fieldID,
		DatabaseID: id,
		Name:       body.Name,
		Type:       types.DatabaseFieldType(body.Type),
		Options:    types.JSON(body.Options),
		Width:      body.Width,
		SortOrder:  body.SortOrder,
		IsPrimary:  body.IsPrimary,
	}
	if err := h.svc.UpdateField(c.Request.Context(), f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, f)
}

func (h *DatabaseHandler) DeleteField(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	fieldID := c.Param("field_id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.DeleteField(c.Request.Context(), id, fieldID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// --- rows ---

type rowBody struct {
	Data json.RawMessage `json:"data"`
}

func (h *DatabaseHandler) AddRow(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body rowBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Data == nil {
		body.Data = json.RawMessage(`{}`)
	}
	row := &types.DatabaseRow{
		DatabaseID: id,
		Data:       body.Data,
		CreatedBy:  userID,
	}
	if err := h.svc.AddRow(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *DatabaseHandler) UpdateRow(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	rowID := c.Param("row_id")
	if _, err := h.svc.GetRow(c.Request.Context(), tenantID, rowID); err != nil {
		if errors.Is(err, database.ErrRowNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "row not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body rowBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Data == nil {
		body.Data = json.RawMessage(`{}`)
	}
	row := &types.DatabaseRow{
		ID:         rowID,
		DatabaseID: id,
		Data:       body.Data,
	}
	if err := h.svc.UpdateRow(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

type reorderBody struct {
	IDs []string `json:"ids"`
}

func (h *DatabaseHandler) ReorderRows(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body reorderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ReorderRows(c.Request.Context(), body.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reordered": len(body.IDs)})
}

func (h *DatabaseHandler) DeleteRow(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	rowID := c.Param("row_id")
	if err := h.svc.DeleteRow(c.Request.Context(), tenantID, rowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *DatabaseHandler) ListRows(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, total, err := h.svc.ListRows(c.Request.Context(), id, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows, "total": total, "limit": limit, "offset": offset})
}

// --- views ---

type viewBody struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	SortOrder int             `json:"sort_order"`
	IsDefault bool            `json:"is_default"`
}

func (h *DatabaseHandler) AddView(c *gin.Context) {
	tenantID, userID, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body viewBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Config == nil {
		body.Config = json.RawMessage(`{}`)
	}
	v := &types.DatabaseView{
		DatabaseID: id,
		Type:       types.DatabaseViewType(body.Type),
		Name:       body.Name,
		Config:     types.JSON(body.Config),
		SortOrder:  body.SortOrder,
		IsDefault:  body.IsDefault,
		CreatedBy:  userID,
	}
	if err := h.svc.AddView(c.Request.Context(), v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h *DatabaseHandler) UpdateView(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	viewID := c.Param("view_id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var body viewBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Config == nil {
		body.Config = json.RawMessage(`{}`)
	}
	v := &types.DatabaseView{
		ID:         viewID,
		DatabaseID: id,
		Type:       types.DatabaseViewType(body.Type),
		Name:       body.Name,
		Config:     types.JSON(body.Config),
		SortOrder:  body.SortOrder,
		IsDefault:  body.IsDefault,
	}
	if err := h.svc.UpdateView(c.Request.Context(), v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *DatabaseHandler) DeleteView(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	viewID := c.Param("view_id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.DeleteView(c.Request.Context(), id, viewID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *DatabaseHandler) ListViews(c *gin.Context) {
	tenantID, _, ok := h.resolveCtx(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if _, err := h.svc.GetDetail(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, database.ErrDatabaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	views, err := h.svc.ListViews(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": views})
}
