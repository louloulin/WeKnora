package mfasp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DefaultDigits is the standard 6-digit TOTP code length.
const DefaultDigits = 6

// DefaultPeriod is the 30-second time step the overwhelming
// majority of authenticator apps use.
const DefaultPeriod = 30

// DefaultAlgorithm is SHA1 per RFC 6238 §1.2. SHA256 and SHA512 are
// allowed by the standard but rejected by every major authenticator
// app, so we hard-code SHA1.
const DefaultAlgorithm = "SHA1"

// SecretLength is 20 bytes (160 bits), the recommended secret size
// in RFC 4226 §5.3.
const SecretLength = 20

// skewTolerance is the number of periods we accept before / after
// the current step. ±1 is the industry default and accommodates
// clock drift up to ~30 seconds.
const skewTolerance = 1

// RecoveryCodeLength is the alphanumeric length of a single
// recovery code (excludes dashes). 10 chars from a 32-symbol
// alphabet gives ~50 bits of entropy.
const RecoveryCodeLength = 10

// RecoveryAlphabet is the human-friendly alphabet used for
// recovery codes. Excludes 0/O, 1/I/L to reduce transcription
// mistakes.
const RecoveryAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// Errors surfaced to the handler / service layers.
var (
	ErrInvalidSecret  = errors.New("mfasp: secret must be 20 raw bytes (160 bits)")
	ErrInvalidCode    = errors.New("mfasp: code must be 6 digits")
	ErrCodeOutOfRange = errors.New("mfasp: code is outside the accepted drift window")
)

// Secret is a base32-encoded TOTP secret. We store the base32 form
// (no padding) so the persistence layer never has to think about
// raw bytes; encoding is reversible via DecodeSecret.
type Secret string

// GenerateSecret returns a fresh base32 secret of SecretLength
// bytes. Crypto/rand — never math/rand — because the secret is the
// only thing standing between an attacker and a forged TOTP.
func GenerateSecret() (Secret, error) {
	raw := make([]byte, SecretLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mfasp: read random: %w", err)
	}
	return Secret(strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")), nil
}

// DecodeSecret returns the raw 20-byte secret backing the base32
// string. Used for HMAC but never sent to the client after
// enrollment.
func decodeSecret(s Secret) ([]byte, error) {
	// Accept base32 with or without padding (Google Authenticator
	// exports without; we accept both).
	encoded := string(s)
	if pad := len(encoded) % 8; pad != 0 {
		encoded += strings.Repeat("=", 8-pad)
	}
	raw, err := base32.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSecret, err)
	}
	if len(raw) != SecretLength {
		return nil, ErrInvalidSecret
	}
	return raw, nil
}

// Code returns the current TOTP code for the secret at time t. The
// period defaults to 30s; pass an alternative via the Options
// argument if a non-default is needed (kept around for tests).
func (s Secret) Code(t time.Time) (string, error) {
	return s.CodeAt(t, DefaultPeriod)
}

// CodeAt is the period-explicit form.
func (s Secret) CodeAt(t time.Time, periodSeconds int) (string, error) {
	raw, err := decodeSecret(s)
	if err != nil {
		return "", err
	}
	if periodSeconds <= 0 {
		periodSeconds = DefaultPeriod
	}
	counter := uint64(t.Unix()) / uint64(periodSeconds)
	return hotp(raw, counter, DefaultDigits), nil
}

// Verify checks the supplied 6-digit code against the current and
// adjacent time steps (±skewTolerance). The returned bool is true
// when the code is valid; the second return value is the step
// counter that matched (useful for replay-prevention) or 0 when no
// match was found.
//
// The match uses crypto/subtle.ConstantTimeCompare so the timing
// does not leak which digit position diverged first. Callers should
// persist the matched counter to a "last_used_counter" column and
// reject any code whose counter is <= last_used_counter.
func (s Secret) Verify(code string, t time.Time) (bool, uint64, error) {
	return s.VerifyWithOptions(code, t, VerifyOptions{})
}

// VerifyOptions tunes the verifier. Period defaults to 30s; skew
// defaults to ±1.
type VerifyOptions struct {
	Period int
	Skew   int
}

// VerifyWithOptions is the explicit form of Verify.
func (s Secret) VerifyWithOptions(code string, t time.Time, opts VerifyOptions) (bool, uint64, error) {
	raw, err := decodeSecret(s)
	if err != nil {
		return false, 0, err
	}
	period := opts.Period
	if period <= 0 {
		period = DefaultPeriod
	}
	skew := opts.Skew
	if skew < 0 {
		skew = 0
	}
	if skew == 0 {
		skew = skewTolerance
	}
	// Normalise the code: drop spaces/dashes, enforce digit-only.
	norm := normaliseCode(code)
	if len(norm) != DefaultDigits {
		return false, 0, ErrInvalidCode
	}
	currentCounter := uint64(t.Unix()) / uint64(period)
	for offset := -skew; offset <= skew; offset++ {
		var c uint64
		if offset >= 0 {
			c = currentCounter + uint64(offset)
		} else {
			c = currentCounter - uint64(-offset)
		}
		expected := hotp(raw, c, DefaultDigits)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(norm)) == 1 {
			return true, c, nil
		}
	}
	return false, 0, nil
}

// ProvisioningURI returns the otpauth:// URI for the secret. The
// QR code that authenticator apps scan is the rendering of this URI.
// Mirrors the Google Authenticator key-uri format exactly so the
// enrollment works in every major app.
//
// account is typically the user's email; issuer should match the
// deployment name shown to end users.
func (s Secret) ProvisioningURI(account, issuer string) string {
	q := url.Values{}
	q.Set("secret", string(s))
	q.Set("issuer", issuer)
	q.Set("algorithm", DefaultAlgorithm)
	q.Set("digits", fmt.Sprintf("%d", DefaultDigits))
	q.Set("period", fmt.Sprintf("%d", DefaultPeriod))
	label := url.QueryEscape(issuer)
	if account != "" {
		label = url.QueryEscape(issuer) + ":" + url.QueryEscape(account)
	}
	return fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())
}

// hotp implements RFC 4226 §5.3 dynamic truncation. The counter
// is encoded big-endian into 8 bytes (HMAC-SHA1 input length).
func hotp(secret []byte, counter uint64, digits int) string {
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(counterBytes[:])
	sum := mac.Sum(nil)
	// Dynamic truncation per RFC 4226 §5.3.
	offset := sum[len(sum)-1] & 0x0F
	truncated := (uint32(sum[offset])&0x7F)<<24 |
		(uint32(sum[offset+1])&0xFF)<<16 |
		(uint32(sum[offset+2])&0xFF)<<8 |
		(uint32(sum[offset+3]) & 0xFF)
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	code := truncated % mod
	// Zero-pad to digit count.
	return fmt.Sprintf("%0*d", digits, code)
}

// normaliseCode strips whitespace and dashes from a user-entered
// code; many authenticator apps display "123 456" rather than
// "123456". Anything non-digit is rejected.
func normaliseCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '\t':
			// skip
		default:
			return ""
		}
	}
	return b.String()
}
