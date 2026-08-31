package samlsp

import (
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
)

// testSP builds a self-signed SPConfig + IdPConfig for tests.
// Returns the SPConfig and an IdPConfig whose certificate is the
// same as the SP (tests that need a separate IdP cert can override).
func testSP(t *testing.T) (*SPConfig, *IdPConfig) {
	t.Helper()
	spKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen sp key: %v", err)
	}
	spCert := selfSigned(t, spKey, "CN=weknora-sp-test")
	idpCert := selfSigned(t, spKey, "CN=test-idp")
	sp := &SPConfig{
		EntityID:    "https://weknora.example/saml/metadata",
		ACSURL:      "https://weknora.example/saml/acs",
		SLOURL:      "https://weknora.example/saml/slo",
		MetadataURL: "https://weknora.example/saml/metadata",
		Key:         spKey,
		Certificate: spCert,
	}
	idp := &IdPConfig{
		TenantID:     42,
		Name:         "Test IdP",
		EntityID:     "https://idp.example/saml/metadata",
		SSOURL:      "https://idp.example/saml/sso",
		SLOURL:      "https://idp.example/saml/slo",
		Certificate: idpCert,
		NameIDFormat: "email",
		AttributeMap: map[string]string{
			"email":       "email",
			"displayName": "name",
		},
	}
	return sp, idp
}

func selfSigned(t *testing.T, key *rsa.PrivateKey, cn string) *x509.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func TestSPConfig_Validate(t *testing.T) {
	sp, _ := testSP(t)
	if err := sp.Validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
	bad := &SPConfig{}
	if err := bad.Validate(); err == nil {
		t.Errorf("empty config should fail validation")
	}
}

func TestSPConfig_Metadata_RoundTrip(t *testing.T) {
	sp, _ := testSP(t)
	body, err := sp.Metadata()
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	s := string(body)
	if !strings.HasPrefix(s, `<?xml`) {
		t.Errorf("metadata missing xml header: %q", first80(s))
	}
	if !strings.Contains(s, sp.EntityID) {
		t.Errorf("metadata missing entityID: %q", first80(s))
	}
	if !strings.Contains(s, sp.ACSURL) {
		t.Errorf("metadata missing ACSURL: %q", first80(s))
	}
	if !strings.Contains(s, base64.StdEncoding.EncodeToString(sp.Certificate.Raw)) {
		t.Errorf("metadata missing signing cert: %q", first80(s))
	}
}

func TestMakeAuthenticationRequest_BuildsValidRedirect(t *testing.T) {
	sp, idp := testSP(t)
	res, err := sp.MakeAuthenticationRequest(idp, 42)
	if err != nil {
		t.Fatalf("make auth req: %v", err)
	}
	if !strings.HasPrefix(res.URL, idp.SSOURL+"?") {
		t.Errorf("URL should start with SSO URL, got %q", first80(res.URL))
	}
	if !strings.Contains(res.URL, "SAMLRequest=") {
		t.Errorf("URL missing SAMLRequest: %q", first80(res.URL))
	}
	if !strings.Contains(res.URL, "RelayState=") {
		t.Errorf("URL missing RelayState: %q", first80(res.URL))
	}
	if res.RequestID == "" {
		t.Errorf("request id empty")
	}
}

func TestRelayState_RoundTrip(t *testing.T) {
	tenant, reqID, err := DecodeRelayState(encodeRelayStateOrFatal(t, 42, "abc123"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tenant != 42 {
		t.Errorf("tenant: got %d want 42", tenant)
	}
	if reqID != "abc123" {
		t.Errorf("reqID: got %s want abc123", reqID)
	}
}

func TestRelayState_MalformedReturnsError(t *testing.T) {
	if _, _, err := DecodeRelayState("@@@not-base64@@@"); err == nil {
		t.Errorf("malformed relay state should error")
	}
	if _, _, err := DecodeRelayState(base64.RawURLEncoding.EncodeToString([]byte("no-colon"))); err == nil {
		t.Errorf("malformed payload should error")
	}
}

func TestDeflateB64_RoundTrip(t *testing.T) {
	raw := []byte(`<samlp:AuthnRequest>abc</samlp:AuthnRequest>`)
	enc := deflateB64(raw)
	if enc == base64.StdEncoding.EncodeToString(raw) {
		t.Errorf("deflate should compress, not return raw")
	}
	// Round-trip via flate reader.
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}
	r := flate.NewReader(strings.NewReader(string(dec)))
	defer r.Close()
	out := make([]byte, 1024)
	n, err := r.Read(out)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("inflate: %v", err)
	}
	if string(out[:n]) != string(raw) {
		t.Errorf("inflate mismatch: got %q want %q", out[:n], raw)
	}
}

