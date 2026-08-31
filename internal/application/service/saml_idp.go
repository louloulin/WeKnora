package service

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ErrSAMLIdPAlreadyExists is returned when a tenant tries to
// create a second IdP config (the schema enforces this with a
// unique index on tenant_id).
var (
	ErrSAMLIdPAlreadyExists = errors.New("saml: idp config already exists for tenant")
	// ErrSAMLIdPNotFound is re-exported from the repository so
	// callers can use a single sentinel regardless of layer.
	ErrSAMLIdPNotFound = repository.ErrSAMLIdPNotFound
)

// ErrSAMLInvalidCertificate is returned when the admin pastes a
// certificate the service cannot parse. The error message is
// safe to surface to admin UIs but never to end users.
var ErrSAMLInvalidCertificate = errors.New("saml: certificate is not a valid PEM or base64 DER X.509")

// samlIdPService is the per-tenant IdP CRUD service.
type samlIdPService struct {
	repo interfaces.SAMLIdPRepository
}

// NewSAMLIdPService wires the service.
func NewSAMLIdPService(repo interfaces.SAMLIdPRepository) interfaces.SAMLIdPService {
	return &samlIdPService{repo: repo}
}

// Create validates the request, parses the certificate, and
// persists the config.
func (s *samlIdPService) Create(ctx context.Context, tenantID uint64, req types.SAMLIdPConfigCreateRequest) (*types.SAMLIdPConfig, error) {
	cert, err := parseCertificate(req.Certificate)
	if err != nil {
		return nil, err
	}
	if existing, _ := s.repo.GetByTenantID(ctx, tenantID); existing != nil {
		return nil, ErrSAMLIdPAlreadyExists
	}
	cfg := &types.SAMLIdPConfig{
		TenantID:     tenantID,
		Name:         req.Name,
		EntityID:     req.EntityID,
		SSOURL:       req.SSOURL,
		SLOURL:       req.SLOURL,
		Certificate:  base64.StdEncoding.EncodeToString(cert.Raw),
		NameIDFormat: defaultIfEmpty(req.NameIDFormat, "email"),
		AttributeMap: toJSONMap(req.AttributeMap),
		Enabled:      req.Enabled == nil || *req.Enabled,
	}
	if err := s.repo.Create(ctx, cfg); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "saml: created IdP config %s for tenant %d", cfg.Name, tenantID)
	return cfg, nil
}

// Get returns the IdP config for a tenant.
func (s *samlIdPService) Get(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error) {
	return s.repo.GetByTenantID(ctx, tenantID)
}

// GetEnabled is the fast-path used by the SAML login flow.
func (s *samlIdPService) GetEnabled(ctx context.Context, tenantID uint64) (*types.SAMLIdPConfig, error) {
	cfg, err := s.repo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrSAMLIdPNotFound
	}
	return cfg, nil
}

// Update mutates an existing IdP config. Only non-nil fields in
// the request are applied.
func (s *samlIdPService) Update(ctx context.Context, tenantID uint64, req types.SAMLIdPConfigUpdateRequest) (*types.SAMLIdPConfig, error) {
	cfg, err := s.repo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.SSOURL != nil {
		cfg.SSOURL = *req.SSOURL
	}
	if req.SLOURL != nil {
		cfg.SLOURL = *req.SLOURL
	}
	if req.Certificate != nil {
		cert, err := parseCertificate(*req.Certificate)
		if err != nil {
			return nil, err
		}
		cfg.Certificate = base64.StdEncoding.EncodeToString(cert.Raw)
	}
	if req.NameIDFormat != nil {
		cfg.NameIDFormat = *req.NameIDFormat
	}
	if req.AttributeMap != nil {
		cfg.AttributeMap = toJSONMap(*req.AttributeMap)
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if err := s.repo.Update(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Delete soft-deletes the IdP config.
func (s *samlIdPService) Delete(ctx context.Context, tenantID uint64) error {
	return s.repo.Delete(ctx, tenantID)
}

// parseCertificate accepts either PEM (-----BEGIN CERTIFICATE-----)
// or base64-encoded DER and returns the parsed *x509.Certificate.
// The service is the gatekeeper — only validated certs are stored.
func parseCertificate(raw string) (*x509.Certificate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrSAMLInvalidCertificate
	}
	if strings.HasPrefix(raw, "-----BEGIN") {
		block, _ := pem.Decode([]byte(raw))
		if block == nil {
			return nil, ErrSAMLInvalidCertificate
		}
		return x509.ParseCertificate(block.Bytes)
	}
	// Try base64-encoded DER.
	der, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode failed", ErrSAMLInvalidCertificate)
	}
	return x509.ParseCertificate(der)
}

func defaultIfEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}


// toJSONMap converts the typed attribute map (map[string]string)
// the request body uses into the JSONMap (map[string]any) the
// model layer stores. Conversion is value-preserving; multi-valued
// attributes land in the service layer later if the IdP ever
// emits them.
func toJSONMap(in map[string]string) types.JSONMap {
	out := make(types.JSONMap, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
