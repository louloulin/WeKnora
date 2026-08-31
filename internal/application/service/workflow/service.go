package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// Service is the entry point for AI Workflow Builder CRUD.
type Service struct {
	repo interfaces.WorkflowRepository
	now  func() time.Time
}

// NewService constructs a Service.
func NewService(repo interfaces.WorkflowRepository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SetNow overrides the wall clock for tests.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// Create validates the workflow DAG and persists a new workflow.
func (s *Service) Create(ctx context.Context, w *types.Workflow) error {
	if w.Name == "" {
		return errors.New("workflow: name is required")
	}
	if err := ValidateDAG(w); err != nil {
		return err
	}
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	if w.Version == 0 {
		w.Version = 1
	}
	now := s.now()
	w.CreatedAt = now
	w.UpdatedAt = now
	return s.repo.CreateWorkflow(ctx, w)
}

// Get returns a single workflow by ID.
func (s *Service) Get(ctx context.Context, tenantID uint64, id string) (*types.Workflow, error) {
	return s.repo.GetWorkflow(ctx, tenantID, id)
}

// ListByKB returns every workflow bound to a knowledge base.
func (s *Service) ListByKB(ctx context.Context, tenantID uint64, kbID string) ([]*types.Workflow, error) {
	return s.repo.ListWorkflowsByKB(ctx, tenantID, kbID)
}

// Update mutates an existing workflow, re-validating the DAG.
func (s *Service) Update(ctx context.Context, w *types.Workflow) error {
	if err := ValidateDAG(w); err != nil {
		return err
	}
	w.Version++
	w.UpdatedAt = s.now()
	return s.repo.UpdateWorkflow(ctx, w)
}

// Delete removes a workflow by ID.
func (s *Service) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.DeleteWorkflow(ctx, tenantID, id)
}

// Run starts a new workflow execution. The returned run is persisted in
// the queued state; full execution is handled by Executor (out of scope
// for the foundation).
func (s *Service) Run(ctx context.Context, tenantID uint64, workflowID string, triggeredBy string, input map[string]any) (*types.WorkflowRun, error) {
	w, err := s.repo.GetWorkflow(ctx, tenantID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow: get: %w", err)
	}
	if !w.Enabled {
		return nil, errors.New("workflow: disabled")
	}
	inputJSON, _ := json.Marshal(input)
	if inputJSON == nil {
		inputJSON = json.RawMessage("{}")
	}
	run := &types.WorkflowRun{
		ID:          uuid.NewString(),
		WorkflowID:  workflowID,
		TenantID:    tenantID,
		Status:      string(types.WorkflowStatusQueued),
		TriggeredBy: triggeredBy,
		Input:       inputJSON,
		CreatedAt:   s.now(),
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// GetRun returns a workflow run with its per-node runs.
func (s *Service) GetRun(ctx context.Context, id string) (*types.WorkflowRun, error) {
	return s.repo.GetRun(ctx, id)
}

// ListRuns returns the latest runs for a workflow.
func (s *Service) ListRuns(ctx context.Context, workflowID string, limit int) ([]*types.WorkflowRun, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListRunsByWorkflow(ctx, workflowID, limit)
}
