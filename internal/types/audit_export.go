package types

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AuditExportFormat enumerates the supported export encodings. JSON
// stays structured for downstream tooling; CSV is the human-readable
// default that an admin can open in Excel.
type AuditExportFormat string

const (
	AuditExportFormatCSV  AuditExportFormat = "csv"
	AuditExportFormatJSON AuditExportFormat = "json"
)

// AuditExportStatus tracks the lifecycle of an export request. The
// current implementation runs exports synchronously inside the
// request handler so the only realistic transitions are pending ->
// succeeded / failed. The enum is kept for parity with future async
// dispatch (asynq) and to make handler logic easier to extend.
type AuditExportStatus string

const (
	AuditExportStatusPending   AuditExportStatus = "pending"
	AuditExportStatusRunning   AuditExportStatus = "running"
	AuditExportStatusSucceeded AuditExportStatus = "succeeded"
	AuditExportStatusFailed    AuditExportStatus = "failed"
)

// AuditExportMaxRows caps the maximum number of rows a single export
// can include. Larger requests are rejected at the request layer to
// keep the in-memory payload bounded.
const AuditExportMaxRows = 100_000

// AuditExport is one row in the audit_exports table. The payload
// itself is generated on demand and never persisted — only the
// metadata (filter + row_count + byte_size + status) is stored, so
// the table stays small even with frequent exports.
type AuditExport struct {
	ID           string             `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID     uint64             `json:"tenant_id" gorm:"index"`
	RequestedBy  string             `json:"requested_by" gorm:"type:varchar(64);index"`
	Format       AuditExportFormat  `json:"format" gorm:"type:varchar(16)"`
	FilterJSON   string             `json:"filter_json" gorm:"type:text"` // JSON-encoded AuditExportFilter
	RowCount     int64              `json:"row_count"`
	ByteSize     int64              `json:"byte_size"`
	Status       AuditExportStatus  `json:"status" gorm:"type:varchar(16)"`
	Error        string             `json:"error,omitempty" gorm:"type:text"`
	CreatedAt    time.Time          `json:"created_at"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty"`
	ExpiresAt    *time.Time         `json:"expires_at,omitempty"`
}

// TableName returns the GORM table name.
func (AuditExport) TableName() string { return "audit_exports" }

// BeforeCreate fills CreatedAt if missing.
func (e *AuditExport) BeforeCreate(tx *gorm.DB) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}

// AuditExportFilter narrows the audit log slice that the export
// should include. All fields are optional; an empty filter exports
// the entire tenant audit trail.
type AuditExportFilter struct {
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	Action     string     `json:"action,omitempty"`
	Outcome    string     `json:"outcome,omitempty"`
	ActorID    string     `json:"actor_id,omitempty"`
	ScopeType  string     `json:"scope_type,omitempty"`
	ScopeID    string     `json:"scope_id,omitempty"`
}

// AuditExportCreateRequest is the body accepted by POST /audit/exports.
// Format defaults to CSV; the filter is optional.
type AuditExportCreateRequest struct {
	Format AuditExportFormat `json:"format"`
	Filter AuditExportFilter `json:"filter"`
	MaxRows int              `json:"max_rows,omitempty"`
}

// Normalize trims + defaults + clamps the request. Returns the typed
// sentinel ErrAuditExportBadInput when validation fails.
func (r *AuditExportCreateRequest) Normalize() error {
	r.Format = AuditExportFormat(strings.ToLower(strings.TrimSpace(string(r.Format))))
	if r.Format == "" {
		r.Format = AuditExportFormatCSV
	}
	if r.Format != AuditExportFormatCSV && r.Format != AuditExportFormatJSON {
		return ErrAuditExportBadInput
	}
	if r.Filter.StartTime != nil && r.Filter.EndTime != nil && r.Filter.EndTime.Before(*r.Filter.StartTime) {
		return ErrAuditExportBadInput
	}
	if r.Filter.Action != "" {
		r.Filter.Action = strings.TrimSpace(r.Filter.Action)
	}
	if r.Filter.ActorID != "" {
		r.Filter.ActorID = strings.TrimSpace(r.Filter.ActorID)
	}
	if r.Filter.ScopeType != "" {
		r.Filter.ScopeType = strings.TrimSpace(r.Filter.ScopeType)
	}
	if r.Filter.ScopeID != "" {
		r.Filter.ScopeID = strings.TrimSpace(r.Filter.ScopeID)
	}
	if r.MaxRows <= 0 {
		r.MaxRows = AuditExportMaxRows
	}
	if r.MaxRows > AuditExportMaxRows {
		r.MaxRows = AuditExportMaxRows
	}
	return nil
}

// AuditExportSummary is the aggregated counter returned by the
// compliance report endpoint. Mirrors the structure an auditor would
// expect: actions per category, denied events, top actors.
type AuditExportSummary struct {
	TenantID         uint64                  `json:"tenant_id"`
	WindowStart      time.Time               `json:"window_start"`
	WindowEnd        time.Time               `json:"window_end"`
	TotalEvents      int64                   `json:"total_events"`
	SuccessCount     int64                   `json:"success_count"`
	DeniedCount      int64                   `json:"denied_count"`
	FailedCount      int64                   `json:"failed_count"`
	UniqueActors     int                     `json:"unique_actors"`
	TopActions       []AuditActionTally      `json:"top_actions"`
	TopActors        []AuditActorTally       `json:"top_actors"`
	ComplianceStatus string                  `json:"compliance_status"` // "ok" / "review" / "violation"
}

// AuditActionTally is one row in the top-actions breakdown.
type AuditActionTally struct {
	Action AuditAction `json:"action"`
	Count  int64       `json:"count"`
}

// AuditActorTally is one row in the top-actors breakdown.
type AuditActorTally struct {
	ActorUserID string `json:"actor_user_id"`
	Count       int64  `json:"count"`
}

// ComplianceReportWindowDays is the default window for compliance
// summaries. 30 days aligns with monthly reporting cadence.
const ComplianceReportWindowDays = 30

// ErrAuditExportBadInput is the typed sentinel returned by Normalize.
// The handler maps errors.Is(err, ErrAuditExportBadInput) to 400.
var ErrAuditExportBadInput = errors.New("audit export: bad input")
