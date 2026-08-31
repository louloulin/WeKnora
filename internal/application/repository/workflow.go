package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// workflowRepository is the gorm-backed implementation of interfaces.WorkflowRepository.
type workflowRepository struct {
	db *gorm.DB
}

// NewWorkflowRepository constructs a gorm WorkflowRepository.
func NewWorkflowRepository(db *gorm.DB) interfaces.WorkflowRepository {
	return &workflowRepository{db: db}
}

// serializeNodes packs a workflow's Nodes / Edges into JSON for storage.
func serializeNodes(w *types.Workflow) {
	if w.Nodes != nil {
		buf, _ := json.Marshal(w.Nodes)
		w.NodeBlob = buf
	}
	if w.Edges != nil {
		buf, _ := json.Marshal(w.Edges)
		w.EdgeBlob = buf
	}
}

// deserializeNodes unpacks a workflow's Nodes / Edges after loading.
func deserializeNodes(w *types.Workflow) {
	if w.NodeBlob != nil {
		_ = json.Unmarshal(w.NodeBlob, &w.Nodes)
	}
	if w.EdgeBlob != nil {
		_ = json.Unmarshal(w.EdgeBlob, &w.Edges)
	}
}

func (r *workflowRepository) CreateWorkflow(ctx context.Context, w *types.Workflow) error {
	serializeNodes(w)
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *workflowRepository) GetWorkflow(ctx context.Context, tenantID uint64, id string) (*types.Workflow, error) {
	var w types.Workflow
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&w).Error; err != nil {
		return nil, err
	}
	deserializeNodes(&w)
	return &w, nil
}

func (r *workflowRepository) ListWorkflowsByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.Workflow, error) {
	var out []*types.Workflow
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("updated_at DESC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	for _, w := range out {
		deserializeNodes(w)
	}
	return out, nil
}

func (r *workflowRepository) UpdateWorkflow(ctx context.Context, w *types.Workflow) error {
	serializeNodes(w)
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *workflowRepository) DeleteWorkflow(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.Workflow{}).Error
}

func (r *workflowRepository) CreateRun(ctx context.Context, run *types.WorkflowRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *workflowRepository) UpdateRun(ctx context.Context, run *types.WorkflowRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *workflowRepository) GetRun(ctx context.Context, id string) (*types.WorkflowRun, error) {
	var run types.WorkflowRun
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&run).Error; err != nil {
		return nil, err
	}
	nrs, err := r.ListNodeRuns(ctx, id)
	if err == nil {
		run.NodeRuns = make([]types.WorkflowNodeRun, len(nrs))
		for i, n := range nrs {
			run.NodeRuns[i] = *n
		}
	}
	return &run, nil
}

func (r *workflowRepository) ListRunsByWorkflow(ctx context.Context, workflowID string, limit int) ([]*types.WorkflowRun, error) {
	var out []*types.WorkflowRun
	if err := r.db.WithContext(ctx).
		Where("workflow_id = ?", workflowID).
		Order("created_at DESC").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *workflowRepository) CreateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error {
	return r.db.WithContext(ctx).Create(nr).Error
}

func (r *workflowRepository) UpdateNodeRun(ctx context.Context, nr *types.WorkflowNodeRun) error {
	return r.db.WithContext(ctx).Save(nr).Error
}

func (r *workflowRepository) ListNodeRuns(ctx context.Context, runID string) ([]*types.WorkflowNodeRun, error) {
	var nrs []types.WorkflowNodeRun
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("created_at ASC").
		Find(&nrs).Error; err != nil {
		return nil, err
	}
	out := make([]*types.WorkflowNodeRun, len(nrs))
	for i := range nrs {
		c := nrs[i]
		out[i] = &c
	}
	return out, nil
}

// now returns the current time; the indirection lets tests freeze time.
var now = func() time.Time { return time.Now().UTC() }

// suppress unused-import warnings in some build configurations.
var _ = now
