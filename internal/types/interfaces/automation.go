package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AutomationRepository persists automations and their runs.
type AutomationRepository interface {
	CreateAutomation(ctx context.Context, a *types.Automation) error
	UpdateAutomation(ctx context.Context, a *types.Automation) error
	GetAutomation(ctx context.Context, tenantID uint64, id string) (*types.Automation, error)
	ListAutomationsByDatabase(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error)
	ListEnabledScheduled(ctx context.Context) ([]*types.Automation, error)
	ListEnabledFieldChange(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error)
	SoftDeleteAutomation(ctx context.Context, tenantID uint64, id string) error

	CreateRun(ctx context.Context, r *types.AutomationRun) error
	UpdateRun(ctx context.Context, r *types.AutomationRun) error
	GetRun(ctx context.Context, tenantID uint64, id string) (*types.AutomationRun, error)
	ListRunsByAutomation(ctx context.Context, tenantID uint64, automationID string, limit int) ([]*types.AutomationRun, error)
}
