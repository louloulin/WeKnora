package handler

import (
	"encoding/base64"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service/marketplace"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// MarketplaceHandler serves the Build #34.x Marketplace + plugin
// signing API. Endpoints (all under /v1):
//
//	POST   /marketplace/vendors              register vendor
//	GET    /marketplace/vendors              list vendors
//	GET    /marketplace/vendors/:slug        get vendor
//
//	POST   /marketplace/plugins              publish signed plugin (admin)
//	GET    /marketplace/plugins              list catalog (published only)
//	GET    /marketplace/plugins/:id          list versions of a plugin
//	POST   /marketplace/plugins/:id/review   review plugin (admin)
//
//	POST   /marketplace/install              install plugin into tenant
//	GET    /marketplace/installed            list tenant installs
//	DELETE /marketplace/installed/:id        uninstall plugin
//
//	GET    /marketplace/audit                list tenant plugin audit log
type MarketplaceHandler struct {
	svc *marketplace.Service
}

// NewMarketplaceHandler constructs a MarketplaceHandler.
func NewMarketplaceHandler(svc *marketplace.Service) *MarketplaceHandler {
	return &MarketplaceHandler{svc: svc}
}

// Mount attaches all /marketplace routes.
func (h *MarketplaceHandler) Mount(rg *gin.RouterGroup) {
	rg.POST("/marketplace/vendors", h.RegisterVendor)
	rg.GET("/marketplace/vendors", h.ListVendors)
	rg.GET("/marketplace/vendors/:slug", h.GetVendor)

	rg.POST("/marketplace/plugins", h.Publish)
	rg.GET("/marketplace/plugins", h.ListCatalog)
	rg.GET("/marketplace/plugins/:id", h.ListVersions)
	rg.POST("/marketplace/plugins/:id/review", h.ReviewPlugin)

	rg.POST("/marketplace/install", h.Install)
	rg.GET("/marketplace/installed", h.ListInstalled)
	rg.DELETE("/marketplace/installed/:id", h.Uninstall)

	rg.GET("/marketplace/audit", h.ListAudit)
}

// --- Vendors ---

func (h *MarketplaceHandler) RegisterVendor(c *gin.Context) {
	var in types.PluginVendor
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.RegisterVendor(c, &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, in)
}

func (h *MarketplaceHandler) ListVendors(c *gin.Context) {
	out, err := h.svc.ListVendors(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *MarketplaceHandler) GetVendor(c *gin.Context) {
	v, err := h.svc.GetVendor(c, c.Param("slug"))
	if err == marketplace.ErrVendorNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, v)
}

// --- Publish ---

type publishInput struct {
	Manifest    *types.PluginManifest `json:"manifest"`
	ArtifactB64 string                `json:"artifact_b64,omitempty"` // base64-encoded tarball
}

func (h *MarketplaceHandler) Publish(c *gin.Context) {
	var in publishInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Manifest == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing manifest"})
		return
	}
	var artifact []byte
	if in.ArtifactB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(in.ArtifactB64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 artifact"})
			return
		}
		artifact = decoded
	}
	rec, err := h.svc.Publish(c, in.Manifest, artifact)
	if err != nil {
		status := http.StatusBadRequest
		if err == marketplace.ErrVendorNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rec)
}

func (h *MarketplaceHandler) ListCatalog(c *gin.Context) {
	out, err := h.svc.ListCatalog(c, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *MarketplaceHandler) ListVersions(c *gin.Context) {
	out, err := h.svc.ListVersions(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

type reviewInput struct {
	Version      string                   `json:"version"`
	Status       types.PluginReviewStatus `json:"status"`
	ReviewerNote string                   `json:"reviewer_note"`
}

func (h *MarketplaceHandler) ReviewPlugin(c *gin.Context) {
	pluginID := c.Param("id")
	var in reviewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actor := c.GetHeader("X-Reviewer-ID")
	if actor == "" {
		if v, ok := c.Get("user_id"); ok {
			actor, _ = v.(string)
		}
	}
	if err := h.svc.ReviewPlugin(c, pluginID, in.Version, in.Status, in.ReviewerNote, actor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- Install ---

type installInput struct {
	PluginID           string   `json:"plugin_id"`
	Version            string   `json:"version"`
	GrantedPermissions []string `json:"granted_permissions"`
}

func (h *MarketplaceHandler) Install(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	if tenantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing tenant"})
		return
	}
	var in installInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	actor := ""
	if v, ok := c.Get("user_id"); ok {
		actor, _ = v.(string)
	}
	tp, err := h.svc.Install(c, tenantID, in.PluginID, in.Version, actor, in.GrantedPermissions)
	if err != nil {
		status := http.StatusBadRequest
		switch err {
		case marketplace.ErrPluginNotFound, marketplace.ErrNotInstalled:
			status = http.StatusNotFound
		case marketplace.ErrPluginNotPublic:
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tp)
}

func (h *MarketplaceHandler) ListInstalled(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	out, err := h.svc.ListInstalled(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *MarketplaceHandler) Uninstall(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	actor := ""
	if v, ok := c.Get("user_id"); ok {
		actor, _ = v.(string)
	}
	if err := h.svc.Uninstall(c, tenantID, c.Param("id"), actor); err != nil {
		if err == marketplace.ErrNotInstalled {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- Audit ---

func (h *MarketplaceHandler) ListAudit(c *gin.Context) {
	tenantID := uint64FromCtx(c)
	out, err := h.svc.ListAudit(c, tenantID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}
