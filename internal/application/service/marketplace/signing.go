package marketplace

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// ErrUntrustedPlugin is returned when the manifest's signature
// does not verify against any registered vendor public key.
var ErrUntrustedPlugin = errors.New("marketplace: untrusted plugin signature")

// ErrInvalidManifest is returned when required manifest fields are
// missing or malformed.
var ErrInvalidManifest = errors.New("marketplace: invalid manifest")

// CanonicalBytesProvider is the minimal interface the verifier needs
// from a manifest. The types.PluginManifest implements it, but we
// declare the dependency here so this package stays free of any
// types-layer imports (no cycles).
type CanonicalBytesProvider interface {
	CanonicalBytes() ([]byte, error)
}

// Signer produces the base64 RSA-PSS signature for a manifest.
func Signer(m CanonicalBytesProvider, key *rsa.PrivateKey) (string, error) {
	bytes, err := m.CanonicalBytes()
	if err != nil {
		return "", err
	}
	hashed := sha256.Sum256(bytes)
	sig, err := rsa.SignPSS(rand.Reader, key, crypto.SHA256, hashed[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// SignatureInput is the minimal input surface the verifier needs.
// Defining it locally avoids the import cycle with internal/types.
type SignatureInput interface {
	CanonicalBytesProvider
	GetAuthorPublicKey() string
	GetSignature() string
}

// VerifySignature parses the PEM-encoded public key and verifies
// the signature against the canonical manifest bytes.
func VerifySignature(m SignatureInput) error {
	if m.GetSignature() == "" {
		return fmt.Errorf("%w: missing signature", ErrInvalidManifest)
	}
	if m.GetAuthorPublicKey() == "" {
		return fmt.Errorf("%w: missing public key", ErrInvalidManifest)
	}
	block, _ := pem.Decode([]byte(m.GetAuthorPublicKey()))
	if block == nil {
		return fmt.Errorf("%w: public key is not PEM", ErrInvalidManifest)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: parse public key: %v", ErrInvalidManifest, err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: public key is not RSA", ErrInvalidManifest)
	}
	sig, err := base64.StdEncoding.DecodeString(m.GetSignature())
	if err != nil {
		return fmt.Errorf("%w: signature is not base64", ErrInvalidManifest)
	}
	canonical, err := m.CanonicalBytes()
	if err != nil {
		return err
	}
	hashed := sha256.Sum256(canonical)
	if err := rsa.VerifyPSS(rsaPub, crypto.SHA256, hashed[:], sig, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrUntrustedPlugin, err)
	}
	return nil
}
