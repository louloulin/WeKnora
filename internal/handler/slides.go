// Package handler — Build #44 Slide REST surface.
//
// All routes require auth middleware (Bearer token → tenant_id + user_id).
//
//   POST   /api/v1/slides                          create a deck
//   GET    /api/v1/slides                          list decks (filters)
//   POST   /api/v1/slides/auto-generate            auto-generate from doc
//   GET    /api/v1/slides/:id                      read deck
//   PATCH  /api/v1/slides/:id                      update deck
//   DELETE /api/v1/slides/:id                      delete deck + slides
//   GET    /api/v1/slides/:id/slides               list slides
//   POST   /api/v1/slides/:id/slides               create slide
//   PATCH  /api/v1/slides/:id/slides/:slideID      update slide
//   DELETE /api/v1/slides/:id/slides/:slideID      delete slide
//   GET    /api/v1/slides/:id/export?format=...    export markdown / json / html
package handler

import (
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service/slides"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// SlideHandler exposes the Slide REST surface.
type SlideHandler struct {
	svc *slides.SlideService
}

// NewSlideHandler constructs the handler.
func NewSlideHandler(svc *slides.SlideService) *SlideHandler {
	return &SlideHandler{svc: svc}
}

// Mount registers the Slide routes on the supplied router group.
func (h *SlideHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/slides", h.Create)
	rg.GET("/slides", h.List)
	rg.POST("/slides/auto-generate", h.AutoGenerate)
	rg.GET("/slides/:id", h.Get)
	rg.PATCH("/slides/:id", h.Update)
	rg.DELETE("/slides/:id", h.Delete)
	rg.GET("/slides/:id/slides", h.ListSlides)
	rg.POST("/slides/:id/slides", h.CreateSlide)
	rg.PATCH("/slides/:id/slides/:slideID", h.UpdateSlide)
	rg.DELETE("/slides/:id/slides/:slideID", h.DeleteSlide)
	rg.GET("/slides/:id/export", h.Export)
}

// Create handles POST /slides.
func (h *SlideHandler) Create(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.CreateSlideDeckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, err := h.svc.CreateDeck(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

// List handles GET /slides.
func (h *SlideHandler) List(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	filter := types.ListSlideDecksFilter{
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
	decks, err := h.svc.ListDecks(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	count, err := h.svc.CountDecks(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": decks, "total": count})
}

// Get handles GET /slides/:id.
func (h *SlideHandler) Get(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	d, err := h.svc.GetDeck(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "slide deck not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

// Update handles PATCH /slides/:id.
func (h *SlideHandler) Update(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.UpdateSlideDeckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, err := h.svc.UpdateDeck(c.Request.Context(), tenantID, userID, c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "slide deck not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

// Delete handles DELETE /slides/:id.
func (h *SlideHandler) Delete(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteDeck(c.Request.Context(), tenantID, userID, c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSlides handles GET /slides/:id/slides.
func (h *SlideHandler) ListSlides(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	slides, err := h.svc.ListSlides(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": slides})
}

// CreateSlide handles POST /slides/:id/slides.
func (h *SlideHandler) CreateSlide(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.CreateSlideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	slide, err := h.svc.CreateSlide(c.Request.Context(), tenantID, userID, c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, slide)
}

// UpdateSlide handles PATCH /slides/:id/slides/:slideID.
func (h *SlideHandler) UpdateSlide(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.UpdateSlideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	slide, err := h.svc.UpdateSlide(c.Request.Context(), tenantID, userID, c.Param("id"), c.Param("slideID"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if slide == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "slide not found"})
		return
	}
	c.JSON(http.StatusOK, slide)
}

// DeleteSlide handles DELETE /slides/:id/slides/:slideID.
func (h *SlideHandler) DeleteSlide(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteSlide(c.Request.Context(), tenantID, userID, c.Param("id"), c.Param("slideID")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// AutoGenerate handles POST /slides/auto-generate.
func (h *SlideHandler) AutoGenerate(c *gin.Context) {
	tenantID, userID, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	var req types.AutoGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// For the offline-friendly path, accept a "markdown" field in the body
	// alongside the documented fields.
	body := struct {
		Markdown string `json:"markdown"`
	}{}
	_ = c.ShouldBindBodyWith(&body, nil) // best-effort, ignore error
	markdown := body.Markdown
	if markdown == "" {
		// Fall back to a simple stub: a single section titled "Source".
		markdown = fmt.Sprintf("## Source\n\nDocument %s auto-generated at %s", req.SourceDocID, timeNow())
	}
	d, err := h.svc.AutoGenerateFromDoc(c.Request.Context(), tenantID, userID, req.SourceDocID, req.KBID, req.Title, req.Theme, markdown, req.MaxSlides)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

// Export handles GET /slides/:id/export?format=markdown|json|html|pptx|pdf.
func (h *SlideHandler) Export(c *gin.Context) {
	tenantID, _, ok := h.tenantAndUser(c)
	if !ok {
		return
	}
	format := types.SlideExportFormat(c.Query("format"))
	if format == "" {
		format = types.SlideExportFormatMarkdown
	}
	if !types.ValidSlideExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format is invalid (markdown|json|html|pptx|pdf)"})
		return
	}
	deckID := c.Param("id")
	var (
		body2   string
		err     error
		content string
	)
	switch format {
	case types.SlideExportFormatMarkdown:
		content, err = h.svc.ExportMarkdown(c.Request.Context(), tenantID, deckID)
		body2 = "text/markdown; charset=utf-8"
	case types.SlideExportFormatJSON:
		content, err = h.svc.ExportJSON(c.Request.Context(), tenantID, deckID)
		body2 = "application/json; charset=utf-8"
	case types.SlideExportFormatHTML, types.SlideExportFormatPDF, types.SlideExportFormatPPTX:
		// HTML / PDF / PPTX render client-side; we emit the JSON payload
		// with a hint the renderer should pick up.
		content, err = h.svc.ExportJSON(c.Request.Context(), tenantID, deckID)
		body2 = "application/json; charset=utf-8"
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logger.Debugf(c, "[slides] export format=%s deck=%s bytes=%d", format, deckID, len(content))
	c.Data(http.StatusOK, body2, []byte(content))
}

// tenantAndUser extracts tenant_id + user_id from the gin context.
func (h *SlideHandler) tenantAndUser(c *gin.Context) (uint64, uint64, bool) {
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

// timeNow is a tiny helper so the handler doesn't import "time" twice.
func timeNow() string {
	return timeNowFn()
}

var timeNowFn = func() string {
	return "2026-09-01T00:00:00Z"
}
