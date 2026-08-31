package weknora

import "time"

// User is the authenticated user profile returned by /auth/me and embedded
// in LoginResponse.
type User struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	AvatarURL   string   `json:"avatar_url"`
	TenantID    string   `json:"tenant_id"`
	Roles       []string `json:"roles"`
}

// KnowledgeBase is a tenant-scoped container for documents, databases, and
// collab docs.
type KnowledgeBase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // wiki / rag / hybrid
	ChunkCount  int       `json:"chunk_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KnowledgeBaseInput is the payload for POST /knowledge-bases.
type KnowledgeBaseInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// KnowledgeBasePatch is the payload for PATCH /knowledge-bases/:id.
type KnowledgeBasePatch struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// KnowledgeBasePage is the page envelope returned by GET /knowledge-bases.
type KnowledgeBasePage struct {
	Items         []KnowledgeBase `json:"items"`
	NextPageToken string          `json:"next_page_token"`
}

// SearchHit is one result returned by /knowledge-bases/:id/search.
type SearchHit struct {
	ChunkID       string   `json:"chunk_id"`
	Score         float64  `json:"score"`
	Text          string   `json:"text"`
	DocumentID    string   `json:"document_id"`
	DocumentTitle string   `json:"document_title"`
	Highlights    []string `json:"highlights"`
}

// SearchRequest is the payload for POST /knowledge-bases/:id/search.
type SearchRequest struct {
	Query  string         `json:"query"`
	TopK   int            `json:"top_k"`
	Rerank bool           `json:"rerank"`
	Filter map[string]any `json:"filter,omitempty"`
}

// SearchResponse is the response from /knowledge-bases/:id/search.
type SearchResponse struct {
	Hits []SearchHit `json:"hits"`
}

// Citation is one source cited by an ask or chat response.
type Citation struct {
	ChunkID       string  `json:"chunk_id"`
	DocumentTitle string  `json:"document_title"`
	Text          string  `json:"text"`
	Score         float64 `json:"score"`
}

// AskRequest is the payload for POST /knowledge-bases/:id/ask.
type AskRequest struct {
	Question       string `json:"question"`
	ConversationID string `json:"conversation_id,omitempty"`
	Stream         bool   `json:"stream,omitempty"`
}

// AskResponse is the response from POST /knowledge-bases/:id/ask.
type AskResponse struct {
	Answer         string     `json:"answer"`
	Citations      []Citation `json:"citations"`
	ConversationID string     `json:"conversation_id"`
}

// ChatMessage is a single message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"` // user / assistant / system
	Content string `json:"content"`
}

// ChatRequest is the payload for POST /knowledge-bases/:id/chat (SSE).
type ChatRequest struct {
	Messages       []ChatMessage `json:"messages"`
	ConversationID string        `json:"conversation_id,omitempty"`
}

// ChatChunkType enumerates the streaming event types emitted by /chat.
type ChatChunkType string

const (
	ChatChunkDelta    ChatChunkType = "delta"
	ChatChunkCitation ChatChunkType = "citation"
	ChatChunkDone     ChatChunkType = "done"
	ChatChunkError    ChatChunkType = "error"
)

// ChatChunk is one NDJSON event emitted by the streaming /chat endpoint.
type ChatChunk struct {
	Type     ChatChunkType `json:"type"`
	Content  string        `json:"content,omitempty"`
	Citation *Citation     `json:"citation,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// Conversation is the persistent record of a chat session.
type Conversation struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	KBID      string         `json:"kb_id"`
	Messages  []ChatMessage  `json:"messages"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ConversationPage is the page envelope for GET /conversations.
type ConversationPage struct {
	Items         []Conversation `json:"items"`
	NextPageToken string         `json:"next_page_token"`
}

// Database describes a multi-dim table schema.
type Database struct {
	ID      string             `json:"id"`
	KBID    string             `json:"kb_id"`
	Name    string             `json:"name"`
	Columns []DatabaseColumn   `json:"columns"`
	Views   []DatabaseView     `json:"views"`
}

// DatabaseColumn is one column definition.
type DatabaseColumn struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Options map[string]any `json:"options,omitempty"`
}

// DatabaseView is one saved view on a database.
type DatabaseView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// DatabaseInput is the payload for POST /knowledge-bases/:id/databases.
type DatabaseInput struct {
	Name    string                       `json:"name"`
	Columns []DatabaseInputColumn        `json:"columns"`
}

// DatabaseInputColumn is a column at creation time.
type DatabaseInputColumn struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Options map[string]any `json:"options,omitempty"`
}

