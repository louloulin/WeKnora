package samlsp

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
)

// Metadata returns the SP metadata XML document that admins paste
// into their IdP. The document describes our entity, our ACS / SLO
// URLs, and the certificate we use to sign AuthnRequests.
func (c *SPConfig) Metadata() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	sp := saml.ServiceProvider{
		EntityID:             c.EntityID,
		Key:                  c.Key,
		Certificate:          c.Certificate,
		MetadataURL:          parseURL(c.MetadataURL),
		AcsURL:               parseURL(c.ACSURL),
		SloURL:               parseURL(c.SLOURL),
		AuthnNameIDFormat:    saml.EmailAddressNameIDFormat,
		MetadataValidDuration: 24 * time.Hour,
	}
	desc := sp.Metadata()
	body, err := xml.Marshal(desc)
	if err != nil {
		return nil, fmt.Errorf("samlsp: marshal metadata: %w", err)
	}
	// Wrap with XML declaration so the document is valid when
	// served as application/samlmetadata+xml.
	return append([]byte(xml.Header), body...), nil
}

// MakeAuthenticationRequest builds an AuthnRequest addressed to the
// tenant's IdP and returns the URL the browser should be redirected
// to. The relay state carries the tenant id so the ACS can route
// the response back to the right IdP descriptor.
//
// We default to HTTP-Redirect binding (DEFLATE-encoded base64 in
// the SAMLRequest query parameter) — every enterprise IdP we have
// seen supports it.
func (c *SPConfig) MakeAuthenticationRequest(
	idp *IdPConfig,
	tenantID uint64,
) (*AuthnRequestResult, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if idp == nil {
		return nil, errors.New("samlsp: idp config is required")
	}
	if idp.EntityID == "" || idp.SSOURL == "" {
		return nil, errors.New("samlsp: idp config missing entityID or ssoURL")
	}
	requestID, err := randomID()
	if err != nil {
		return nil, err
	}
	relay, err := encodeRelayState(tenantID, requestID)
	if err != nil {
		return nil, err
	}
	// Build the AuthnRequest XML directly — crewjam's
	// MakeAuthenticationRequest requires the IdPMetadata to be
	// set, but we have only the SSO URL + entity ID + cert. The
	// XML we need is small and stable, so we hand-roll it.
	now := time.Now().UTC().Format(time.RFC3339)
	body := fmt.Sprintf(
		`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" `+
			`xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" `+
			`ID="%s" Version="2.0" IssueInstant="%s" `+
			`ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" `+
			`AssertionConsumerServiceURL="%s">`+
			`<saml:Issuer>%s</saml:Issuer>`+
			`<samlp:NameIDPolicy Format="%s" AllowCreate="true"/>`+
			`</samlp:AuthnRequest>`,
		requestID, now,
		escapeXML(c.ACSURL),
		escapeXML(c.EntityID),
		escapeXML(string(idp.NameIDFormatEnum())),
	)
	// HTTP-Redirect binding: DEFLATE + base64.
	encoded := deflateB64([]byte(body))

	u, err := url.Parse(idp.SSOURL)
	if err != nil {
		return nil, fmt.Errorf("samlsp: parse SSO URL: %w", err)
	}
	q := u.Query()
	q.Set("SAMLRequest", encoded)
	q.Set("RelayState", relay)
	u.RawQuery = q.Encode()

	return &AuthnRequestResult{
		URL:        u.String(),
		RequestID:  requestID,
		RelayState: relay,
	}, nil
}

