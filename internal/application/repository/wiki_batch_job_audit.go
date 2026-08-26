package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// wikiBatchAuditRepository persists wiki_batch_job_audit rows.
//
// Append-only by design (Build #14 D2: compliance + simplification). The
// repo only exposes Insert + read paths; there is intentionally no Update
// or Delete from request paths. Cleanup is left to a future cron (Build
// #14.x) that operates on the ExpiredEvents read path.
//
// Build #14.
type wikiBatchAuditRepository struct {
	db *gorm.DB
}

// NewWikiBatchAuditRepository wires the concrete repository. Returns
// the interface so consumers depend on contracts only.
func NewWikiBatchAuditRepository(db *gorm.DB) interfaces.WikiBatchAuditRepository {
	return &wikiBatchAuditRepository{db: db}
}

// auditRow is the GORM-side mirror of WikiBatchJobAuditEvent. We keep
// them separate so the public type stays JSON-shaped (no GORM tags,
// no bigserial int64 column tag noise leaking into the API surface).
type auditRow struct {
	ID              int64           `gorm:"column:id;primaryKey;autoIncrement"`
	TenantID        uint64          `gorm:"column:tenant_id"`
	KnowledgeBaseID string          `gorm:"column:knowledge_base_id"`
	BatchJobID      string          `gorm:"column:batch_job_id"`
	Action          string          `gorm:"column:action"`
	ActorID         string          `gorm:"column:actor_id"`
	OccurredAt      time.Time       `gorm:"column:occurred_at"`
	Metadata        types.JSON      `gorm:"column:metadata"`
	// CorrelationID — X-Request-ID or worker stamp from Build #25.
	// Empty string → NULL column (the column is nullable).
	CorrelationID string `gorm:"column:correlation_id;size:64"`
}

// TableName pins the GORM table name (avoids the pluralized default).
func (auditRow) TableName() string { return "wiki_batch_job_audit" }

// Insert appends one audit row. Stamps OccurredAt with server time
// when the caller left it zero, and returns the row with the assigned
// ID + OccurredAt populated. Metadata is JSONB on the wire; we use the
// existing types.JSON helper to round-trip arbitrary maps without a
// hand-written Marshal/Unmarshal.
func (r *wikiBatchAuditRepository) Insert(ctx context.Context, event *types.WikiBatchJobAuditEvent) error {
	if event == nil {
		return gorm.ErrInvalidData
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	meta, err := marshalMetadata(event.Metadata)
	if err != nil {
		return err
	}
	row := auditRow{
		TenantID:        event.TenantID,
		KnowledgeBaseID: event.KnowledgeBaseID,
		BatchJobID:      event.BatchJobID,
		Action:          string(event.Action),
		ActorID:         event.ActorID,
		OccurredAt:      event.OccurredAt,
		Metadata:        meta,
		CorrelationID:   event.CorrelationID,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	event.ID = row.ID
	event.OccurredAt = row.OccurredAt
	if len(meta) > 0 {
		event.Metadata = unmarshalMetadata(row.Metadata)
	}
	return nil
}

// ListByJobID returns every audit row for one batch job, oldest-first.
// Per-job cardinality is bounded (<= 7 events) so no pagination.
func (r *wikiBatchAuditRepository) ListByJobID(ctx context.Context, kbID, jobID string) ([]*types.WikiBatchJobAuditEvent, error) {
	var rows []auditRow
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND batch_job_id = ?", kbID, jobID).
		Order("occurred_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToEvents(rows), nil
}

// ListByKB returns audit rows scoped to one KB, newest-first, with
// optional filters. `actor`, `action`, `since` use zero values
// (empty / zero time / "") to skip that filter. The `since` lower
// bound is enforced at the handler layer (max 90 days per Build #14
// D4) — the repo trusts whatever value it receives.
//
// Pagination is 1-based; the pageSize cap is enforced at the handler.
// The total count runs as a separate count query (cheap thanks to
// the kb_id+occurred_at index).
func (r *wikiBatchAuditRepository) ListByKB(
	ctx context.Context,
	kbID, actor string,
	action types.WikiBatchAuditAction,
	since time.Time,
	page, pageSize int,
) ([]*types.WikiBatchJobAuditEvent, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	q := r.db.WithContext(ctx).Model(&auditRow{}).
		Where("knowledge_base_id = ?", kbID)
	if actor != "" {
		q = q.Where("actor_id = ?", actor)
	}
	if action != "" {
		q = q.Where("action = ?", string(action))
	}
	if !since.IsZero() {
		q = q.Where("occurred_at >= ?", since)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []auditRow
	if err := q.Order("occurred_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rowsToEvents(rows), total, nil
}

// ListExpiredEvents returns events older than `before`, capped by
// `limit`. Pass limit=0 for no cap. Used by the future cleanup cron
// (Build #14.x) — exposed now so the path is testable from harness.
func (r *wikiBatchAuditRepository) ListExpiredEvents(ctx context.Context, before time.Time, limit int) ([]*types.WikiBatchJobAuditEvent, error) {
	q := r.db.WithContext(ctx).Model(&auditRow{}).
		Where("occurred_at < ?", before).
		Order("occurred_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []auditRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToEvents(rows), nil
}

// marshalMetadata converts the optional metadata map into a JSONB
// byte slice. nil maps become nil (column nullable) so we don't store
// empty objects for events that carry no payload.
func marshalMetadata(m map[string]interface{}) (types.JSON, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return types.JSON(b), nil
}

// unmarshalMetadata is the symmetric reverse used after Insert when we
// want to round-trip the metadata back to the caller's struct. nil
// input → nil map.
func unmarshalMetadata(b types.JSON) map[string]interface{} {
	if len(b) == 0 {
		return nil
	}
	m, err := b.Map()
	if err != nil {
		// Bad metadata should not poison the API response — return
		// nil and let the caller treat the row as metadata-less.
		return nil
	}
	return m
}

// rowsToEvents converts GORM-side rows into the public type, decoding
// metadata along the way.
func rowsToEvents(rows []auditRow) []*types.WikiBatchJobAuditEvent {
	out := make([]*types.WikiBatchJobAuditEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, &types.WikiBatchJobAuditEvent{
			ID:              row.ID,
			TenantID:        row.TenantID,
			KnowledgeBaseID: row.KnowledgeBaseID,
			BatchJobID:      row.BatchJobID,
			Action:          types.WikiBatchAuditAction(row.Action),
			ActorID:         row.ActorID,
			OccurredAt:      row.OccurredAt,
			Metadata:        unmarshalMetadata(row.Metadata),
			CorrelationID:   row.CorrelationID,
		})
	}
	return out
}