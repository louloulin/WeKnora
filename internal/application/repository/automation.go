package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// AutomationRepository is the gorm-backed implementation of
// interfaces.AutomationRepository. It works against both postgres
// and sqlite because all queries use the standard gorm dialect.
type AutomationRepository struct {
	db *gorm.DB
}

// NewAutomationRepository wires the repo.
func NewAutomationRepository(db *gorm.DB) *AutomationRepository {
	return &AutomationRepository{db: db}
}

func (r *AutomationRepository) CreateAutomation(ctx context.Context, a *types.Automation) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *AutomationRepository) UpdateAutomation(ctx context.Context, a *types.Automation) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *AutomationRepository) GetAutomation(ctx context.Context, tenantID uint64, id string) (*types.Automation, error) {
	var a types.Automation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (r *AutomationRepository) ListAutomationsByDatabase(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error) {
	var out []*types.Automation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND database_id = ?", tenantID, databaseID).
		Order("created_at DESC").
		Find(&out).Error
	return out, err
}

func (r *AutomationRepository) ListEnabledScheduled(ctx context.Context) ([]*types.Automation, error) {
	var out []*types.Automation
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND trigger_type = ?", true, types.AutomationTriggerScheduled).
		Find(&out).Error
	return out, err
}

func (r *AutomationRepository) ListEnabledFieldChange(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error) {
	var out []*types.Automation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND database_id = ? AND enabled = ? AND trigger_type = ?",
			tenantID, databaseID, true, types.AutomationTriggerFieldChange).
		Find(&out).Error
	return out, err
}

func (r *AutomationRepository) SoftDeleteAutomation(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.Automation{}).Error
}

func (r *AutomationRepository) CreateRun(ctx context.Context, run *types.AutomationRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *AutomationRepository) UpdateRun(ctx context.Context, run *types.AutomationRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *AutomationRepository) GetRun(ctx context.Context, tenantID uint64, id string) (*types.AutomationRun, error) {
	var run types.AutomationRun
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (r *AutomationRepository) ListRunsByAutomation(ctx context.Context, tenantID uint64, automationID string, limit int) ([]*types.AutomationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []*types.AutomationRun
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND automation_id = ?", tenantID, automationID).
		Order("started_at DESC").
		Limit(limit).
		Find(&out).Error
	return out, err
}
