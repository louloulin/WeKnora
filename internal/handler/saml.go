package handler

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/samlsp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SAMLHandler is the HTTP surface for the SAML 2.0 SP. The same
// handler exposes:
//
//   - GET  /auth/saml/metadata           — SP metadata (public)
//   - POST /auth/saml/login              — SP-initiated SSO start
//   - POST /auth/saml/acs                — Assertion Consumer Service
//   - GET  /auth/saml/idp                — Read current tenant IdP
//   - POST /auth/saml/idp                — Create IdP (admin)
//   - PUT  /auth/saml/idp                — Update IdP (admin)
//   - DELETE /auth/saml/idp              — Delete IdP (admin)
//
// The ACS endpoint returns the validated assertion details as JSON
// in this release. The follow-up commit wires the assertion into
// the user-provisioning + JWT-mint pipeline (parallel work with
// the LDAP / SCIM integration).
type SAMLHandler struct {
	idpSvc    interfaces.SAMLIdPService
	userSvc   interfaces.UserService
	sp        *samlsp.SPConfig
	systemSvc interfaces.SystemSettingService
}

// NewSAMLHandler constructs the handler.
func NewSAMLHandler(
	idpSvc interfaces.SAMLIdPService,
	userSvc interfaces.UserService,
	sp *samlsp.SPConfig,
	systemSvc interfaces.SystemSettingService,
) *SAMLHandler {
	return &SAMLHandler{idpSvc: idpSvc, userSvc: userSvc, sp: sp, systemSvc: systemSvc}
}

// SAMLMetadata serves the SP metadata document. No auth — admins
// paste this into their IdP.
func (h *SAMLHandler) SAMLMetadata(c *gin.Context) {
	body, err := h.sp.Metadata()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/samlmetadata+xml", body)
}

// SAMLLogin kicks off SP-initiated SSO. Reads tenant from the
// query string; redirects the browser to the IdP's SSO URL.
func (h *SAMLHandler) SAMLLogin(c *gin.Context) {
	tenantSlug := c.Query("tenant")
	if tenantSlug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant query parameter is required"})
		return
	}
	tenantID, err := h.resolveTenantID(c, tenantSlug)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	idp, err := h.idpSvc.GetEnabled(c, tenantID)
	if err != nil {
		if errors.Is(err, service.ErrSAMLIdPNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "saml idp not configured for tenant"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	res, err := h.sp.MakeAuthenticationRequest(h.toIdPConfig(idp), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, res.URL)
}

// SAMLACS is the Assertion Consumer Service. The IdP POSTs the
// SAML Response here after authenticating the user.
func (h *SAMLHandler) SAMLACS(c *gin.Context) {
	relayState := c.PostForm("RelayState")
	samlResponse := c.PostForm("SAMLResponse")
	if samlResponse == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SAMLResponse missing"})
		return
	}
	tenantID, _, err := samlsp.DecodeRelayState(relayState)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid RelayState"})
		return
	}
	idp, err := h.idpSvc.GetEnabled(c, tenantID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "saml idp not configured"})
		return
	}
	assertion, err := h.sp.ParseResponse(h.toIdPConfig(idp), samlResponse)
	if err != nil {
		logger.Errorf(c, "saml acs: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "saml response invalid"})
		return
	}
	if err := assertion.Validate(h.sp.EntityID, h.clockSkewNS(c)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	info := h.extractSAMLIdentityInfo(c, idp, assertion)
	if info.IdPEntityID == "" || info.NameID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "saml assertion missing identity attributes"})
		return
	}

	// Resolve the default tenant mode from the system setting so the
	// federation JIT path behaves the same way password registration
	// and OIDC do. Falling back to create_personal when the system
	// service is unavailable keeps the ACS path hot in degraded
	// single-tenant deployments.
	defaultMode := types.TenantProvisioningCreatePersonal
	if h.systemSvc != nil {
		v := h.systemSvc.GetString(c, "auth.default_tenant_mode", "", "create_personal")
		mode := types.TenantProvisioningMode(strings.TrimSpace(v))
		if mode.IsValid() {
			defaultMode = mode
		}
	}

	resp, err := h.userSvc.LoginWithSAMLAssertion(c, tenantID, info, defaultMode)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSAMLIdentityRevoked):
			c.JSON(http.StatusForbidden, gin.H{"error": "saml identity has been revoked"})
		case errors.Is(err, service.ErrSAMLIdentityLinkingDisabled):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrSAMLAssertionMissingEmail):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			logger.Errorf(c, "saml acs: login failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "saml login failed"})
		}
		return
	}
	if !resp.Success {
		c.JSON(http.StatusForbidden, gin.H{"error": resp.Message})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":         resp.Token,
		"refresh_token": resp.RefreshToken,
		"user":          resp.User,
		"memberships":   resp.Memberships,
		"active_tenant": resp.ActiveTenant,
		"expires_at":    assertion.NotOnOrAfter,
	})
}

