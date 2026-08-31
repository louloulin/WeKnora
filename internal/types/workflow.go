package types

import (
	"encoding/json"
	"time"
)

// WorkflowNodeType enumerates the node kinds supported by the AI Workflow
// Builder (Build #37). It is a superset of the Build #33 Automation action
// kinds and adds form + LLM nodes.
type WorkflowNodeType string

const (
	// Triggers — the entry point of a workflow.
	WorkflowTriggerManual     WorkflowNodeType = "manual_trigger"
	WorkflowTriggerScheduled  WorkflowNodeType = "scheduled_trigger"
	WorkflowTriggerEvent      WorkflowNodeType = "event_trigger"
	WorkflowTriggerWebhook    WorkflowNodeType = "webhook_trigger"
	WorkflowTriggerFormSubmit WorkflowNodeType = "form_submit_trigger"

	// Action / body nodes.
	WorkflowFormInput WorkflowNodeType = "form_input"
	WorkflowAIAgent   WorkflowNodeType = "ai_agent"
	WorkflowAILLM     WorkflowNodeType = "ai_llm"
	WorkflowAutomation WorkflowNodeType = "automation_call"
	WorkflowSendWebhook WorkflowNodeType = "send_webhook"
	WorkflowNotify    WorkflowNodeType = "notify"

	// Terminal / output.
	WorkflowReturn WorkflowNodeType = "return"
)

// Workflow is a multi-node DAG combining triggers, AI calls, form
// inputs, and automation. The schema is intentionally flexible — each
// node carries an opaque Config blob whose shape depends on NodeType.
type Workflow struct {
	ID         string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID   uint64          `json:"tenant_id" gorm:"index;type:varchar(36)"`
	KBID       string          `json:"kb_id" gorm:"index;type:varchar(36)"`
	Name       string          `json:"name" gorm:"type:varchar(256)"`
	Version    int             `json:"version"`
	Enabled    bool            `json:"enabled"`
	Nodes      []WorkflowNode  `json:"nodes" gorm:"-"`
	Edges      []WorkflowEdge  `json:"edges" gorm:"-"`
	NodeBlob   json.RawMessage `json:"-" gorm:"type:jsonb"` // persisted nodes
	EdgeBlob   json.RawMessage `json:"-" gorm:"type:jsonb"` // persisted edges
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// TableName tells GORM to use workflows table.
func (Workflow) TableName() string { return "workflows" }

// WorkflowNode is one node in the workflow DAG.
type WorkflowNode struct {
	ID         string          `json:"id"`
	Type       WorkflowNodeType `json:"type"`
	Config     json.RawMessage `json:"config,omitempty"`
	PositionX  int             `json:"position_x"` // for the visual editor
	PositionY  int             `json:"position_y"`
}

// WorkflowEdge is a directed connection between two nodes.
type WorkflowEdge struct {
	ID        string `json:"id"`
	SrcNodeID string `json:"src_node_id"`
	DstNodeID string `json:"dst_node_id"`
	// Condition is a tiny DSL expression evaluated against the previous
	// node's output. Empty string means "always take this edge".
	Condition string `json:"condition,omitempty"`
}

// WorkflowInput is the payload for POST /knowledgebase/:kb_id/workflows.
type WorkflowInput struct {
	Name    string         `json:"name"`
	Nodes   []WorkflowNode `json:"nodes"`
	Edges   []WorkflowEdge `json:"edges"`
	Enabled bool           `json:"enabled"`
}

// WorkflowRun is one execution of a workflow.
type WorkflowRun struct {
	ID         string             `json:"id" gorm:"primaryKey;type:varchar(36)"`
	WorkflowID string             `json:"workflow_id" gorm:"index;type:varchar(36)"`
	TenantID   uint64             `json:"tenant_id" gorm:"index;type:varchar(36)"`
	Status     string             `json:"status" gorm:"type:varchar(32)"`
	TriggeredBy string           `json:"triggered_by" gorm:"type:varchar(64)"`
	Input      json.RawMessage    `json:"input" gorm:"type:jsonb"`
	Output     json.RawMessage    `json:"output" gorm:"type:jsonb"`
	Error      string             `json:"error" gorm:"type:text"`
	StartedAt  *time.Time         `json:"started_at,omitempty"`
	FinishedAt *time.Time         `json:"finished_at,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	NodeRuns   []WorkflowNodeRun  `json:"node_runs" gorm:"-"`
}

// TableName tells GORM to use workflow_runs table.
func (WorkflowRun) TableName() string { return "workflow_runs" }

// WorkflowNodeRun is the per-node execution record within a workflow run.
type WorkflowNodeRun struct {
	ID         string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	RunID      string          `json:"run_id" gorm:"index;type:varchar(36)"`
	NodeID     string          `json:"node_id" gorm:"index;type:varchar(36)"`
	Status     string          `json:"status" gorm:"type:varchar(32)"`
	Input      json.RawMessage `json:"input" gorm:"type:jsonb"`
	Output     json.RawMessage `json:"output" gorm:"type:jsonb"`
	Error      string          `json:"error" gorm:"type:text"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// TableName tells GORM to use workflow_node_runs table.
func (WorkflowNodeRun) TableName() string { return "workflow_node_runs" }

// WorkflowStatus enumerates the run lifecycle states.
type WorkflowStatus string

const (
	WorkflowStatusQueued    WorkflowStatus = "queued"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusSucceeded WorkflowStatus = "succeeded"
	WorkflowStatusFailed    WorkflowStatus = "failed"
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)
