// Package automation is the v0.7.27 Build #33 (P0 gap G32) automation
// / button engine. It provides CRUD on Automation records plus a DAG
// runner that executes steps in topological order with retries and
// per-step output propagation.
package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Service is the entry point for automation CRUD and execution.
type Service struct {
	repo    interfaces.AutomationRepository
	actions *ActionRegistry
	now     func() time.Time
}

// NewService wires the service with the default action registry. The
// optional agentRunner enables the RunAgent action; pass nil for tests
// that do not exercise that path (the action returns a sentinel error
// when invoked without a runner).
func NewService(repo interfaces.AutomationRepository, agentRunner AgentRunner) *Service {
	reg := NewActionRegistry()
	reg.Register(UpdateFieldAction{})
	reg.Register(CreateRowAction{})
	reg.Register(SendWebhookAction{})
	reg.Register(NotifyAction{})
	if agentRunner != nil {
		reg.Register(RunAgentAction{Runner: agentRunner})
	} else {
		reg.Register(RunAgentAction{})
	}
	return &Service{repo: repo, actions: reg, now: time.Now}
}

// NewServiceWithRegistry lets the caller register custom actions
// before the service starts. Used by tests and integrations that
// need a custom RunAgent runner.
func NewServiceWithRegistry(repo interfaces.AutomationRepository, reg *ActionRegistry) *Service {
	return &Service{repo: repo, actions: reg, now: time.Now}
}

// SetNow overrides the wall clock for tests.
func (s *Service) SetNow(now func() time.Time) {
	s.now = now
}

// Errors surfaced to callers.
var (
	ErrNotFound = errors.New("automation: not found")
)

// Create validates the DAG and persists a new automation.
func (s *Service) Create(ctx context.Context, a *types.Automation) error {
	if err := ValidateDAG(a); err != nil {
		return err
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := s.now()
	a.CreatedAt = now
	a.UpdatedAt = now
	return s.repo.CreateAutomation(ctx, a)
}

// Update validates the new DAG and persists the change.
func (s *Service) Update(ctx context.Context, a *types.Automation) error {
	if _, err := s.repo.GetAutomation(ctx, a.TenantID, a.ID); err != nil {
		return err
	}
	if err := ValidateDAG(a); err != nil {
		return err
	}
	a.UpdatedAt = s.now()
	return s.repo.UpdateAutomation(ctx, a)
}

// Delete soft-deletes the automation. Running runs are not aborted;
// the next scheduler tick will see Enabled=false and skip them.
func (s *Service) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.SoftDeleteAutomation(ctx, tenantID, id)
}

// Get loads the automation by id.
func (s *Service) Get(ctx context.Context, tenantID uint64, id string) (*types.Automation, error) {
	a, err := s.repo.GetAutomation(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrNotFound
	}
	return a, nil
}

// ListByDatabase returns all automations for a database.
func (s *Service) ListByDatabase(ctx context.Context, tenantID uint64, databaseID string) ([]*types.Automation, error) {
	return s.repo.ListAutomationsByDatabase(ctx, tenantID, databaseID)
}

