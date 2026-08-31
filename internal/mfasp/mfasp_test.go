package mfasp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors. We use the SHA1 / 6-digit /
// period=30s path because that is the Google Authenticator default.
//
// Secret = "12345678901234567890" (ASCII bytes, NOT base32).
// We translate to base32 below.
const rfc6238SecretPlain = "12345678901234567890"

func init() {
	// Pre-compute the base32 encoding so the test fixture table is
	// readable. No side effects beyond that.
}

// base32 of the ASCII string "12345678901234567890" with no
// padding. Computed offline to keep the table below stable.
const rfc6238Base32Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func mustSecret(t *testing.T) Secret {
	return Secret(rfc6238Base32Secret)
}

// RFC 6238 Appendix B SHA1 vectors (period=30, digits=8 — but we
// only support 6; the truncated 6-digit suffix is what we test).
// We verify against the published T value and slice off the last 6
// digits from the 8-digit code to confirm our truncation.
var rfc6238Cases = []struct {
	t        int64  // unix seconds
	fullCode string // 8-digit published value
	want6    string // last 6 digits of the truncated HMAC
}{
	{59, "94287082", "287082"},
	{1111111109, "07081804", "081804"},
	{1111111111, "14050471", "050471"},
	{1234567890, "89005924", "005924"},
	{2000000000, "69279037", "279037"},
	{20000000000, "65353130", "353130"},
}

func TestGenerateSecretShape(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	// 20 bytes → 32 base32 chars without padding.
	if len(s) != 32 {
		t.Fatalf("len(secret) = %d, want 32", len(s))
	}
	for _, r := range s {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", r) {
			t.Fatalf("non-base32 char: %q in %q", r, s)
		}
	}
}

func TestGenerateSecretEntropy(t *testing.T) {
	a, _ := GenerateSecret()
	b, _ := GenerateSecret()
	if a == b {
		t.Fatalf("two consecutive secrets must differ")
	}
}

func TestCodeMatchesRFC6238Truncation(t *testing.T) {
	sec := mustSecret(t)
	for _, tc := range rfc6238Cases {
		got, err := sec.CodeAt(time.Unix(tc.t, 0), 30)
		if err != nil {
			t.Fatalf("CodeAt(%d): %v", tc.t, err)
		}
		if got != tc.want6 {
			t.Fatalf("t=%d: got %q want %q", tc.t, got, tc.want6)
		}
	}
}

func TestVerifyAcceptsValidCode(t *testing.T) {
	sec := mustSecret(t)
	t0 := time.Unix(1234567890, 0)
	code, _ := sec.CodeAt(t0, 30)
	ok, counter, err := sec.VerifyWithOptions(code, t0, VerifyOptions{Period: 30})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("valid code rejected")
	}
	if counter == 0 {
		t.Fatalf("counter should be non-zero")
	}
}

func TestVerifyRejectsWrongCode(t *testing.T) {
	sec := mustSecret(t)
	ok, _, err := sec.VerifyWithOptions("000000", time.Unix(1234567890, 0), VerifyOptions{Period: 30})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("wrong code accepted")
	}
}

func TestVerifyAllowsClockDrift(t *testing.T) {
	sec := mustSecret(t)
	// Code for t=1234567890; we are verifying at t=1234567890+45
	// which falls into the next period (skew=1 covers ±30s).
	t0 := time.Unix(1234567890, 0)
	t1 := t0.Add(45 * time.Second)
	code, _ := sec.CodeAt(t0, 30)
	ok, _, err := sec.VerifyWithOptions(code, t1, VerifyOptions{Period: 30, Skew: 1})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("drifted code rejected")
	}
}

