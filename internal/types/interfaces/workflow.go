package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WorkflowRepository persists workflows + runs.
type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, w *types.Workflow) error
	GetWorkflow(ctx context.Context, tenantID uint64, id string) (*types.Workflow, error)
	ListWorkflowsByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.Workflow, error)
	UpdateWorkflow(ctx context.Context, w *types.Workflow) error
	DeleteWorkflow(ctx context.Context, tenantID uint64, id string) error

	CreateRun(ctx context.Context, r *types.WorkflowRun) error
	UpdateRun(ctx context.Context, r *types.WorkflowRun) error
	GetRun(ctx context.Context, id string) (*types.WorkflowRun, error)
	ListRunsByWorkflow(ctx context.Context, workflowID string, limit int) ([]*types.WorkflowRun, error)

	CreateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error
	UpdateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error
	ListNodeRuns(ctx context.Context, runID string) ([]*types.WorkflowNodeRun, error)
}
