package types

import "time"

// ConnectorKind enumerates the supported external sources. New
// connector implementations can be added in code without a schema
// change — the column is varchar(64).
type ConnectorKind string

const (
	ConnectorSlack      ConnectorKind = "slack"
	ConnectorEmail      ConnectorKind = "email"
	ConnectorWebhook    ConnectorKind = "webhook"
	ConnectorRSS        ConnectorKind = "rss"
	ConnectorConfluence ConnectorKind = "confluence"
	ConnectorNotion     ConnectorKind = "notion"
	ConnectorJira       ConnectorKind = "jira"
)

// AllConnectorKinds lists the registered connector implementations.
// Used by the admin UI to populate the kind selector.
var AllConnectorKinds = []ConnectorKind{
	ConnectorSlack,
	ConnectorEmail,
	ConnectorWebhook,
	ConnectorRSS,
	ConnectorConfluence,
	ConnectorNotion,
	ConnectorJira,
}

// IngestConnector is a tenant-scoped registration of one external
// source. The config column holds credentials + parameters in a
// kind-specific JSON shape — see each Connector implementation for
// the expected keys.
type IngestConnector struct {
	ID              uint64       `json:"id"`
	TenantID        string       `json:"tenant_id" gorm:"index"`
	Name            string       `json:"name" gorm:"type:varchar(255)"`
	Kind            ConnectorKind `json:"kind" gorm:"type:varchar(64)"`
	Config          string       `json:"config" gorm:"type:text"` // JSON-encoded
	KnowledgeBaseID string       `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	Enabled         bool         `json:"enabled"`
	LastSyncAt      *time.Time   `json:"last_sync_at,omitempty"`
	LastError       string       `json:"last_error" gorm:"type:text"`
	CreatedBy       string       `json:"created_by" gorm:"index"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// TableName tells GORM to use the ingest_connectors table.
func (IngestConnector) TableName() string { return "ingest_connectors" }

// IngestJobStatus enumerates the run lifecycle.
type IngestJobStatus string

const (
	IngestJobQueued    IngestJobStatus = "queued"
	IngestJobRunning   IngestJobStatus = "running"
	IngestJobSucceeded IngestJobStatus = "succeeded"
	IngestJobFailed    IngestJobStatus = "failed"
)

// IngestJob records one sync attempt against an IngestConnector.
// result_count is the number of messages / items ingested into KB
// during this run.
type IngestJob struct {
	ID          uint64         `json:"id"`
	TenantID    string         `json:"tenant_id" gorm:"index"`
	ConnectorID uint64         `json:"connector_id" gorm:"index"`
	Status      IngestJobStatus `json:"status" gorm:"type:varchar(32)"`
	ResultCount int            `json:"result_count"`
	Error       string         `json:"error" gorm:"type:text"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// TableName tells GORM to use the ingest_jobs table.
func (IngestJob) TableName() string { return "ingest_jobs" }

// ConnectorMessage is one piece of content returned by a Connector's
// Fetch call. The kind-specific Connector implementation builds
// these from its source data; the IngestService then turns each
// message into a Knowledge entry under the connector's target
// knowledge_base_id.
type ConnectorMessage struct {
	// ID is the upstream message id (e.g. Slack ts, email message-id).
	// Used for dedup — if a message with the same ID has already been
	// ingested for this connector, the run skips it.
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Author    string            `json:"author,omitempty"`
	URL       string            `json:"url,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