// Row is one row in a database.
type Row struct {
	ID         string         `json:"id"`
	DatabaseID string         `json:"database_id"`
	Values     map[string]any `json:"values"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// RowInput is the payload for POST /.../rows.
type RowInput struct {
	Values map[string]any `json:"values"`
}

// FormulaEvalRequest is the payload for POST /knowledge-bases/:id/formula/eval.
type FormulaEvalRequest struct {
	Expression string         `json:"expression"`
	Context    map[string]any `json:"context,omitempty"`
}

// FormulaEvalResponse is the response from /formula/eval.
type FormulaEvalResponse struct {
	Value any    `json:"value"`
	Type  string `json:"type"`
	Error string `json:"error,omitempty"`
}

// AutomationTriggerType enumerates the four trigger kinds.
type AutomationTriggerType string

const (
	TriggerManual     AutomationTriggerType = "manual"
	TriggerScheduled  AutomationTriggerType = "scheduled"
	TriggerRowChanged AutomationTriggerType = "row_changed"
	TriggerWebhook    AutomationTriggerType = "webhook"
)

// AutomationActionType enumerates the five action kinds (Build #33).
type AutomationActionType string

const (
	ActionUpdateField AutomationActionType = "update_field"
	ActionCreateRow   AutomationActionType = "create_row"
	ActionSendWebhook AutomationActionType = "send_webhook"
	ActionRunAgent    AutomationActionType = "run_agent"
	ActionNotify      AutomationActionType = "notify"
)

// Automation is a full automation definition.
type Automation struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenant_id"`
	KBID          string                 `json:"kb_id"`
	DatabaseID    string                 `json:"database_id"`
	Name          string                 `json:"name"`
	TriggerType   AutomationTriggerType  `json:"trigger_type"`
	TriggerConfig map[string]any         `json:"trigger_config"`
	Steps         []AutomationStep       `json:"steps"`
	Enabled       bool                   `json:"enabled"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// AutomationStep is one node in the automation DAG.
type AutomationStep struct {
	ID         string               `json:"id"`
	ActionType AutomationActionType `json:"action_type"`
	Config     map[string]any       `json:"config"`
	NextIDs    []string             `json:"next_ids"`
}

// AutomationInput is the payload for POST /.../automations.
type AutomationInput struct {
	DatabaseID    string                 `json:"database_id"`
	Name          string                 `json:"name"`
	TriggerType   AutomationTriggerType  `json:"trigger_type"`
	TriggerConfig map[string]any         `json:"trigger_config,omitempty"`
	Steps         []AutomationStep       `json:"steps"`
	Enabled       bool                   `json:"enabled,omitempty"`
}

// AutomationRun is one execution of an automation.
type AutomationRun struct {
	ID           string         `json:"id"`
	AutomationID string         `json:"automation_id"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   time.Time      `json:"finished_at"`
	StepRuns     []StepRun      `json:"step_runs"`
}

// StepRun is one step's execution result within an AutomationRun.
type StepRun struct {
	StepID string         `json:"step_id"`
	Status string         `json:"status"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error"`
}

// CollabDoc is a collaborative document record (Yjs CRDT + binary).
type CollabDoc struct {
	ID             string    `json:"id"`
	KBID           string    `json:"kb_id"`
	Title          string    `json:"title"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CurrentVersion int       `json:"current_version"`
}

// CollabDocInput is the payload for POST /collaborative-docs.
type CollabDocInput struct {
	KBID  string `json:"kb_id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// CollabDocFile is the metadata returned by /collaborative-docs/:id/upload.
type CollabDocFile struct {
	DocID       string    `json:"doc_id"`
	Version     int       `json:"version"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int       `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// Agent is a Custom Agent Studio agent.
type Agent struct {
	ID           string   `json:"id"`
	TenantID     string   `json:"tenant_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model"`
	Tools        []string `json:"tools"`
	Memory       string   `json:"memory"`
	SystemPrompt string   `json:"system_prompt"`
}

// AgentInput is the payload for POST /agents.
type AgentInput struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model"`
	Tools        []string `json:"tools"`
	Memory       string   `json:"memory"`
	SystemPrompt string   `json:"system_prompt"`
}

// AgentRun is one execution of an agent.
type AgentRun struct {
	ID          string         `json:"id"`
	AgentID     string         `json:"agent_id"`
	Status      string         `json:"status"`
	TriggeredBy string         `json:"triggered_by"`
	Input       map[string]any `json:"input"`
	Output      map[string]any `json:"output"`
	StepsCount  int            `json:"steps_count"`
	TokensUsed  int            `json:"tokens_used"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  time.Time      `json:"finished_at"`
}

// AgentRunRequest is the payload for POST /agents/:id/runs.
type AgentRunRequest struct {
	Input map[string]any `json:"input"`
}

// Connector is an installed AI connector (M365, Google, Slack, ...).
type Connector struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	LastSyncAt time.Time      `json:"last_sync_at"`
	Config     map[string]any `json:"config"`
}

// ConnectorInput is the payload for POST /connectors.
type ConnectorInput struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

// VerificationRequest is the payload for POST /knowledge-bases/:id/verify.
type VerificationRequest struct {
	PageID     string `json:"page_id,omitempty"`
	IncludeKG  bool   `json:"include_kg"`
}

// VerificationReport is the response from /verify.
type VerificationReport struct {
	KBID       string             `json:"kb_id"`
	ScannedAt  time.Time          `json:"scanned_at"`
	TrustScore float64            `json:"trust_score"`
	Findings   []VerificationFinding `json:"findings"`
}

// VerificationFinding is one issue surfaced by AI Verification.
type VerificationFinding struct {
	PageID   string `json:"page_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}
