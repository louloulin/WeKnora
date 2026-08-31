package container

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"strings"
	"github.com/Tencent/WeKnora/internal/samlsp"
	"time"
)

// decodePEMCertificate parses a PEM-encoded CERTIFICATE block.
func decodePEMCertificate(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("saml: cert PEM decode failed")
	}
	return x509.ParseCertificate(block.Bytes)
}

// decodePEMPrivateKey parses a PEM-encoded RSA PRIVATE KEY block.
func decodePEMPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("saml: key PEM decode failed")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	// Fall back to PKCS#8.
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
	}
	return nil, errors.New("saml: unsupported private key format (use PKCS#1 or PKCS#8 RSA)")
}

// generateDevCert creates an ephemeral self-signed cert for local
// development. The key is discarded with the process — never use
// this in production.
func generateDevCert() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      pkix.Name{CommonName: "weknora-saml-sp-dev"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// newSAMLSPConfig reads the SP-side SAML configuration from the
// process environment. The cert + key are PEM-encoded; we generate
// a self-signed pair on first boot when neither env var is set so
// the SP works out of the box for local development. Production
// deployments must provision SAML_SP_CERT_PEM + SAML_SP_KEY_PEM.
//
// TODO: read the SP cert from the admin-managed settings table
// once the SAML admin UI lands (today the admin can only manage
// the IdP-side config; the SP side is operator-managed).
func newSAMLSPConfig() (*samlsp.SPConfig, error) {
	cfg := &samlsp.SPConfig{
		EntityID:    envOr("SAML_SP_ENTITY_ID", "urn:weknora:saml:sp"),
		ACSURL:      envOr("SAML_SP_ACS_URL", "http://localhost:8080/api/v1/auth/saml/acs"),
		SLOURL:      envOr("SAML_SP_SLO_URL", "http://localhost:8080/api/v1/auth/saml/slo"),
		MetadataURL: envOr("SAML_SP_METADATA_URL", "http://localhost:8080/api/v1/auth/saml/metadata"),
	}
	certPEM := envOr("SAML_SP_CERT_PEM", "")
	keyPEM := envOr("SAML_SP_KEY_PEM", "")
	if certPEM != "" && keyPEM != "" {
		cert, err := decodePEMCertificate(certPEM)
		if err != nil {
			return nil, err
		}
		key, err := decodePEMPrivateKey(keyPEM)
		if err != nil {
			return nil, err
		}
		cfg.Certificate = cert
		cfg.Key = key
	} else {
		// Dev fallback: ephemeral self-signed cert. The key never
		// leaves the process so this is acceptable for local dev.
		cert, key, err := generateDevCert()
		if err != nil {
			return nil, err
		}
		cfg.Certificate = cert
		cfg.Key = key
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
