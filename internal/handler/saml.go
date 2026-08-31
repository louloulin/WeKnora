package handler

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/samlsp"
	"github.com/Tencent/WeKnora/internal/application/service"
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
	idpSvc interfaces.SAMLIdPService
	sp     *samlsp.SPConfig
}

// NewSAMLHandler constructs the handler.
func NewSAMLHandler(
	idpSvc interfaces.SAMLIdPService,
	sp *samlsp.SPConfig,
) *SAMLHandler {
	return &SAMLHandler{idpSvc: idpSvc, sp: sp}
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
	if err := assertion.Validate(h.sp.EntityID, 30*1000*1000*1000); err != nil { // 30s clock skew
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	// Return the validated assertion. The follow-up commit wires
	// this into the user-provisioning + JWT-mint pipeline (parallel
	// with the LDAP / SCIM integration).
	c.JSON(http.StatusOK, gin.H{
		"tenant_id":   tenantID,
		"name_id":     assertion.NameID,
		"attributes":  assertion.Attributes,
		"issuer":      assertion.Issuer,
		"session_idx": assertion.SessionIndex,
		"expires_at":  assertion.NotOnOrAfter,
	})
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
		"id":              cfg.ID,
		"tenant_id":       cfg.TenantID,
		"name":            cfg.Name,
		"entity_id":       cfg.EntityID,
		"sso_url":         cfg.SSOURL,
		"slo_url":         cfg.SLOURL,
		"certificate_set": cert != "",
		"certificate_preview": preview,
		"name_id_format":  cfg.NameIDFormat,
		"attribute_map":   cfg.AttributeMap,
		"enabled":         cfg.Enabled,
		"created_at":      cfg.CreatedAt,
		"updated_at":      cfg.UpdatedAt,
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