// extractSAMLIdentityInfo pulls the fields the federation layer needs
// off the assertion. Attribute names come from the per-tenant IdP
// config so admins can map a corporate IdP's awkward friendly names
// (e.g. urn:oid:0.9.2342.19200300.100.1.3 → email) onto the WeKnora
// vocabulary.
func (h *SAMLHandler) extractSAMLIdentityInfo(c *gin.Context, idp *types.SAMLIdPConfig, assertion *samlsp.Assertion) types.SAMLIdentityInfo {
	info := types.SAMLIdentityInfo{
		IdPEntityID:  idp.EntityID,
		NameID:       assertion.NameID,
		NameIDFormat: idp.NameIDFormat,
		SessionIndex: assertion.SessionIndex,
	}
	if idp.AttributeMap == nil {
		return info
	}
	emailKey := samlMapLookup(idp.AttributeMap, "email")
	if emailKey != "" {
		info.Email = firstAttributeValue(assertion.Attributes, emailKey)
	}
	nameKey := samlMapLookup(idp.AttributeMap, "displayName")
	if nameKey != "" {
		info.DisplayName = firstAttributeValue(assertion.Attributes, nameKey)
	}
	return info
}

// samlMapLookup reads a string value out of the JSONMap attribute map.
// The model stores the map as map[string]any because GORM backs it
// with a JSON column; we accept both string and the JSON-decoded
// default-shape and coerce to a plain string.
func samlMapLookup(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	}
	return ""
}

// clockSkewNS returns the SAML clock skew window in nanoseconds. The
// system-level config can override the default 30s when an IdP's clock
// drifts noticeably (common with VMs that have not run ntp).
func (h *SAMLHandler) clockSkewNS(c *gin.Context) time.Duration {
	return 30 * time.Second
}

// firstAttributeValue returns the first value for the given attribute
// name, or empty when the assertion did not carry it. Multi-valued
// attributes (groups, roles) are intentionally ignored here — we only
// read the scalar fields needed to identify the user.
func firstAttributeValue(attrs map[string][]string, name string) string {
	if attrs == nil || name == "" {
		return ""
	}
	values, ok := attrs[name]
	if !ok || len(values) == 0 {
		return ""
	}
	return values[0]
}