// Run executes an automation end-to-end. The function returns when
// the run finishes (success or failure) and records a row in the
// run table.
func (s *Service) Run(ctx context.Context, a *types.Automation, inputs *types.AutomationRunInputs) (*types.AutomationRun, error) {
	run := &types.AutomationRun{
		ID:           uuid.NewString(),
		TenantID:     a.TenantID,
		AutomationID: a.ID,
		Trigger:      a.TriggerType,
		Status:       types.AutomationRunRunning,
		StartedAt:    s.now(),
	}
	ac := &ActionContext{
		TenantID:   a.TenantID,
		DatabaseID: a.DatabaseID,
		Trigger:    a.TriggerType,
		Output:     map[string]any{},
		Now:        s.now,
	}
	if inputs != nil {
		ac.TenantID = inputs.TenantID
		ac.DatabaseID = inputs.DatabaseID
		ac.RowID = inputs.RowID
		ac.UserID = inputs.UserID
		if inputs.ManualPayload != nil {
			ac.Output["manual"] = inputs.ManualPayload
		}
		if inputs.ChangedColumn != "" {
			ac.Output["changed_column"] = inputs.ChangedColumn
		}
		if inputs.NewValue != nil {
			ac.Output["new_value"] = inputs.NewValue
		}
		if inputs.OldValue != nil {
			ac.Output["old_value"] = inputs.OldValue
		}
	}

	// Persist the initial pending run row so the caller can observe
	// progress even if we crash mid-run.
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("automation: persist run: %w", err)
	}

	// Topological order so we can resume partial runs if we ever
	// add checkpointing.
	order := TopologicalSort(a)
	byID := make(map[string]*types.AutomationStep, len(a.Steps))
	for i := range a.Steps {
		byID[a.Steps[i].ID] = &a.Steps[i]
	}

	for _, stepID := range order {
		step := byID[stepID]
		sr := s.executeStep(ctx, step, ac)
		run.StepRuns = append(run.StepRuns, sr)
		if sr.Status == types.AutomationRunFailed {
			run.Status = types.AutomationRunFailed
			run.Error = sr.Error
			now := s.now()
			run.FinishedAt = &now
			if err := s.repo.UpdateRun(ctx, run); err != nil {
				logger.Errorf(ctx, "automation: update failed run: %v", err)
			}
			s.markLastFire(a, types.AutomationRunFailed)
			return run, errors.New(sr.Error)
		}
	}

	run.Status = types.AutomationRunSucceeded
	now := s.now()
	run.FinishedAt = &now
	if err := s.repo.UpdateRun(ctx, run); err != nil {
		logger.Errorf(ctx, "automation: update run: %v", err)
	}
	s.markLastFire(a, types.AutomationRunSucceeded)
	return run, nil
}

// executeStep runs one step with retries. Each retry is recorded
// as its own StepRun entry so the operator can inspect failure
// history.
func (s *Service) executeStep(ctx context.Context, step *types.AutomationStep, ac *ActionContext) types.AutomationStepRun {
	action := s.actions.Get(step.ActionType)
	if action == nil {
		return types.AutomationStepRun{
			StepID:    step.ID,
			Status:    types.AutomationRunFailed,
			StartedAt: s.now(),
			Error:     fmt.Sprintf("no action registered for kind %q", step.ActionType),
		}
	}
	attempts := step.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		started := s.now()
		result, err := action.Run(ctx, ac, step)
		finished := s.now()
		if err == nil {
			// Merge result into ActionContext.Output so later
			// steps can reference it via {{steps.<id>.<key>}}.
			if result != nil && result.Output != nil {
				ac.Output[step.ID] = result.Output
			}
			return types.AutomationStepRun{
				StepID:     step.ID,
				Status:     types.AutomationRunSucceeded,
				StartedAt:  started,
				FinishedAt: &finished,
				Output:     toJSON(result),
			}
		}
		lastErr = err
		// Retry with linear backoff capped at 5s.
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return types.AutomationStepRun{
					StepID:    step.ID,
					Status:    types.AutomationRunCancelled,
					StartedAt: started,
					Error:     ctx.Err().Error(),
				}
			case <-time.After(time.Duration(i+1) * time.Second):
			}
		}
	}
	return types.AutomationStepRun{
		StepID:    step.ID,
		Status:    types.AutomationRunFailed,
		StartedAt: s.now(),
		Error:     lastErr.Error(),
	}
}

// markLastFire updates the automation with the latest fire status.
// We swallow persistence errors so they do not mask the run failure.
func (s *Service) markLastFire(a *types.Automation, status types.AutomationRunStatus) {
	if a == nil {
		return
	}
	now := s.now()
	a.LastFiredAt = &now
	a.LastFireStatus = status
	a.UpdatedAt = now
	if err := s.repo.UpdateAutomation(context.Background(), a); err != nil {
		logger.Errorf(context.Background(), "automation: markLastFire: %v", err)
	}
}

func toJSON(v *ActionResult) types.JSON {
	if v == nil {
		return nil
	}
	b, err := jsonMarshal(v.Output)
	if err != nil {
		return nil
	}
	return b
}


// jsonMarshal is a tiny local helper so we don't import encoding/json at the top.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