func TestDecodeAny_HandlesURLSafeAndStd(t *testing.T) {
	raw := []byte("hello world")
	std := base64.StdEncoding.EncodeToString(raw)
	urlSafe := base64.URLEncoding.EncodeToString(raw)
	for _, c := range []string{std, urlSafe} {
		got, err := decodeAny(c)
		if err != nil {
			t.Errorf("decode %q: %v", c, err)
			continue
		}
		if string(got) != string(raw) {
			t.Errorf("decode %q = %q, want %q", c, got, raw)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	cases := map[string]string{
		`a&b<c>d"e'f`: `a&amp;b&lt;c&gt;d&quot;e&apos;f`,
		"plain":        "plain",
	}
	for in, want := range cases {
		if got := escapeXML(in); got != want {
			t.Errorf("escapeXML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseResponse_MissingCertRejected(t *testing.T) {
	sp, _ := testSP(t)
	_, err := sp.ParseResponse(&IdPConfig{}, "anything")
	if err == nil {
		t.Errorf("missing cert should error")
	}
}

func TestAssertion_Validate(t *testing.T) {
	a := &Assertion{
		NameID:       "alice@example.com",
		Recipient:    "https://weknora.example/saml/acs",
		NotOnOrAfter: time.Now().Add(time.Hour),
	}
	if err := a.Validate("https://weknora.example/saml/acs", 30*time.Second); err != nil {
		t.Errorf("fresh assertion should validate: %v", err)
	}
	expired := &Assertion{
		NameID:       "alice@example.com",
		Recipient:    "https://weknora.example/saml/acs",
		NotOnOrAfter: time.Now().Add(-time.Hour),
	}
	if err := expired.Validate("https://weknora.example/saml/acs", 0); err == nil {
		t.Errorf("expired assertion should fail validation")
	}
	wrongRecipient := &Assertion{
		NameID:       "alice@example.com",
		Recipient:    "https://other.example/acs",
		NotOnOrAfter: time.Now().Add(time.Hour),
	}
	if err := wrongRecipient.Validate("https://weknora.example/saml/acs", 0); err == nil {
		t.Errorf("wrong recipient should fail validation")
	}
	if err := (&Assertion{}).Validate("x", 0); err == nil {
		t.Errorf("empty assertion should fail validation")
	}
}

func TestNameIDFormatEnum(t *testing.T) {
	cases := map[string]saml.NameIDFormat{
		"":            saml.EmailAddressNameIDFormat,
		"email":       saml.EmailAddressNameIDFormat,
		"persistent":  saml.PersistentNameIDFormat,
		"transient":   saml.TransientNameIDFormat,
		"unspecified": saml.UnspecifiedNameIDFormat,
	}
	idp := &IdPConfig{}
	for in, want := range cases {
		idp.NameIDFormat = in
		if got := idp.NameIDFormatEnum(); got != want {
			t.Errorf("NameIDFormat(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPEMHelpers_RoundTrip(t *testing.T) {
	_, idp := testSP(t)
	pem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: idp.Certificate.Raw,
	})
	if !strings.Contains(string(pem), "BEGIN CERTIFICATE") {
		t.Errorf("PEM missing cert header")
	}
}

// helpers ------------------------------------------------------------

func encodeRelayStateOrFatal(t *testing.T, tenant uint64, reqID string) string {
	t.Helper()
	s, err := encodeRelayState(tenant, reqID)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return s
}

func first80(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "..."
}
