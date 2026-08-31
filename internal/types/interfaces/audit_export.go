package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// AuditExportRepository is the storage primitive for the
// audit_exports metadata table. The actual export payload is generated
// on demand — only the (filter, row_count, byte_size, status) tuple is
// persisted so the table stays compact.
type AuditExportRepository interface {
	// Create inserts a new export row.
	Create(ctx context.Context, export *typeAlias) error
	// GetByID returns a single export. Returns (nil, nil) when missing
	// — the service translates nil into ErrAuditExportNotFound.
	GetByID(ctx context.Context, tenantID uint64, id string) (*typeAlias, error)
	// ListByTenant returns the most recent exports, paginated.
	ListByTenant(ctx context.Context, tenantID uint64, limit int) ([]typeAlias, error)
	// UpdateStatus transitions an export to the supplied status +
	// optional error message + optional row_count + byte_size.
	UpdateStatus(ctx context.Context, id string, status types.AuditExportStatus, rowCount int64, byteSize int64, errMsg string) error
}

// typeAlias is an internal alias so the interface file does not pull
// the full types package; the real AuditExport type is referenced from
// the repository implementation directly via the types.AuditExport
// pointer.
type typeAlias = types.AuditExport

// AuditExportService is the business-logic facade for audit exports.
// It orchestrates the AuditLogRepository scan + AuditExportRepository
// metadata write + payload encoding (CSV / JSON).
type AuditExportService interface {
	// CreateAndRun validates the request, opens the audit log scan
	// under the supplied filter, encodes the payload, persists the
	// metadata row, and returns the populated AuditExport.
	CreateAndRun(ctx context.Context, tenantID uint64, requestedBy string, req types.AuditExportCreateRequest) (*types.AuditExport, []byte, error)

	// Get returns a previously-created export. Returns
	// ErrAuditExportNotFound when the row is missing.
	Get(ctx context.Context, tenantID uint64, id string) (*types.AuditExport, error)

	// List returns the most recent exports for a tenant.
	List(ctx context.Context, tenantID uint64, limit int) ([]types.AuditExport, error)

	// ComplianceSummary rolls up the audit_logs slice over the trailing
	// window and returns the top-action / top-actor breakdowns used by
	// the compliance dashboard.
	ComplianceSummary(ctx context.Context, tenantID uint64, windowDays int) (*types.AuditExportSummary, error)
}

// Ensure time is referenced so gofmt doesn't strip the import when
// AuditExportService is the only interface consumer.
var _ = time.Time{}
