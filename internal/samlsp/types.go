package samlsp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/crewjam/saml"
)

// SPConfig holds the WeKnora-side Service Provider configuration.
// One SPConfig is shared across all tenants because the entity ID,
// ACS URL, and SLO URL are product-level constants; only the IdP
// side differs per tenant.
type SPConfig struct {
	// EntityID is the SP entity id advertised in metadata and used
	// in the AuthnRequest issuer. Must be stable per environment.
	EntityID string

	// ACSURL is the assertion consumer service URL — the browser is
	// POSTed here with the SAML Response after IdP authentication.
	ACSURL string

	// SLOURL is the single logout service URL (optional).
	SLOURL string

	// MetadataURL is the SP metadata endpoint URL.
	MetadataURL string

	// Key + Certificate sign our AuthnRequests and the SP
	// metadata document. Operators provision these via the admin
	// UI; the private key is stored encrypted at rest (envelope
	// encryption via the tenant secret KMS).
	Key         *rsa.PrivateKey
	Certificate *x509.Certificate
}

// Validate checks the SPConfig is internally consistent.
func (c *SPConfig) Validate() error {
	if c.EntityID == "" {
		return errors.New("samlsp: EntityID is required")
	}
	if _, err := url.Parse(c.ACSURL); err != nil {
		return fmt.Errorf("samlsp: ACSURL parse: %w", err)
	}
	if c.Key == nil {
		return errors.New("samlsp: Key is required")
	}
	if c.Certificate == nil {
		return errors.New("samlsp: Certificate is required")
	}
	return nil
}

// ServiceProvider builds the crewjam/saml ServiceProvider that the
// library uses internally to build requests and parse responses.
// The IdP descriptor is plugged in at call time because it is
// tenant-specific.
func (c *SPConfig) ServiceProvider(idpMetadata *saml.EntityDescriptor) saml.ServiceProvider {
	return saml.ServiceProvider{
		EntityID:          c.EntityID,
		Key:               c.Key,
		Certificate:       c.Certificate,
		MetadataURL:       parseURL(c.MetadataURL),
		AcsURL:            parseURL(c.ACSURL),
		SloURL:            parseURL(c.SLOURL),
		IDPMetadata:       idpMetadata,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
		MetadataValidDuration: 24 * time.Hour,
		AllowIDPInitiated: false,
	}
}

func parseURL(raw string) url.URL {
	if raw == "" {
		return url.URL{}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return url.URL{}
	}
	return *u
}

// IdPConfig holds the per-tenant Identity Provider descriptor. The
// IdPMetadata field is the parsed saml.EntityDescriptor (decoded
// from the XML the admin pasted into the UI or fetched from a
// well-known URL).
type IdPConfig struct {
	// TenantID is the tenant this IdP belongs to. Used as the
	// foreign key in saml_idp_configs.
	TenantID uint64
	// Name is a human-friendly label shown in the admin UI.
	Name string
	// EntityID is the IdP entity id from the metadata.
	EntityID string
	// SSOURL is the IdP single sign-on URL (HTTP-Redirect binding).
	SSOURL string
	// SLOURL is the IdP single logout URL (optional).
	SLOURL string
	// Certificate is the IdP signing certificate (PEM-decoded).
	// We do not store the IdP metadata XML — only the fields we
	// actually need, which makes rotation easier.
	Certificate *x509.Certificate
	// NameIDFormat is the NameID format we ask for in the
	// AuthnRequest. EmailAddress is the default; persistent /
	// transient are exposed for enterprise IdPs that prefer them.
	NameIDFormat string
	// AttributeMap maps IdP attribute friendly names to our
	// internal user fields. Empty entries mean "do not import".
	// Example: { "email": "email", "displayName": "name",
	//           "groups": "groups" }.
	AttributeMap map[string]string
}

// NameIDFormatEnum returns the typed NameID format.
func (c *IdPConfig) NameIDFormatEnum() saml.NameIDFormat {
	switch c.NameIDFormat {
	case "persistent":
		return saml.PersistentNameIDFormat
	case "transient":
		return saml.TransientNameIDFormat
	case "unspecified":
		return saml.UnspecifiedNameIDFormat
	default:
		return saml.EmailAddressNameIDFormat
	}
}

// AuthnRequestResult is the output of building an AuthnRequest.
type AuthnRequestResult struct {
	// URL is the IdP SSO URL with the SAMLRequest query parameter
	// and (if applicable) Signature + SigAlg parameters attached.
	URL string
	// RequestID is the value of the ID attribute on the
	// AuthnRequest XML; the IdP echoes it back in the Response.
	RequestID string
	// RelayState is the opaque value we attach; for WeKnora it
	// carries the tenant id encoded as base64 so the ACS can
	// route the response to the right IdP lookup.
	RelayState string
}

// EncodedAuthnRequest is the b64-encoded XML AuthnRequest body used
// in the HTTP-POST binding. Returned alongside AuthnRequestResult
// when the caller wants to render an auto-submitting form instead
// of a redirect.
type EncodedAuthnRequest struct {
	SAMLRequest string // base64-encoded AuthnRequest XML
	RelayState  string
}

// Assertion is the parsed, signature-validated assertion extracted
// from a SAML Response. The handler turns this into a WeKnora user
// session.
type Assertion struct {
	NameID         string
	NameQualifier  string
	Attributes     map[string][]string
	SessionIndex   string
	NotOnOrAfter   time.Time
	Issuer         string
	Recipient      string
	InResponseTo   string
}

// Validate checks the assertion is fresh and addressed to us.
func (a *Assertion) Validate(spEntityID string, maxClockSkew time.Duration) error {
	if a == nil {
		return errors.New("samlsp: assertion is nil")
	}
	if a.NameID == "" {
		return errors.New("samlsp: assertion missing NameID")
	}
	if a.Recipient != "" && spEntityID != "" && a.Recipient != spEntityID {
		return fmt.Errorf("samlsp: assertion recipient %q != sp %q", a.Recipient, spEntityID)
	}
	if a.NotOnOrAfter.IsZero() {
		return errors.New("samlsp: assertion missing NotOnOrAfter")
	}
	if time.Now().Add(maxClockSkew).After(a.NotOnOrAfter) {
		return errors.New("samlsp: assertion expired")
	}
	return nil
}

// B64 is a tiny helper used by templates when rendering the
// HTTP-POST binding auto-submit form.
func B64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