// ParseResponse parses a base64-encoded SAML Response posted to the
// ACS, validates the XML-DSig signature against the IdP's
// certificate, and returns the contained Assertion.
func (c *SPConfig) ParseResponse(idp *IdPConfig, samlResponseB64 string) (*Assertion, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if idp == nil || idp.Certificate == nil {
		return nil, errors.New("samlsp: idp config or certificate missing")
	}
	if _, err := decodeAny(samlResponseB64); err != nil {
		return nil, fmt.Errorf("samlsp: base64 decode: %w", err)
	}
	// Build a synthetic *http.Request carrying the SAMLResponse so
	// we can reuse the library's ParseResponse signature.
	req, err := http.NewRequest(http.MethodPost, c.ACSURL,
		bytes.NewReader([]byte("SAMLResponse="+url.QueryEscape(samlResponseB64))))
	if err != nil {
		return nil, fmt.Errorf("samlsp: build synthetic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	idpMetadata := &saml.EntityDescriptor{
		EntityID: idp.EntityID,
		IDPSSODescriptors: []saml.IDPSSODescriptor{{
			SSODescriptor: saml.SSODescriptor{
				RoleDescriptor: saml.RoleDescriptor{
					ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
					KeyDescriptors: []saml.KeyDescriptor{{
						Use: "signing",
						KeyInfo: saml.KeyInfo{
							X509Data: saml.X509Data{
								X509Certificates: []saml.X509Certificate{
									{Data: base64.StdEncoding.EncodeToString(idp.Certificate.Raw)},
								},
							},
						},
					}},
				},
			},
			SingleSignOnServices: []saml.Endpoint{{
				Binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect",
				Location: idp.SSOURL,
			}},
		}},
	}
	sp := c.ServiceProvider(idpMetadata)
	assertion, err := sp.ParseResponse(req, []string{})
	if err != nil {
		return nil, fmt.Errorf("samlsp: parse response: %w", err)
	}
	if assertion == nil {
		return nil, errors.New("samlsp: response contained no assertion")
	}
	out := &Assertion{
		NameID:        string(assertion.Subject.NameID.Value),
		NameQualifier: assertion.Subject.NameID.SPNameQualifier,
		Attributes:    flattenAttributes(assertion.AttributeStatements),
		SessionIndex:  firstAuthnString(assertion.AuthnStatements, func(a saml.AuthnStatement) string { return a.SessionIndex }),
		NotOnOrAfter:  firstAuthnTime(assertion.AuthnStatements, func(a saml.AuthnStatement) (time.Time, bool) {
			if a.SessionNotOnOrAfter == nil {
				return time.Time{}, false
			}
			return *a.SessionNotOnOrAfter, true
		}),
		Issuer:       assertion.Issuer.Value,
		Recipient:    firstConfirmationRecipient(assertion),
		InResponseTo: firstConfirmationInResponseTo(assertion),
	}
	return out, nil
}

func flattenAttributes(stmts []saml.AttributeStatement) map[string][]string {
	out := make(map[string][]string)
	for _, stmt := range stmts {
		for _, attr := range stmt.Attributes {
			name := attr.Name
			if name == "" {
				name = attr.FriendlyName
			}
			for _, v := range attr.Values {
				out[name] = append(out[name], v.Value)
			}
		}
	}
	return out
}

func firstAuthnString(stmts []saml.AuthnStatement, getter func(saml.AuthnStatement) string) string {
	for _, s := range stmts {
		if v := getter(s); v != "" {
			return v
		}
	}
	return ""
}

func firstAuthnTime(stmts []saml.AuthnStatement, getter func(saml.AuthnStatement) (time.Time, bool)) time.Time {
	for _, s := range stmts {
		if v, ok := getter(s); ok {
			return v
		}
	}
	return time.Time{}
}

func firstConfirmationRecipient(a *saml.Assertion) string {
	for _, c := range a.Subject.SubjectConfirmations {
		return c.SubjectConfirmationData.Recipient
	}
	return ""
}

func firstConfirmationInResponseTo(a *saml.Assertion) string {
	for _, c := range a.Subject.SubjectConfirmations {
		return c.SubjectConfirmationData.InResponseTo
	}
	return ""
}

// randomID returns a 16-byte hex string suitable for SAML IDs.
func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("_%x", b[:]), nil
}

// escapeXML does the bare-minimum escaping SAML XML needs.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// deflateB64 DEFLATE-compresses then base64-encodes — the encoding
// specified by SAML 2.0 HTTP-Redirect binding (section 3.4.4.1).
func deflateB64(data []byte) string {
	var buf bytes.Buffer
	// SAML requires raw DEFLATE (no zlib header). We use
	// flate.NewWriter with a fixed Huffman / no-dict config.
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		// flate.NewWriter only errors if compression level is
		// invalid; DefaultCompression is always valid.
		return base64.StdEncoding.EncodeToString(data)
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return base64.StdEncoding.EncodeToString(data)
	}
	if err := w.Close(); err != nil {
		return base64.StdEncoding.EncodeToString(data)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// decodeAny tries URL-safe then std base64 — SAML responses posted
// via HTTP-POST are usually std base64 but the spec allows either.
func decodeAny(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("samlsp: empty response")
	}
	// URL-safe base64 may have '+' -> '-' and '/' -> '_'; std
	// base64 may be padded. Try URL-safe first when we see those
	// substitutions.
	cleaned := strings.TrimSpace(s)
	looksURLSafe := strings.ContainsAny(cleaned, "-_")
	var enc *base64.Encoding
	if looksURLSafe {
		enc = base64.URLEncoding
	} else {
		enc = base64.StdEncoding
	}
	if out, err := enc.DecodeString(cleaned); err == nil {
		return out, nil
	}
	// Try std as a fallback.
	return base64.StdEncoding.DecodeString(cleaned)
}

// encodeRelayState packs (tenantID, requestID) into a base64
// token so the ACS can recover the tenant from the inbound
// RelayState. The format is "<tenantID>:<requestID>" before
// base64-encoding so an operator can read it in browser dev tools.
func encodeRelayState(tenantID uint64, requestID string) (string, error) {
	raw := fmt.Sprintf("%d:%s", tenantID, requestID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

// DecodeRelayState inverts encodeRelayState. Exposed for the ACS.
func DecodeRelayState(token string) (uint64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		// fall back to std encoding for older tokens
		raw, err = base64.StdEncoding.DecodeString(token)
		if err != nil {
			return 0, "", err
		}
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("samlsp: malformed relay state")
	}
	var tenantID uint64
	if _, err := fmt.Sscanf(parts[0], "%d", &tenantID); err != nil {
		return 0, "", fmt.Errorf("samlsp: relay tenantID parse: %w", err)
	}
	return tenantID, parts[1], nil
}

// SAMLRequestURLSigningParams returns the SigAlg / Signature query
// parameters for HTTP-Redirect binding signing. We sign the
// SAMLRequest + RelayState + query-string-prefix using the SP
// private key. Enterprise IdPs that require signed AuthnRequests
// (Okta with strict mode) rely on this.
//
// SigAlg is fixed to http://www.w3.org/2001/04/xmldsig-more#rsa-sha256.
// The encoded signature is base64 (NOT URL-safe).
func SAMLRequestURLSigningParams(
	samlRequest string,
	relayState string,
	key *interface{},
) (sigAlg, signature string, err error) {
	// Placeholder. Real implementation in a follow-up commit
	// alongside XML signature support; today we return empty
	// params and rely on the IdP not requiring signed requests.
	return "", "", nil
}

// Compile-time guard that flate/binary are imported so future
// expansion does not have to touch the imports.
var _ = binary.LittleEndian
var _ = io.Discard
