// Package dlp implements the v0.7.22 Data Loss Prevention scanner. The
// scanner combines (a) built-in regex patterns for high-confidence
// sensitive data (credit cards, government IDs, SSN, email, phone, IP),
// (b) tenant-defined custom regex, and (c) tenant-defined dictionaries.
//
// Files in this package:
//
//   builtin.go   — built-in regex patterns (this file)
//   scanner.go   — Scan() entry point + regex / dictionary evaluation
//   service.go   — tenant policy loading + violation logging
//
// Three sub-services compose the public DLPScanner surface, wired in
// one Provide() block (see internal/container/container.go).
package dlp

import "regexp"

// builtinPatterns maps the canonical built-in pattern name to its
// compiled regex. The match groups are positional: group 0 is the full
// match; the scanner uses the full match as the "matched_value".
//
// Sourced from public references:
//   - Visa / Mastercard / Amex BIN ranges (ISO/IEC 7812)
//   - China resident ID card (GB 11643-1999, 18-digit + checksum)
//   - US SSN (SSA format, no hyphens for OCR-resistant matching)
//   - RFC 5322 simplified email matcher
//   - E.164 international phone (with optional separators)
//   - IPv4 dotted quad
//
// Calibration: kept conservative to minimize false positives. Loosening
// any pattern (e.g. accepting all 16-digit numbers) would inflate recall
// at the cost of precision.
var builtinPatterns = map[string]*regexp.Regexp{
	// Credit cards: 13-19 digit numbers, optionally separated by space or dash.
	// Anchored to common BIN prefixes to reduce false positives. The
	// (?:\\d{4}[ -]?){3}\\d{1,4} tail matches the canonical 4-4-4-4
	// grouping used on receipts and OCR'd card numbers.
	"credit_card": regexp.MustCompile(
		`\b(?:4\d{3}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}|4\d{12}(?:\d{3})?|5[1-5]\d{2}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}|5[1-5]\d{14}|2[2-7]\d{2}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}|2[2-7]\d{13}|6(?:011|5\d{2})[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}|3[47]\d{2}[ -]?\d{6}[ -]?\d{5})\b`,
	),
	// China resident ID card (18 digits, last is checksum X or 0-9)
	"id_card_cn": regexp.MustCompile(
		`\b[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`,
	),
	// US Social Security Number (9-digit, with leading-digit guard).
	// RE2 doesn't support negative lookahead, so we conservatively match
	// 9-digit numbers that don't start with 000 / 666 / 9 — covers the
	// 90% case. Tests verify recall on a fixture corpus.
	"ssn_us": regexp.MustCompile(
		`\b(?:0[1-9]|[1-578]\d|6[0-57-9])\d{7}\b`,
	),
	// Email (RFC 5322 simplified)
	"email": regexp.MustCompile(
		`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`,
	),
	// China mobile phone (11 digits, starts with 1, second digit 3-9)
	"phone_cn": regexp.MustCompile(
		`\b1[3-9]\d{9}\b`,
	),
	// International phone (E.164, with optional + prefix and 7-15 digits)
	"phone_intl": regexp.MustCompile(
		`\+\d{1,3}[ \-]?\d{3,4}[ \-]?\d{3,4}[ \-]?\d{0,4}\b`,
	),
	// IPv4 dotted quad (0-255.0-255.0-255.0-255)
	"ip_addr": regexp.MustCompile(
		`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\b`,
	),
}

// BuiltinNames returns the canonical list of built-in pattern names.
// Order is stable — used in admin UI dropdowns and test fixtures.
func BuiltinNames() []string {
	return []string{
		"credit_card", "id_card_cn", "ssn_us",
		"email", "phone_cn", "phone_intl", "ip_addr",
	}
}

// IsBuiltinName reports whether name is one of the registered built-ins.
// Used by the service to validate pattern_value for builtin rules.
func IsBuiltinName(name string) bool {
	_, ok := builtinPatterns[name]
	return ok
}
