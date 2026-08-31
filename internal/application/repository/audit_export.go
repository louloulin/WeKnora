package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// NewAuditExportRepository returns a GORM-backed AuditExportRepository.
// The constructor follows the same pattern as the other repositories
// in this package so tests can swap a fake without rewiring DI.
func NewAuditExportRepository(db *gorm.DB) interfaces.AuditExportRepository {
	return &auditExportRepository{db: db}
}

type auditExportRepository struct {
	db *gorm.DB
}

// Create inserts a new export row.
func (r *auditExportRepository) Create(ctx context.Context, export *types.AuditExport) error {
	return r.db.WithContext(ctx).Create(export).Error
}

// GetByID returns a single export scoped to the tenant. Returns
// (nil, nil) when missing — the service translates nil into
// ErrAuditExportNotFound so the repo doesn't need to import sentinels.
func (r *auditExportRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.AuditExport, error) {
	var export types.AuditExport
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&export).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &export, nil
}

// ListByTenant returns the most recent exports for a tenant, newest
// first. The limit is clamped at 100 by the service so the repo can
// stay simple.
func (r *auditExportRepository) ListByTenant(ctx context.Context, tenantID uint64, limit int) ([]types.AuditExport, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []types.AuditExport
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if rows == nil {
		rows = []types.AuditExport{}
	}
	return rows, err
}

// UpdateStatus transitions an export to the supplied status. The
// row_count + byte_size pair are updated when status is succeeded;
// the error message is updated when status is failed.
func (r *auditExportRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status types.AuditExportStatus,
	rowCount int64,
	byteSize int64,
	errMsg string,
) error {
	updates := map[string]interface{}{
		"status":  status,
		"error":   errMsg,
	}
	if status == types.AuditExportStatusSucceeded {
		updates["row_count"] = rowCount
		updates["byte_size"] = byteSize
		now := time.Now()
		updates["finished_at"] = now
		// Default 7-day retention; the cleanup sweep picks them up after.
		updates["expires_at"] = now.Add(7 * 24 * time.Hour)
	}
	if status == types.AuditExportStatusFailed {
		now := time.Now()
		updates["finished_at"] = now
	}
	return r.db.WithContext(ctx).
		Model(&types.AuditExport{}).
		Where("id = ?", id).
		Updates(updates).Error
}
