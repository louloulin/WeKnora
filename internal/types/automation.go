package types

import (
	"encoding/json"
	"time"
)

// AutomationTriggerType enumerates the supported trigger kinds. The
// engine fires runs in response to these.
type AutomationTriggerType string

const (
	AutomationTriggerManual      AutomationTriggerType = "manual"       // user clicks button
	AutomationTriggerFieldChange AutomationTriggerType = "field_change" // row column updated
	AutomationTriggerScheduled   AutomationTriggerType = "scheduled"    // cron
	AutomationTriggerWebhook     AutomationTriggerType = "webhook"      // inbound HTTP
)

// AllAutomationTriggerTypes lists the registered trigger types.
var AllAutomationTriggerTypes = []AutomationTriggerType{
	AutomationTriggerManual,
	AutomationTriggerFieldChange,
	AutomationTriggerScheduled,
	AutomationTriggerWebhook,
}

// AutomationActionType enumerates the supported action kinds. Each
// step in an automation DAG references one of these.
type AutomationActionType string

const (
	AutomationActionUpdateField AutomationActionType = "update_field" // write to row column
	AutomationActionCreateRow   AutomationActionType = "create_row"   // append new row
	AutomationActionSendWebhook AutomationActionType = "send_webhook" // outbound HTTP POST
	AutomationActionRunAgent    AutomationActionType = "run_agent"    // invoke custom agent
	AutomationActionNotify      AutomationActionType = "notify"       // user mention / email
)

// AllAutomationActionTypes lists the registered action types.
var AllAutomationActionTypes = []AutomationActionType{
	AutomationActionUpdateField,
	AutomationActionCreateRow,
	AutomationActionSendWebhook,
	AutomationActionRunAgent,
	AutomationActionNotify,
}

// AutomationRunStatus is the lifecycle of an automation run.
type AutomationRunStatus string

const (
	AutomationRunPending   AutomationRunStatus = "pending"
	AutomationRunRunning   AutomationRunStatus = "running"
	AutomationRunSucceeded AutomationRunStatus = "succeeded"
	AutomationRunFailed    AutomationRunStatus = "failed"
	AutomationRunCancelled AutomationRunStatus = "cancelled"
)

// Automation is a database-scoped rule: a trigger plus a DAG of
// steps that execute when the trigger fires.
type Automation struct {
	ID            string                 `json:"id" gorm:"primaryKey;type:varchar(64)"`
	TenantID      uint64                 `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string               `json:"kb_id" gorm:"index;type:varchar(64)"`
	DatabaseID    string                 `json:"database_id" gorm:"index;type:varchar(64)"`
	Name          string                 `json:"name" gorm:"type:varchar(255)"`
	Description   string                 `json:"description" gorm:"type:text"`
	TriggerType   AutomationTriggerType  `json:"trigger_type" gorm:"type:varchar(32);index"`
	TriggerConfig JSON                   `json:"trigger_config" gorm:"type:json"`
	Enabled       bool                   `json:"enabled" gorm:"default:true;index"`
	Steps         []AutomationStep       `json:"steps" gorm:"serializer:json;type:json"`
	CreatedBy     uint64                 `json:"created_by"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	LastFiredAt   *time.Time             `json:"last_fired_at,omitempty"`
	LastFireStatus AutomationRunStatus   `json:"last_fire_status,omitempty"`
}

// AutomationStep is one node in an automation DAG. The NextIDs slice
// gives downstream steps; cycles are rejected at save time.
type AutomationStep struct {
	ID         string               `json:"id" gorm:"primaryKey;type:varchar(64)"`
	Name       string               `json:"name" gorm:"type:varchar(255)"`
	ActionType AutomationActionType `json:"action_type" gorm:"type:varchar(32)"`
	Config     JSON                 `json:"config" gorm:"type:json"`
	NextIDs    []string             `json:"next_ids" gorm:"serializer:json;type:json"`
	// Retries is the maximum number of times to retry this step on
	// failure. 0 means no retries (just record the failure).
	Retries int `json:"retries"`
}

// AutomationRun records one execution of an automation. The
// per-step results are stored inline so an operator can inspect
// exactly which step failed.
type AutomationRun struct {
	ID          string                 `json:"id" gorm:"primaryKey;type:varchar(64)"`
	TenantID    uint64                 `json:"tenant_id" gorm:"index"`
	AutomationID string                `json:"automation_id" gorm:"index;type:varchar(64)"`
	Trigger     AutomationTriggerType  `json:"trigger" gorm:"type:varchar(32)"`
	Status      AutomationRunStatus    `json:"status" gorm:"type:varchar(32);index"`
	StartedAt   time.Time              `json:"started_at"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	StepRuns    []AutomationStepRun    `json:"step_runs" gorm:"serializer:json;type:json"`
	Error       string                 `json:"error" gorm:"type:text"`
	RetriedCount int                   `json:"retried_count"`
}

// AutomationStepRun records the outcome of one step in a run.
type AutomationStepRun struct {
	StepID    string          `json:"step_id"`
	Status    AutomationRunStatus `json:"status"`
	StartedAt time.Time       `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Error     string          `json:"error"`
	Output    JSON            `json:"output" gorm:"type:json"`
}

// TriggerConfigFieldChange is the parsed trigger_config for
// AutomationTriggerFieldChange. Either ColumnID or ColumnName must
// be set; ColumnID takes precedence.
type TriggerConfigFieldChange struct {
	ColumnID   string `json:"column_id,omitempty"`
	ColumnName string `json:"column_name,omitempty"`
	// OnlyOnInsert / OnlyOnUpdate restrict when the trigger fires.
	OnlyOnInsert bool `json:"only_on_insert,omitempty"`
	OnlyOnUpdate bool `json:"only_on_update,omitempty"`
}

// TriggerConfigScheduled parses the trigger_config for
// AutomationTriggerScheduled. Cron follows the 5-field standard.
type TriggerConfigScheduled struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone,omitempty"`
}

// TriggerConfigWebhook parses the trigger_config for
// AutomationTriggerWebhook. A token is generated on Create so the
// caller can hand out the URL /api/v1/automations/:id/webhook?token=
type TriggerConfigWebhook struct {
	Token       string `json:"token,omitempty"`
	Description string `json:"description,omitempty"`
}

// AutomationRunInputs is the data passed into Execute(). For
// AutomationTriggerFieldChange it includes the changed column name
// and old/new value.
type AutomationRunInputs struct {
	TenantID     uint64                 `json:"tenant_id"`
	DatabaseID   string                 `json:"database_id"`
	RowID        string                 `json:"row_id,omitempty"`
	UserID       uint64                 `json:"user_id,omitempty"`
	ChangedColumn string                `json:"changed_column,omitempty"`
	OldValue     any                    `json:"old_value,omitempty"`
	NewValue     any                    `json:"new_value,omitempty"`
	ManualPayload map[string]any        `json:"manual_payload,omitempty"`
}

// ParseTriggerConfigFieldChange unmarshals trigger_config into the
// typed shape. Returns a zero value when trigger_config is empty.
func ParseTriggerConfigFieldChange(raw JSON) (TriggerConfigFieldChange, error) {
	var c TriggerConfigFieldChange
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

// ParseTriggerConfigScheduled unmarshals trigger_config into the
// typed shape.
func ParseTriggerConfigScheduled(raw JSON) (TriggerConfigScheduled, error) {
	var c TriggerConfigScheduled
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

// ParseTriggerConfigWebhook unmarshals trigger_config into the
// typed shape.
func ParseTriggerConfigWebhook(raw JSON) (TriggerConfigWebhook, error) {
	var c TriggerConfigWebhook
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}
