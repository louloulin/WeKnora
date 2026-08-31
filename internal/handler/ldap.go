package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/ldapsp"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LDAPHandler exposes the directory integration endpoints. The
// surface mirrors SAMLHandler: per-tenant CRUD + a "test" probe +
// an admin login endpoint that exchanges username/password for the
// local token pair.
type LDAPHandler struct {
	configSvc *service.LDAPConfigService
	userSvc   interfaces.UserService
}

// NewLDAPHandler constructs the handler.
func NewLDAPHandler(configSvc *service.LDAPConfigService, userSvc interfaces.UserService) *LDAPHandler {
	return &LDAPHandler{configSvc: configSvc, userSvc: userSvc}
}

// CreateConfig — POST /api/v1/ldap/configs
func (h *LDAPHandler) CreateConfig(c *gin.Context) {
	tenantID, ok := requireTenantID(c)
	if !ok {
		return
	}
	var req types.LDAPConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Override tenant_id from the JWT so the client cannot pick a
	// different tenant. The model field stays out of the request
	// body to keep the contract symmetric across SSO protocols.
	out, err := h.configSvc.Create(c.Request.Context(), &req)
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	// Pin the tenant id; the service create ignored it.
	out.TenantID = tenantID
	c.JSON(http.StatusCreated, out)
}

// GetConfigForTenant — GET /api/v1/ldap/configs
func (h *LDAPHandler) GetConfigForTenant(c *gin.Context) {
	tenantID, ok := requireTenantID(c)
	if !ok {
		return
	}
	out, err := h.configSvc.GetByTenant(c.Request.Context(), tenantID)
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// GetConfig — GET /api/v1/ldap/configs/:id
func (h *LDAPHandler) GetConfig(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	out, err := h.configSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListConfigs — GET /api/v1/ldap/admin/configs (admin only)
func (h *LDAPHandler) ListConfigs(c *gin.Context) {
	rows, err := h.configSvc.List(c.Request.Context())
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}

// UpdateConfig — PUT /api/v1/ldap/configs/:id
func (h *LDAPHandler) UpdateConfig(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req types.LDAPConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.configSvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// DeleteConfig — DELETE /api/v1/ldap/configs/:id
func (h *LDAPHandler) DeleteConfig(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configSvc.Delete(c.Request.Context(), id); err != nil {
		respondLDAPError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// TestConnection — POST /api/v1/ldap/configs/:id/test
func (h *LDAPHandler) TestConnection(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configSvc.TestConnection(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LoginRequest is the body for POST /api/v1/auth/ldap/login. Mirrors
// OIDC's request shape so the frontend code is symmetric.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login — POST /api/v1/auth/ldap/login
//
// The handler delegates to userService.LoginWithLDAPCredentials
// (same path as OIDC and SAML). The default tenant mode is the
// shared auth.default_tenant_mode system setting, resolved by the
// caller from the request context — empty for now lets the service
// fall back to its own default.
func (h *LDAPHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenantID, ok := requireTenantID(c)
	if !ok {
		return
	}
	provisioning := types.TenantProvisioningMode(c.GetHeader("X-WeKnora-Tenant-Mode"))
	resp, err := h.userSvc.LoginWithLDAPCredentials(c.Request.Context(), tenantID, req.Username, req.Password, provisioning)
	if err != nil {
		respondLDAPError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// requireTenantID pulls the tenant_id off the auth context. We do
// not import the auth helper here — the middleware chain guarantees
// the claim is present when the route is auth-protected.
func requireTenantID(c *gin.Context) (uint64, bool) {
	v, ok := c.Get("tenant_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id missing in auth context"})
		return 0, false
	}
	switch x := v.(type) {
	case uint64:
		return x, true
	case int:
		if x <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant_id"})
			return 0, false
		}
		return uint64(x), true
	default:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant_id type"})
		return 0, false
	}
}

func parseIDParam(c *gin.Context, name string) (uint64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return id, true
}

// respondLDAPError maps the service-layer sentinels into distinct
// HTTP statuses. Unknown errors degrade to 500 with a generic
// message — never echo the raw driver error to the client.
func respondLDAPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrLDAPConfigNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "ldap config not found"})
	case errors.Is(err, service.ErrLDAPFederationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "ldap federation not found"})
	case errors.Is(err, service.ErrLDAPFederationRevoked):
		c.JSON(http.StatusForbidden, gin.H{"error": "ldap federation revoked"})
	case errors.Is(err, service.ErrLDAPInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	case errors.Is(err, service.ErrLDAPEntryNotFound):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	case errors.Is(err, service.ErrLDAPMissingEmail):
		c.JSON(http.StatusBadRequest, gin.H{"error": "directory entry missing email"})
	case errors.Is(err, service.ErrLDAPIdentityLinkingDisabled):
		c.JSON(http.StatusForbidden, gin.H{"error": "identity linking disabled"})
	case errors.Is(err, ldapsp.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
	default:
		logger.Errorf(c.Request.Context(), "ldap handler error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
