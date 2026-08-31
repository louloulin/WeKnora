package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// AgentTrigger is a scheduled / event / webhook hook that fires a custom
// agent. The (tenant_id, agent_id, name) triple is the natural key.
type AgentTrigger struct {
	ID             uint64    `json:"id"`
	TenantID       uint64    `json:"tenant_id" gorm:"index"`
	AgentID        string    `json:"agent_id" gorm:"type:varchar(36);index"`
	TriggerType    string    `json:"trigger_type" gorm:"type:varchar(32)"`
	Name           string    `json:"name" gorm:"type:varchar(128)"`
	TriggerConfig  string    `json:"trigger_config" gorm:"type:text"`
	PayloadTemplate string   `json:"payload_template" gorm:"type:text"`
	Status         string    `json:"status" gorm:"type:varchar(32);default:'active'"`
	LastFiredAt    *time.Time `json:"last_fired_at,omitempty"`
	LastFireStatus string    `json:"last_fire_status,omitempty" gorm:"type:varchar(32)"`
	NextFireAt     *time.Time `json:"next_fire_at,omitempty"`
	CreatedBy      uint64    `json:"created_by" gorm:"index"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName tells GORM to use the agent_triggers table for this model.
func (AgentTrigger) TableName() string { return "agent_triggers" }

// AgentRun captures a single execution of a custom agent. Append-only —
// updates only change status / output / counters, never the inputs.
type AgentRun struct {
	ID             uint64     `json:"id"`
	TenantID       uint64     `json:"tenant_id" gorm:"index"`
	AgentID        string     `json:"agent_id" gorm:"type:varchar(36);index"`
	TriggerID      *uint64    `json:"trigger_id,omitempty" gorm:"index"`
	TriggeredBy    string     `json:"triggered_by" gorm:"type:varchar(32)"`
	TriggeredUser  *uint64    `json:"triggered_user,omitempty"`
	Status         string     `json:"status" gorm:"type:varchar(32);default:'queued'"`
	InputPayload   string     `json:"input_payload" gorm:"type:text"`
	OutputPayload  string     `json:"output_payload" gorm:"type:text"`
	ErrorMessage   string     `json:"error_message" gorm:"type:text"`
	StepsCount     int        `json:"steps_count"`
	TokensUsed     int64      `json:"tokens_used"`
	CostMicros     int64      `json:"cost_micros"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMs     int        `json:"duration_ms"`
	CreatedAt      time.Time  `json:"created_at"`
}

// TableName tells GORM to use the agent_runs table for this model.
func (AgentRun) TableName() string { return "agent_runs" }

// AgentCredential stores encrypted third-party credentials for tool calls.
// Ciphertext is AES-256-GCM with the nonce prepended; auth tag is stored
// separately for cross-driver verification.
type AgentCredential struct {
	ID             uint64     `json:"id"`
	TenantID       uint64     `json:"tenant_id" gorm:"index"`
	Name           string     `json:"name" gorm:"type:varchar(128)"`
	CredentialType string     `json:"credential_type" gorm:"type:varchar(32)"`
	// Ciphertext + nonce + auth_tag are exposed only via the Vault service,
	// never over the wire. JSON marshal ignores them.
	Ciphertext []byte `json:"-"`
	Nonce      []byte `json:"-"`
	AuthTag    []byte `json:"-"`
	EncMeta    string `json:"enc_meta" gorm:"type:text"`
	CreatedBy  uint64 `json:"created_by" gorm:"index"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName tells GORM to use the agent_credentials table for this model.
func (AgentCredential) TableName() string { return "agent_credentials" }

// AgentCreditLedgerEntry is an append-only ledger row tracking quota
// consumption. balance_after is the running balance after this entry.
type AgentCreditLedgerEntry struct {
	ID            uint64    `json:"id"`
	TenantID      uint64    `json:"tenant_id" gorm:"index"`
	AgentID       string    `json:"agent_id" gorm:"type:varchar(36);index"`
	RunID         *uint64   `json:"run_id,omitempty" gorm:"index"`
	Operation     string    `json:"operation" gorm:"type:varchar(32)"`
	Unit          string    `json:"unit" gorm:"type:varchar(32)"`
	Quantity      int64     `json:"quantity"`
	BalanceAfter  int64     `json:"balance_after"`
	PolicyVersion int64     `json:"policy_version"`
	Notes         string    `json:"notes" gorm:"type:text"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName tells GORM to use the agent_credit_ledger table for this model.
func (AgentCreditLedgerEntry) TableName() string { return "agent_credit_ledger" }

// AgentQuotaPolicy is a versioned quota policy. Each edit creates a new
// version (old versions remain immutable for audit).
type AgentQuotaPolicy struct {
	ID                     uint64    `json:"id"`
	TenantID               uint64    `json:"tenant_id" gorm:"index"`
	Name                   string    `json:"name" gorm:"type:varchar(128)"`
	Version                int64     `json:"version"`
	MonthlyTokens          int64     `json:"monthly_tokens"`
	DailyInvocations       int64     `json:"daily_invocations"`
	PerRunCostCapMicros    int64     `json:"per_run_cost_cap_micros"`
	PerAgentConcurrency    int       `json:"per_agent_concurrency"`
	IsActive               bool      `json:"is_active"`
	CreatedBy              uint64    `json:"created_by" gorm:"index"`
	CreatedAt              time.Time `json:"created_at"`
}

// TableName tells GORM to use the agent_quota_policies table for this model.
func (AgentQuotaPolicy) TableName() string { return "agent_quota_policies" }

// AgentStudio constants — trigger / status enums centralized.
const (
	AgentTriggerTypeManual  = "manual"
	AgentTriggerTypeCron    = "cron"
	AgentTriggerTypeEvent   = "event"
	AgentTriggerTypeWebhook = "webhook"

	AgentTriggerStatusActive   = "active"
	AgentTriggerStatusPaused   = "paused"
	AgentTriggerStatusArchived = "archived"

	AgentRunStatusQueued    = "queued"
	AgentRunStatusRunning   = "running"
	AgentRunStatusSucceeded = "succeeded"
	AgentRunStatusFailed    = "failed"
	AgentRunStatusTimeout   = "timeout"
	AgentRunStatusCancelled = "cancelled"

	AgentCredTypeAPIKey = "api_key"
	AgentCredTypeOAuth2 = "oauth2"
	AgentCredTypeBasic  = "basic"
	AgentCredTypeBearer = "bearer"
	AgentCredTypeCustom = "custom"

	AgentLedgerOpCharge = "charge"
	AgentLedgerOpRefund = "refund"
	AgentLedgerOpGrant  = "grant"
	AgentLedgerOpExpire = "expire"
	AgentLedgerOpAdjust = "adjust"

	AgentUnitTokens       = "tokens"
	AgentUnitInvocations  = "invocations"
	AgentUnitCostMicros   = "cost_micros"
)

// AgentStudioParseTriggerConfig parses the JSON-encoded trigger_config
// string into a generic map. The runtime decides how to interpret keys
// (cron expr / event filter / webhook path) based on trigger_type.
func AgentStudioParseTriggerConfig(raw string) (map[string]any, error) {
	out := map[string]any{}
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("agent_studio: invalid trigger_config: %w", err)
	}
	return out, nil
}

// AgentStudioStringifyTriggerConfig serializes a map back into the
// JSON-encoded form expected by the trigger_config TEXT column.
func AgentStudioStringifyTriggerConfig(cfg map[string]any) (string, error) {
	if cfg == nil {
		return "{}", nil
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
