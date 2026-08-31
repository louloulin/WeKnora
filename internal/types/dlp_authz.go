package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// DLPPolicy is a versioned data-loss-prevention policy. Each edit creates
// a new version row (old versions remain immutable for audit).
type DLPPolicy struct {
	ID            uint64    `json:"id"`
	TenantID      uint64    `json:"tenant_id" gorm:"index"`
	Name          string    `json:"name" gorm:"type:varchar(128)"`
	Version       int64     `json:"version"`
	ResourceScope string    `json:"resource_scope" gorm:"type:varchar(64);default:'*'"`
	Severity      string    `json:"severity" gorm:"type:varchar(32);default:'medium'"`
	Action        string    `json:"action" gorm:"type:varchar(32);default:'log'"`
	IsActive      bool      `json:"is_active"`
	Description   string    `json:"description" gorm:"type:text"`
	CreatedBy     uint64    `json:"created_by" gorm:"index"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName tells GORM to use the dlp_policies table for this model.
func (DLPPolicy) TableName() string { return "dlp_policies" }

// DLPRule is one regex / dictionary / builtin entry inside a policy.
// A policy typically has many rules; all fire during a scan.
type DLPRule struct {
	ID            uint64    `json:"id"`
	PolicyID      uint64    `json:"policy_id" gorm:"index"`
	TenantID      uint64    `json:"tenant_id" gorm:"index"`
	PatternType   string    `json:"pattern_type" gorm:"type:varchar(32)"`
	PatternValue  string    `json:"pattern_value" gorm:"type:text"`
	Severity      string    `json:"severity" gorm:"type:varchar(32);default:'medium'"`
	Enabled       bool      `json:"enabled"`
	Description   string    `json:"description" gorm:"type:text"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName tells GORM to use the dlp_rules table for this model.
func (DLPRule) TableName() string { return "dlp_rules" }

// DLPViolation is an append-only scan result. Queryable for SOC 2 /
// GDPR audits.
type DLPViolation struct {
	ID             uint64    `json:"id"`
	TenantID       uint64    `json:"tenant_id" gorm:"index"`
	PolicyID       uint64    `json:"policy_id" gorm:"index"`
	RuleID         *uint64   `json:"rule_id,omitempty" gorm:"index"`
	Resource       string    `json:"resource" gorm:"type:varchar(64)"`
	ResourceID     string    `json:"resource_id" gorm:"type:varchar(36)"`
	ActorID        uint64    `json:"actor_id" gorm:"index"`
	MatchedValue   string    `json:"matched_value" gorm:"type:text"`
	Context        string    `json:"context" gorm:"type:text"`
	MatchedPattern string    `json:"matched_pattern" gorm:"type:varchar(128)"`
	ActionTaken    string    `json:"action_taken" gorm:"type:varchar(32)"`
	Severity       string    `json:"severity" gorm:"type:varchar(32)"`
	AuditLogID     *uint64   `json:"audit_log_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// TableName tells GORM to use the dlp_violations table for this model.
func (DLPViolation) TableName() string { return "dlp_violations" }

// AuthZPolicyVersion is an immutable versioned AuthZ policy entry. The
// (tenant, policy_key) family has many rows; the highest-version row is
// the live policy.
type AuthZPolicyVersion struct {
	ID         uint64    `json:"id"`
	TenantID   uint64    `json:"tenant_id" gorm:"index"`
	PolicyKey  string    `json:"policy_key" gorm:"type:varchar(128);index"`
	Version    int64     `json:"version"`
	Expression string    `json:"expression" gorm:"type:text"`
	Decision   string    `json:"decision" gorm:"type:varchar(32)"`
	Metadata   string    `json:"metadata" gorm:"type:text"`
	CreatedBy  uint64    `json:"created_by" gorm:"index"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName tells GORM to use the authz_policy_versions table for this model.
func (AuthZPolicyVersion) TableName() string { return "authz_policy_versions" }

// DLP constants — pattern types, severities, actions.
const (
	DLPPatternRegex     = "regex"
	DLPPatternDictionary = "dictionary"
	DLPPatternBuiltin   = "builtin"

	// Builtin pattern names. Keep this list in sync with
	// internal/application/service/dlp/builtin.go.
	DLPBuiltinCreditCard = "credit_card"
	DLPBuiltinIDCardCN   = "id_card_cn"
	DLPBuiltinSSNUS      = "ssn_us"
	DLPBuiltinEmail      = "email"
	DLPBuiltinPhoneCN    = "phone_cn"
	DLPBuiltinPhoneIntl  = "phone_intl"
	DLPBuiltinIPAddr     = "ip_addr"

	DLPSeverityLow      = "low"
	DLPSeverityMedium   = "medium"
	DLPSeverityHigh     = "high"
	DLPSeverityCritical = "critical"

	DLPActionLog      = "log"
	DLPActionBlock    = "block"
	DLPActionRedact   = "redact"
	DLPActionNotifyDPO = "notify_dpo"
)

// AuthZ constants — decisions.
const (
	AuthZDecisionAllow       = "allow"
	AuthZDecisionDeny        = "deny"
	AuthZDecisionConditional = "conditional"
)

// DLPValidSeverity reports whether s is a recognised severity.
func DLPValidSeverity(s string) bool {
	switch s {
	case DLPSeverityLow, DLPSeverityMedium, DLPSeverityHigh, DLPSeverityCritical:
		return true
	}
	return false
}

// DLPValidAction reports whether s is a recognised action.
func DLPValidAction(s string) bool {
	switch s {
	case DLPActionLog, DLPActionBlock, DLPActionRedact, DLPActionNotifyDPO:
		return true
	}
	return false
}

// DLPValidPatternType reports whether s is a recognised pattern type.
func DLPValidPatternType(s string) bool {
	switch s {
	case DLPPatternRegex, DLPPatternDictionary, DLPPatternBuiltin:
		return true
	}
	return false
}

// AuthZValidDecision reports whether s is a recognised decision.
func AuthZValidDecision(s string) bool {
	switch s {
	case AuthZDecisionAllow, AuthZDecisionDeny, AuthZDecisionConditional:
		return true
	}
	return false
}

// ParseMetadata deserialises the authz_policy_versions.metadata column
// into a free-form map. Used by the admin UI to surface author / commit /
// ticket info.
func (a *AuthZPolicyVersion) ParseMetadata() (map[string]any, error) {
	out := map[string]any{}
	if a.Metadata == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(a.Metadata), &out); err != nil {
		return nil, fmt.Errorf("authz.metadata: %w", err)
	}
	return out, nil
}
