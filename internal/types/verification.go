package types

import (
	"time"
)

// VerificationStatus is the outcome of running a verification pass on a
// wiki page. It maps to the UI badge color: ok (green), warning
// (amber), bad (red). Pages that no longer exist or are archived
// short-circuit to "missing".
type VerificationStatus string

const (
	// VerificationStatusOK means the page passed all checks. The
	// scanner emits OK only when freshness, contradiction, link health,
	// and trust score all return within their thresholds.
	VerificationStatusOK VerificationStatus = "ok"

	// VerificationStatusWarning means at least one check produced a
	// non-blocking concern — e.g. freshness > 180 days, or a single
	// broken outbound link. The UI surfaces an amber chip and lets the
	// owner decide whether to act.
	VerificationStatusWarning VerificationStatus = "warning"

	// VerificationStatusBad means at least one hard rule failed —
	// contradiction detected, multiple broken links, or trust score
	// below 0.4. The UI surfaces a red chip and emails the owner.
	VerificationStatusBad VerificationStatus = "bad"

	// VerificationStatusMissing means the page is gone. The scanner
	// keeps a tombstone row so historical reports stay explainable.
	VerificationStatusMissing VerificationStatus = "missing"
)

// VerificationCheck is one row in the verification report. Severity
// drives the chip colour; SuggestedAction is what the UI surfaces in
// the "Fix it" menu (re-index, archive, merge, etc).
type VerificationCheck struct {
	Code            string            `json:"code"`            // freshness | contradiction | link_health | trust_score
	Severity        VerificationStatus `json:"severity"`        // ok | warning | bad
	Message         string            `json:"message"`         // human-readable
	SuggestedAction string            `json:"suggested_action"` // re_index | archive | merge | review | none
	Details         map[string]string  `json:"details,omitempty"`
}

// VerificationReport is the per-page verification output. Pages that
// have not been scanned since the freshness window collapse to a
// single "stale: not_scanned_since" check rather than fabricating
// numbers.
type VerificationReport struct {
	PageID          string             `json:"page_id"`
	Slug            string             `json:"slug"`
	KnowledgeBaseID string             `json:"knowledge_base_id"`
	TenantID        string             `json:"tenant_id"`
	Status          VerificationStatus `json:"status"`
	TrustScore      float64            `json:"trust_score"`
	Checks          []VerificationCheck `json:"checks"`
	ScannedAt       time.Time           `json:"scanned_at"`
}

// VerificationSummary is the per-KB rollup. Status counts are emitted
// so the panel can render a single status chip per KB without
// iterating every page.
type VerificationSummary struct {
	KnowledgeBaseID string             `json:"knowledge_base_id"`
	Total           int                `json:"total"`
	OK              int                `json:"ok"`
	Warning         int                `json:"warning"`
	Bad             int                `json:"bad"`
	Missing         int                `json:"missing"`
	AvgTrustScore   float64            `json:"avg_trust_score"`
	ScannedAt       time.Time          `json:"scanned_at"`
}
