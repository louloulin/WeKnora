package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AuditExportHandler exposes the v0.7.25 Build #24 audit export +
// compliance report surface. The constructor takes the
// AuditExportService interface so tests can wire a fake.
//
// All endpoints require Owner / Admin role — the rbacGuards in the
// route file enforce that. The handler does not duplicate the check.
type AuditExportHandler struct {
	svc interfaces.AuditExportService
}

// NewAuditExportHandler wires the handler. A nil service is rejected
// so a missing DI wiring fails loudly at startup.
func NewAuditExportHandler(svc interfaces.AuditExportService) *AuditExportHandler {
	if svc == nil {
		panic("handler.NewAuditExportHandler: svc is required")
	}
	return &AuditExportHandler{svc: svc}
}

// validateContext extracts the tenant + user id from the gin context.
func (h *AuditExportHandler) validateContext(c *gin.Context) (uint64, string, bool) {
	tenantID := c.GetUint64("tenant_id")
	userID := c.GetString("user_id")
	if tenantID == 0 || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return 0, "", false
	}
	return tenantID, userID, true
}

// CreateAndDownload — POST /api/v1/audit/exports
//
// Synchronous export — runs the audit scan + payload encode in-line
// and streams the result as an attachment. The metadata row is
// persisted so the operator can re-fetch the export from the list
// endpoint later.
//
// Body: { format?: "csv" | "json", filter?: AuditExportFilter, max_rows?: int }
// Response: 200 application/octet-stream (attachment) + JSON metadata
// header via X-Audit-Export-Id so the client can track the run.
func (h *AuditExportHandler) CreateAndDownload(c *gin.Context) {
	tenantID, userID, ok := h.validateContext(c)
	if !ok {
		return
	}

	var req types.AuditExportCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	export, payload, err := h.svc.CreateAndRun(c.Request.Context(), tenantID, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, types.ErrAuditExportBadInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c.Request.Context(), "audit export: create+run failed: tenant=%d err=%v", tenantID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Record the export action itself in the audit log so admins can
	// see who pulled which slice of the trail.
	logger.Infof(c.Request.Context(),
		"audit export: tenant=%d user=%s id=%s format=%s rows=%d bytes=%d",
		tenantID, userID, export.ID, export.Format, export.RowCount, export.ByteSize)

	contentType := "text/csv; charset=utf-8"
	if export.Format == types.AuditExportFormatJSON {
		contentType = "application/json; charset=utf-8"
	}
	filename := fmt.Sprintf("audit-export-%s-%s.%s", export.TenantID, export.ID, export.Format)

	c.Header("X-Audit-Export-Id", export.ID)
	c.Header("X-Audit-Export-Row-Count", strconv.FormatInt(export.RowCount, 10))
	c.Header("X-Audit-Export-Byte-Size", strconv.FormatInt(export.ByteSize, 10))
	c.Data(http.StatusOK, contentType, payload)
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
}

// Get — GET /api/v1/audit/exports/:id
//
// Returns the export metadata. The actual payload is not re-issued
// from this endpoint — clients that want to re-download should call
// the dedicated download endpoint below. This keeps the list/get
// surface compact.
func (h *AuditExportHandler) Get(c *gin.Context) {
	tenantID, _, ok := h.validateContext(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	export, err := h.svc.Get(c.Request.Context(), tenantID, id)
	if err != nil {
		switch {
		case errors.Is(err, interfaces.ErrAuditExportNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, export)
}

// List — GET /api/v1/audit/exports
//
// Returns the most recent exports for the tenant (default 50).
func (h *AuditExportHandler) List(c *gin.Context) {
	tenantID, _, ok := h.validateContext(c)
	if !ok {
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	rows, err := h.svc.List(c.Request.Context(), tenantID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []types.AuditExport{}
	}
	c.JSON(http.StatusOK, gin.H{"exports": rows})
}

// ComplianceSummary — GET /api/v1/audit/report?window_days=30
//
// Returns the compliance summary for the trailing window. The default
// window is 30 days; the upper bound is 365 days to keep the scan
// bounded.
func (h *AuditExportHandler) ComplianceSummary(c *gin.Context) {
	tenantID, _, ok := h.validateContext(c)
	if !ok {
		return
	}
	windowDays := types.ComplianceReportWindowDays
	if v := c.Query("window_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			windowDays = n
		}
	}
	summary, err := h.svc.ComplianceSummary(c.Request.Context(), tenantID, windowDays)
	if err != nil {
		logger.Errorf(c.Request.Context(), "compliance summary failed: tenant=%d err=%v", tenantID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}
