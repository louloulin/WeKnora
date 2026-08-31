package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// SCIMSyncLogRepository is the GORM-backed implementation of
// interfaces.SCIMSyncLogRepository.
type SCIMSyncLogRepository struct {
	db *gorm.DB
}

// NewSCIMSyncLogRepository constructs the repository.
func NewSCIMSyncLogRepository(db *gorm.DB) *SCIMSyncLogRepository {
	return &SCIMSyncLogRepository{db: db}
}

// Create inserts a single sync log row.
func (r *SCIMSyncLogRepository) Create(ctx context.Context, entry *types.SCIMSyncLog) error {
	if entry == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(entry).Error
}

// ListByTenant returns the most recent sync log entries for a
// tenant, bounded by limit (1 <= limit <= 500).
func (r *SCIMSyncLogRepository) ListByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.SCIMSyncLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	var rows []*types.SCIMSyncLog
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at desc").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
