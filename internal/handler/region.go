package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service/region"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// RegionHandler serves the multi-region + data-residency API. Endpoints:
//	GET    /regions                           list all regions
//	GET    /regions/:id                       get region by id
//	POST   /regions                           upsert region (admin only)
//	GET    /tenants/:id/region                get tenant region binding
//	POST   /tenants/:id/region                bind tenant to region + policy
//	DELETE /tenants/:id/region                remove binding
//	GET    /audit/cross-region                list denied cross-region actions
//	GET    /audit/cross-region/:tenant_id     list audit entries for a tenant
type RegionHandler struct {
	svc *region.Service
}

// NewRegionHandler constructs a RegionHandler.
func NewRegionHandler(svc *region.Service) *RegionHandler {
	return &RegionHandler{svc: svc}
}

// Mount attaches all /regions and /tenants/:id/region routes.
func (h *RegionHandler) Mount(rg *gin.RouterGroup) {
	rg.GET("/regions", h.List)
	rg.GET("/regions/:id", h.Get)
	rg.POST("/regions", h.Upsert)
	rg.GET("/tenants/:id/region", h.GetBinding)
	rg.POST("/tenants/:id/region", h.Bind)
	rg.DELETE("/tenants/:id/region", h.Unbind)
	rg.GET("/audit/cross-region", h.ListDenied)
	rg.GET("/audit/cross-region/:tenant_id", h.ListByTenant)
}

// List returns every region record.
func (h *RegionHandler) List(c *gin.Context) {
	out, err := h.svc.ListRegions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// Get returns one region record by id.
func (h *RegionHandler) Get(c *gin.Context) {
	rec, err := h.svc.GetRegion(c, c.Param("id"))
	if err == region.ErrNotFound || err == region.ErrInvalidRegion {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rec)
}

// Upsert creates or updates a region (admin).
func (h *RegionHandler) Upsert(c *gin.Context) {
	var in types.RegionRecord
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UpsertRegion(c, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

// GetBinding returns the current region binding for a tenant.
func (h *RegionHandler) GetBinding(c *gin.Context) {
	tenantID, ok := bindUint64Param(c, "id")
	if !ok {
		return
	}
	b, err := h.svc.GetTenantBinding(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if b == nil {
		c.JSON(http.StatusOK, gin.H{"binding": nil})
		return
	}
	c.JSON(http.StatusOK, b)
}

// Bind installs (or replaces) the tenant's region binding.
func (h *RegionHandler) Bind(c *gin.Context) {
	tenantID, ok := bindUint64Param(c, "id")
	if !ok {
		return
	}
	var in types.TenantRegionBinding
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.TenantID = tenantID
	if err := h.svc.BindTenant(c, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

// Unbind removes the binding.
func (h *RegionHandler) Unbind(c *gin.Context) {
	tenantID, ok := bindUint64Param(c, "id")
	if !ok {
		return
	}
	if err := h.svc.UnbindTenant(c, tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListDenied returns the most recent denied cross-region actions (admin).
func (h *RegionHandler) ListDenied(c *gin.Context) {
	out, err := h.svc.ListDeniedAudit(c, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListByTenant returns the most recent cross-region audit entries for a tenant.
func (h *RegionHandler) ListByTenant(c *gin.Context) {
	tenantID, ok := bindUint64Param(c, "tenant_id")
	if !ok {
		return
	}
	out, err := h.svc.ListAuditByTenant(c, tenantID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// bindUint64Param extracts a uint64 path parameter, writing a 400 if
// the value is not a valid integer.
func bindUint64Param(c *gin.Context, name string) (uint64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return id, true
}