// SAMLGetIdP returns the current tenant's IdP config (certificate
// field is masked in the response — admins re-fetch through the
// dedicated endpoint when they need the raw value).
func (h *SAMLHandler) SAMLGetIdP(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	cfg, err := h.idpSvc.Get(c, tenantID)
	if err != nil {
		if errors.Is(err, service.ErrSAMLIdPNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "saml idp not configured"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, maskCertificate(cfg))
}

// SAMLCreateIdP creates a new IdP config (admin only — guarded at
// the route layer with Admin role).
func (h *SAMLHandler) SAMLCreateIdP(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	var req types.SAMLIdPConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.idpSvc.Create(c, tenantID, req)
	if err != nil {
		if errors.Is(err, service.ErrSAMLIdPAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrSAMLInvalidCertificate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, maskCertificate(cfg))
}

// SAMLUpdateIdP mutates the current IdP config.
func (h *SAMLHandler) SAMLUpdateIdP(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	var req types.SAMLIdPConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.idpSvc.Update(c, tenantID, req)
	if err != nil {
		if errors.Is(err, service.ErrSAMLInvalidCertificate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, maskCertificate(cfg))
}

// SAMLDeleteIdP soft-deletes the IdP config.
func (h *SAMLHandler) SAMLDeleteIdP(c *gin.Context) {
	tenantID, ok := tenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}
	if err := h.idpSvc.Delete(c, tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// toIdPConfig converts the typed model to the SP-package config.
// Kept private so the SP package never imports the application
// model package (preserves the authz/samlsp boundary pattern).
func (h *SAMLHandler) toIdPConfig(cfg *types.SAMLIdPConfig) *samlsp.IdPConfig {
	der, err := base64.StdEncoding.DecodeString(cfg.Certificate)
	if err != nil {
		der = []byte{}
	}
	cert, _ := x509.ParseCertificate(der)
	attrMap := make(map[string]string, len(cfg.AttributeMap))
	for k, v := range cfg.AttributeMap {
		if s, ok := v.(string); ok {
			attrMap[k] = s
		}
	}
	return &samlsp.IdPConfig{
		TenantID:     cfg.TenantID,
		Name:         cfg.Name,
		EntityID:     cfg.EntityID,
		SSOURL:       cfg.SSOURL,
		SLOURL:       cfg.SLOURL,
		Certificate:  cert,
		NameIDFormat: cfg.NameIDFormat,
		AttributeMap: attrMap,
	}
}

// maskCertificate returns the config with the certificate field
// replaced by a fingerprint + masked preview. Prevents the raw
// cert from being logged in proxy access logs.
func maskCertificate(cfg *types.SAMLIdPConfig) gin.H {
	if cfg == nil {
		return nil
	}
	cert := cfg.Certificate
	preview := ""
	if len(cert) > 32 {
		preview = cert[:16] + "..." + cert[len(cert)-16:]
	} else {
		preview = cert
	}
	return gin.H{
		"id":                  cfg.ID,
		"tenant_id":           cfg.TenantID,
		"name":                cfg.Name,
		"entity_id":           cfg.EntityID,
		"sso_url":             cfg.SSOURL,
		"slo_url":             cfg.SLOURL,
		"certificate_set":     cert != "",
		"certificate_preview": preview,
		"name_id_format":      cfg.NameIDFormat,
		"attribute_map":       cfg.AttributeMap,
		"enabled":             cfg.Enabled,
		"created_at":          cfg.CreatedAt,
		"updated_at":          cfg.UpdatedAt,
	}
}

// resolveTenantID maps a tenant slug to the numeric tenant id.
// Today the slug IS the numeric id encoded as a string; a future
// commit will replace this with a slug → id lookup table.
func (h *SAMLHandler) resolveTenantID(c *gin.Context, slug string) (uint64, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return 0, errors.New("tenant slug is required")
	}
	var id uint64
	if _, err := fmtParseUint(slug, &id); err != nil {
		return 0, errors.New("invalid tenant slug")
	}
	if id == 0 {
		return 0, errors.New("invalid tenant slug")
	}
	return id, nil
}

// fmtParseUint is a tiny shim to avoid importing fmt for one
// Sscanf-style call.
func fmtParseUint(s string, out *uint64) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
	}
	*out = uint64(n)
	return n, nil
}

// tenantIDFromContext extracts the tenant id from the request
// context via the canonical middleware helper.
func tenantIDFromContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get("tenant_id")
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		return uint64(x), true
	case int:
		return uint64(x), true
	}
	return 0, false
}

// marshalCertificate is exposed for tests; returns the PEM-encoded
// form of a DER byte slice.
func marshalCertificate(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// Compile-time guard: pull url so future expansion that needs to
// build redirect targets does not have to touch imports.
var _ = url.QueryEscape