func TestVerifyRejectsOutOfWindowCode(t *testing.T) {
	sec := mustSecret(t)
	t0 := time.Unix(1234567890, 0)
	// Code for t0, presented at t0+120s (two periods drift, well
	// beyond ±1 step).
	code, _ := sec.CodeAt(t0, 30)
	t1 := t0.Add(120 * time.Second)
	ok, _, err := sec.VerifyWithOptions(code, t1, VerifyOptions{Period: 30, Skew: 1})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("code outside drift window accepted")
	}
}

func TestVerifyNormalisesFormatting(t *testing.T) {
	sec := mustSecret(t)
	t0 := time.Unix(1234567890, 0)
	code, _ := sec.CodeAt(t0, 30)
	// Some authenticator apps insert a space or dash mid-code.
	spaced := code[:3] + " " + code[3:]
	ok, _, err := sec.Verify(spaced, t0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("normalised code rejected")
	}
}

func TestVerifyRejectsShortCode(t *testing.T) {
	sec := mustSecret(t)
	_, _, err := sec.Verify("12345", time.Now())
	if err == nil {
		t.Fatalf("expected error for 5-digit code")
	}
}

func TestVerifyRejectsNonDigitCode(t *testing.T) {
	sec := mustSecret(t)
	_, _, err := sec.Verify("abcdef", time.Now())
	if err == nil {
		t.Fatalf("expected error for non-digit code")
	}
}

func TestProvisioningURI(t *testing.T) {
	sec := mustSecret(t)
	uri := sec.ProvisioningURI("alice@corp.example.com", "WeKnora")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("uri prefix wrong: %q", uri)
	}
	for _, want := range []string{
		"secret=" + rfc6238Base32Secret,
		"issuer=WeKnora",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Fatalf("uri missing %q: %q", want, uri)
		}
	}
}

func TestGenerateRecoveryCodesShape(t *testing.T) {
	codes, err := GenerateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("want 10, got %d", len(codes))
	}
	for _, c := range codes {
		if !strings.Contains(c.Plain, "-") {
			t.Fatalf("plain code missing dash separator: %q", c.Plain)
		}
		if c.Hash == c.Plain {
			t.Fatalf("hash must not equal plain")
		}
		if len(c.Hash) != 64 {
			t.Fatalf("hash length = %d, want 64", len(c.Hash))
		}
	}
}

func TestRecoveryCodesUnique(t *testing.T) {
	codes, _ := GenerateRecoveryCodes(20)
	seen := map[string]struct{}{}
	for _, c := range codes {
		if _, dup := seen[c.Hash]; dup {
			t.Fatalf("duplicate recovery code hash")
		}
		seen[c.Hash] = struct{}{}
	}
}

func TestNormaliseRecoveryCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abcde-fghij", "abcdefghij"},
		{"ABCDE-FGHIJ", "abcdefghij"},
		{"  abcde-fghij  ", "abcdefghij"},
		{"abcde--fghij", "abcdefghij"},
	}
	for _, tc := range cases {
		if got := NormaliseRecoveryCode(tc.in); got != tc.want {
			t.Fatalf("NormaliseRecoveryCode(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestMatchRecoveryCode(t *testing.T) {
	codes, _ := GenerateRecoveryCodes(3)
	hashes := []string{codes[0].Hash, codes[1].Hash, codes[2].Hash}
	// Strip the dash before lookup.
	plain := strings.ReplaceAll(codes[1].Plain, "-", "")
	idx, err := MatchRecoveryCode(plain, hashes)
	if err != nil {
		t.Fatalf("MatchRecoveryCode: %v", err)
	}
	if idx != 1 {
		t.Fatalf("idx = %d, want 1", idx)
	}
}

func TestMatchRecoveryCodeRejectsUnknown(t *testing.T) {
	codes, _ := GenerateRecoveryCodes(2)
	hashes := []string{codes[0].Hash, codes[1].Hash}
	_, err := MatchRecoveryCode("zzzzzzzzzz", hashes)
	if err != ErrInvalidRecovery {
		t.Fatalf("expected ErrInvalidRecovery, got %v", err)
	}
}
