package repository

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// wikiBatchFailureRepository persists wiki_batch_job_failures rows
// (Build #15). Append-only: the worker Inserts one row per failed
// slug, the handler reads via ListByJobID. There is intentionally no
// Update / Delete from request paths — failure rows are diagnostic
// data and stay immutable after the job completes (Build #15 D3:
// consistency with the audit table's append-only posture).
type wikiBatchFailureRepository struct {
	db *gorm.DB
}

// NewWikiBatchFailureRepository wires the concrete repository.
func NewWikiBatchFailureRepository(db *gorm.DB) interfaces.WikiBatchFailureRepository {
	return &wikiBatchFailureRepository{db: db}
}

// failureRow is the GORM-side mirror of WikiBatchJobFailureRecord.
type failureRow struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TenantID        uint64    `gorm:"column:tenant_id"`
	KnowledgeBaseID string    `gorm:"column:knowledge_base_id"`
	BatchJobID      string    `gorm:"column:batch_job_id"`
	Slug            string    `gorm:"column:slug"`
	Code            string    `gorm:"column:code"`
	Error           string    `gorm:"column:error"`
	OccurredAt      time.Time `gorm:"column:occurred_at"`
}

// TableName pins the GORM table name (avoids the pluralized default).
func (failureRow) TableName() string { return "wiki_batch_job_failures" }

// Insert appends one failure row. Stamps OccurredAt with server time
// when the caller left it zero, and returns the row with the assigned
// ID + OccurredAt populated so the caller can echo the assigned id
// back to its caller without a re-fetch.
func (r *wikiBatchFailureRepository) Insert(ctx context.Context, rec *types.WikiBatchJobFailureRecord) error {
	if rec == nil {
		return gorm.ErrInvalidData
	}
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now()
	}
	row := failureRow{
		TenantID:        rec.TenantID,
		KnowledgeBaseID: rec.KnowledgeBaseID,
		BatchJobID:      rec.BatchJobID,
		Slug:            rec.Slug,
		Code:            rec.Code,
		Error:           rec.Error,
		OccurredAt:      rec.OccurredAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	rec.ID = row.ID
	rec.OccurredAt = row.OccurredAt
	return nil
}

// ListByJobID returns failure rows for one batch job, oldest-first,
// with optional code filter (empty string = no filter). Pagination
// is 1-based; the pageSize cap is enforced at the handler layer.
//
// Returns three things in one round-trip:
//   - failures: the current page of rows (oldest-first)
//   - groups:  per-code count slice covering all failures for this job
//   - total:   total failure count (after the code filter) for the UI paginator
//
// The groups slice is computed over the full filtered set, not just
// the page — so the drawer's code tabs always show the full picture
// even if the user is on page 3.
func (r *wikiBatchFailureRepository) ListByJobID(
	ctx context.Context,
	kbID, jobID, code string,
	page, pageSize int,
) ([]*types.WikiBatchJobFailureRecord, []types.WikiBatchFailureGroupCount, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	base := r.db.WithContext(ctx).Model(&failureRow{}).
		Where("knowledge_base_id = ? AND batch_job_id = ?", kbID, jobID)
	if code != "" {
		base = base.Where("code = ?", code)
	}

	// 1) total failure count under the filter (for the paginator).
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, nil, 0, err
	}

	// 2) per-code group counts over the full filtered set.
	type codeRow struct {
		Code  string
		Count int64
	}
	var codeRows []codeRow
	if err := base.Select("code, COUNT(*) AS count").
		Group("code").
		Order("count DESC, code ASC").
		Scan(&codeRows).Error; err != nil {
		return nil, nil, 0, err
	}
	groups := make([]types.WikiBatchFailureGroupCount, 0, len(codeRows))
	for _, cr := range codeRows {
		groups = append(groups, types.WikiBatchFailureGroupCount{
			Code:  cr.Code,
			Count: int(cr.Count),
		})
	}

	// 3) current page of rows (oldest-first so the drawer renders top-down).
	var rows []failureRow
	if err := base.Order("occurred_at ASC, id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, nil, 0, err
	}

	out := make([]*types.WikiBatchJobFailureRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, &types.WikiBatchJobFailureRecord{
			ID:              row.ID,
			TenantID:        row.TenantID,
			KnowledgeBaseID: row.KnowledgeBaseID,
			BatchJobID:      row.BatchJobID,
			Slug:            row.Slug,
			Code:            row.Code,
			Error:           row.Error,
			OccurredAt:      row.OccurredAt,
		})
	}
	return out, groups, total, nil
}