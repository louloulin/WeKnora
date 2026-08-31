// Package mfasp is the low-level Time-based One-Time Password (TOTP)
// + recovery-code primitives used by the WeKnora MFA service. It
// follows the same sp+rep split the SAML/LDAP/SCIM packages use:
// this package owns the cryptographic core, the higher layers
// (service + handler) own persistence + policy.
//
// Standards:
//   - RFC 6238 (TOTP: Time-Based One-Time Password Algorithm)
//   - RFC 4226 (HOTP: An HMAC-Based OTP Algorithm)
//   - Google Authenticator keyURI format (otpauth://totp/...)
//
// The package depends only on the standard library + crypto/subtle
// + encoding/base32; no external deps. Secrets are 20 random bytes
// (160-bit) base32-encoded without padding, the same shape Google
// Authenticator / Authy / 1Password / Okta Verify expect when
// scanning a QR code.
package mfasp
