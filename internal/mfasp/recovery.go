package mfasp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// DefaultRecoveryCount is the number of single-use recovery codes
// generated per enrollment. 10 is the industry default (Okta,
// Auth0, Google).
const DefaultRecoveryCount = 10

// recoveryCodeSeparator is the human-friendly dash inserted every
// 5 chars when displaying a code (e.g. "abcde-fghij"). The dash
// is not part of the actual code value.
const recoveryCodeSeparator = "-"

// ErrInvalidRecovery signals the supplied recovery code did not
// match any stored hash. Surfaced to the handler as 401.
var ErrInvalidRecovery = errors.New("mfasp: recovery code invalid")

// RecoveryCode is a single recovery code. Stored on the client as
// the dashed form (a recovery scratch sheet prints them as
// "abcde-fghij") and hashed on the server (we only ever see the
// hash, never the plaintext).
type RecoveryCode struct {
	// Plain is the human form. Returned to the user exactly once at
	// enrollment; never persisted in plaintext.
	Plain string
	// Hash is the SHA-256 hex of the normalized plaintext.
	Hash string
}

// GenerateRecoveryCodes returns n fresh recovery codes. Plain is
// shown to the user; Hash is what the service layer persists.
//
// The alphabet deliberately excludes look-alikes (0/O, 1/I/L) so
// a transcription error on a scratch sheet still leaves 32^10 ≈
// 10^15 candidates per code, which is plenty at our scale.
func GenerateRecoveryCodes(n int) ([]RecoveryCode, error) {
	if n <= 0 {
		n = DefaultRecoveryCount
	}
	if n > 50 {
		return nil, fmt.Errorf("mfasp: too many recovery codes (%d)", n)
	}
	out := make([]RecoveryCode, 0, n)
	for i := 0; i < n; i++ {
		raw := make([]byte, RecoveryCodeLength)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("mfasp: read random: %w", err)
		}
		var b strings.Builder
		for _, byt := range raw {
			b.WriteByte(RecoveryAlphabet[int(byt)%len(RecoveryAlphabet)])
		}
		plain := b.String()
		out = append(out, RecoveryCode{
			Plain: formatRecoveryCode(plain),
			Hash:  HashRecoveryCode(plain),
		})
	}
	return out, nil
}

// formatRecoveryCode inserts a dash every 5 chars so a printed
// scratch sheet is easy to read. Purely cosmetic — callers must
// call NormaliseRecoveryCode before hashing / comparing.
func formatRecoveryCode(plain string) string {
	if len(plain) <= 5 {
		return plain
	}
	parts := make([]string, 0, (len(plain)+4)/5)
	for i := 0; i < len(plain); i += 5 {
		end := i + 5
		if end > len(plain) {
			end = len(plain)
		}
		parts = append(parts, plain[i:end])
	}
	return strings.Join(parts, recoveryCodeSeparator)
}

// NormaliseRecoveryCode strips the dashed formatting so the hash
// is consistent regardless of how the user entered the code.
func NormaliseRecoveryCode(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.ReplaceAll(s, recoveryCodeSeparator, "")
}

// HashRecoveryCode returns the hex SHA-256 of the normalised code.
// We never store the plaintext — a database leak cannot be replayed
// against the recovery endpoint.
func HashRecoveryCode(plain string) string {
	norm := NormaliseRecoveryCode(plain)
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// MatchRecoveryCode returns the index of the matching hash, or
// ErrInvalidRecovery when none match. The scan is constant-time
// per code (subtle.ConstantTimeCompare) so the response does not
// leak which character diverged first.
func MatchRecoveryCode(plain string, hashes []string) (int, error) {
	if len(hashes) == 0 {
		return -1, ErrInvalidRecovery
	}
	target := HashRecoveryCode(plain)
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(target), []byte(h)) == 1 {
			return i, nil
		}
	}
	return -1, ErrInvalidRecovery
}
