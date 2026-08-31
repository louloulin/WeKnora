package types

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// PolicyAction is the outcome a matched policy emits. The login
// flow maps each value to a concrete response:
//   allow         → mint the JWT as normal
//   deny          → 403, do not mint
//   require_mfa    → mint a short-lived challenge token, user must
//                   complete /api/v1/mfa/verify to upgrade to JWT
type PolicyAction string

const (
	PolicyActionAllow      PolicyAction = "allow"
	PolicyActionDeny       PolicyAction = "deny"
	PolicyActionRequireMFA PolicyAction = "require_mfa"
)

// PolicyConditions is the JSON shape persisted in the conditions
// column. Each field is optional — an empty list means "match all".
//
// Every condition is an ALLOW list (CIDR / country code / device
// posture / role name) — the policy matches when EVERY populated
// field accepts the request context. Day-of-week and hour-of-day
// describe a window during which the policy applies (UTC).
//
// JSON tag names are the on-wire schema; the pg driver persists the
// struct as JSONB via GORM's serializer. We keep the shape tiny so
// the audit log can be human readable.
type PolicyConditions struct {
	// IPCIDRs is a list of accepted CIDR ranges (IPv4 + IPv6).
	// Example: ["10.0.0.0/8", "192.168.1.0/24", "2001:db8::/32"].
	IPCIDRs []string `json:"ip_cidrs,omitempty"`

	// Countries is an ISO 3166-1 alpha-2 allow list of country codes.
	// Example: ["CN", "US", "JP"].
	Countries []string `json:"countries,omitempty"`

	// DevicePostures is an allow list of accepted device trust
	// labels (mdm-attested / managed / unmanaged / unknown).
	DevicePostures []string `json:"device_postures,omitempty"`

	// Roles is an allow list of accepted role names. A user with
	// any of these roles matches. Empty = match any role.
	Roles []string `json:"roles,omitempty"`

	// DaysOfWeek is a list of accepted day-of-week names (UTC, English
	// lowercase). Example: ["monday", "tuesday", "wednesday",
	// "thursday", "friday"]. Empty = any day.
	DaysOfWeek []string `json:"days_of_week,omitempty"`

	// HourRange is a half-open [start, end) hour-of-day window in UTC.
	// Both endpoints are 0-23. Empty = any hour.
	HourRange HourRange `json:"hour_range,omitempty"`
}

// HourRange expresses the [Start, End) UTC window during which a
// policy applies. Stored as a JSON sub-object on the parent.
type HourRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ConditionalAccessPolicy is one row in conditional_access_policies.
// All persistence goes through ConditionalAccessRepository; direct
// repo use is reserved for migration scripts.
type ConditionalAccessPolicy struct {
	ID          uint64 `gorm:"primaryKey"`
	TenantID    string `gorm:"type:varchar(36);not null;index"`
	Name        string `gorm:"type:varchar(128);not null"`
	Description string `gorm:"type:text"`
	// Conditions is stored as JSONB on pg / TEXT on sqlite via the
	// underlying driver. The GORM tag uses serializer:json so the
	// struct round-trips cleanly through both backends.
	Conditions   PolicyConditions `gorm:"type:jsonb;serializer:json"`
	Action       PolicyAction     `gorm:"type:varchar(32);not null"`
	Priority     int             `gorm:"not null;default:100"`
	Enabled      bool            `gorm:"not null;default:true"`
	CreatedBy    string          `gorm:"type:varchar(36);not null;default:''"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (ConditionalAccessPolicy) TableName() string { return "conditional_access_policies" }

// MarshalConditions is a defensive helper used by the service when
// it needs to render the policy as JSON for an audit log. It never
// returns nil — an unset PolicyConditions renders as "{}".
func (p *ConditionalAccessPolicy) MarshalConditions() ([]byte, error) {
	hourEmpty := p.Conditions.HourRange.Start == 0 && p.Conditions.HourRange.End == 0
	if len(p.Conditions.IPCIDRs) == 0 &&
		len(p.Conditions.Countries) == 0 &&
		len(p.Conditions.DevicePostures) == 0 &&
		len(p.Conditions.Roles) == 0 &&
		len(p.Conditions.DaysOfWeek) == 0 &&
		hourEmpty {
		return []byte("{}"), nil
	}
	return json.Marshal(p.Conditions)
}

// EvaluationRequest is the per-login context the evaluator walks
// against the tenant's enabled policies. Fields are populated by the
// handler from c.ClientIP / headers / DB lookups (role).
type EvaluationRequest struct {
	TenantID      string
	UserID        string
	UserRole      string
	ClientIP      string
	CountryCode   string
	DevicePosture string
	Now           time.Time // injected for testability
}

// Decision is the outcome of evaluating all enabled policies for a
// tenant. Reason is a human-readable string suitable for the audit
// log; MatchedPolicyID is non-zero when a rule fired.
type Decision struct {
	Action          PolicyAction `json:"action"`
	Reason          string       `json:"reason"`
	MatchedPolicyID uint64       `json:"matched_policy_id"`
}
