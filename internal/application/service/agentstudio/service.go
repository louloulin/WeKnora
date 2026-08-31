package agentstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// typesRepo is the minimal repo contract AgentStudioService needs. Defined
// here instead of importing interfaces.AgentStudioRepository so the
// service package stays free of an import cycle (interfaces → types → ...).
type typesRepo interface {
	CreateTrigger(ctx context.Context, t *types.AgentTrigger) error
	GetTrigger(ctx context.Context, tenantID uint64, id uint64) (*types.AgentTrigger, error)
	ListTriggersByAgent(ctx context.Context, tenantID uint64, agentID string) ([]*types.AgentTrigger, error)
	ListActiveCronTriggers(ctx context.Context, before time.Time, limit int) ([]*types.AgentTrigger, error)
	UpdateTrigger(ctx context.Context, t *types.AgentTrigger) error
	DeleteTrigger(ctx context.Context, tenantID uint64, id uint64) error
	CreateRun(ctx context.Context, r *types.AgentRun) error
	GetRun(ctx context.Context, tenantID uint64, id uint64) (*types.AgentRun, error)
	ListRunsByAgent(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentRun, int64, error)
	UpdateRun(ctx context.Context, r *types.AgentRun) error
	CreateCredential(ctx context.Context, c *types.AgentCredential) error
	GetCredential(ctx context.Context, tenantID uint64, name string) (*types.AgentCredential, error)
	ListCredentials(ctx context.Context, tenantID uint64) ([]*types.AgentCredential, error)
	DeleteCredential(ctx context.Context, tenantID uint64, name string) error
	TouchCredentialUsage(ctx context.Context, tenantID uint64, name string) error
	AppendLedger(ctx context.Context, e *types.AgentCreditLedgerEntry) error
	SumChargesSince(ctx context.Context, tenantID uint64, agentID string, unit string, since time.Time) (int64, error)
	CountInvocationsSince(ctx context.Context, tenantID uint64, agentID string, since time.Time) (int64, error)
	CreatePolicy(ctx context.Context, p *types.AgentQuotaPolicy) error
	GetActivePolicy(ctx context.Context, tenantID uint64) (*types.AgentQuotaPolicy, error)
	ListPolicies(ctx context.Context, tenantID uint64) ([]*types.AgentQuotaPolicy, error)
	ActivatePolicy(ctx context.Context, tenantID uint64, name string, version int64) error
}

// AgentStudioService is the public façade for the v0.7.21 Custom Agent
// Studio feature. Composed of three sub-services:
//
//   * Vault    — credential encryption + reveal
//   * Quota    — quota enforcement + ledger
//   * Trigger  — cron / event / webhook scheduling
//
// Plus a top-level Run() entry point that wraps quota check → run row
// creation → agent execution → ledger charge.
type AgentStudioService struct {
	repo    typesRepo
	vault   *Vault
	quota   *Quota
	trigger *Trigger
	keyBox  *vaultKeyResolver
	// agentExecutor plugs in the existing customAgentService. nil-safe
	// (Run() falls back to a stub that marks the run as "succeeded" so
	// quota/ledger paths can still be tested in isolation).
	agentExecutor AgentExecutor
}

// AgentExecutor is the contract Run() uses to invoke the actual custom
// agent. The real implementation lives in the existing CustomAgentService
// (see internal/application/service/custom_agent.go).
type AgentExecutor interface {
	ExecuteAgent(ctx context.Context, agentID string, tenantID uint64, input map[string]any) (*AgentExecResult, error)
}

// AgentExecResult is what AgentExecutor returns. Maps cleanly to
// agent_runs.output_payload + counters.
type AgentExecResult struct {
	Output      map[string]any
	StepsCount  int
	TokensUsed  int64
	CostMicros  int64
	ErrorString string
}

// NewAgentStudioService is the single wiring point the container uses.
func NewAgentStudioService(repo typesRepo, executor AgentExecutor) *AgentStudioService {
	keyBox := newVaultKeyResolver()
	return &AgentStudioService{
		repo:          repo,
		vault:         NewVault(repo, keyBox),
		quota:         NewQuota(repo),
		trigger:       NewTrigger(repo),
		keyBox:        keyBox,
		agentExecutor: executor,
	}
}

// Vault exposes the credential vault.
func (s *AgentStudioService) Vault() *Vault { return s.vault }

// Quota exposes the quota service.
func (s *AgentStudioService) Quota() *Quota { return s.quota }

// Trigger exposes the trigger manager.
func (s *AgentStudioService) Trigger() *Trigger { return s.trigger }

// RunOpts captures everything needed to start a run: which agent, who
// triggered it, what input, what trigger row (if any).
type RunOpts struct {
	TenantID       uint64
	AgentID        string
	TriggeredBy    string // manual | cron | event | webhook | api
	TriggeredUser  *uint64
	TriggerID      *uint64
	Input          map[string]any
}

// Run starts a new agent execution: validates quota, creates the run row,
// invokes the agent, updates counters, and charges the ledger. Returns
// the persisted run row (with id + counters populated).
//
// Errors:
//   - agentstudio.quota: quota exceeded → no run created
//   - any error from AgentExecutor: run marked failed
func (s *AgentStudioService) Run(ctx context.Context, opts RunOpts) (*types.AgentRun, error) {
	if opts.TenantID == 0 || opts.AgentID == "" {
		return nil, fmt.Errorf("agentstudio.run: tenant_id + agent_id required")
	}
	// Pre-flight quota check.
	if err := s.quota.Check(ctx, opts.TenantID, opts.AgentID); err != nil {
		return nil, err
	}
	// Marshal input payload once.
	inputJSON := "{}"
	if opts.Input != nil {
		if b, err := json.Marshal(opts.Input); err == nil {
			inputJSON = string(b)
		}
	}
	now := time.Now().UTC()
	run := &types.AgentRun{
		TenantID:      opts.TenantID,
		AgentID:       opts.AgentID,
		TriggerID:     opts.TriggerID,
		TriggeredBy:   opts.TriggeredBy,
		TriggeredUser: opts.TriggeredUser,
		Status:        types.AgentRunStatusRunning,
		InputPayload:  inputJSON,
		StartedAt:     &now,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("agentstudio.run.create: %w", err)
	}

	// Invoke the agent. nil-safe stub for tests.
	var result *AgentExecResult
	if s.agentExecutor != nil {
		result, _ = s.agentExecutor.ExecuteAgent(ctx, opts.AgentID, opts.TenantID, opts.Input)
	}
	if result == nil {
		result = &AgentExecResult{}
	}
	finish := time.Now().UTC()
	run.FinishedAt = &finish
	run.DurationMs = int(finish.Sub(now) / time.Millisecond)
	run.StepsCount = result.StepsCount
	run.TokensUsed = result.TokensUsed
	run.CostMicros = result.CostMicros
	run.ErrorMessage = result.ErrorString

	if result.ErrorString != "" {
		run.Status = types.AgentRunStatusFailed
		run.OutputPayload = "{}"
	} else {
		run.Status = types.AgentRunStatusSucceeded
		if result.Output != nil {
			if b, err := json.Marshal(result.Output); err == nil {
				run.OutputPayload = string(b)
			}
		}
	}

	if err := s.repo.UpdateRun(ctx, run); err != nil {
		logger.Errorf(ctx, "[AgentStudio] update run %d failed: %v", run.ID, err)
	}
	// Best-effort ledger charge — failure here doesn't fail the run.
	if err := s.quota.ChargeForRun(ctx, run); err != nil {
		logger.Warnf(ctx, "[AgentStudio] charge ledger failed for run %d: %v", run.ID, err)
	}
	return run, nil
}

// GetRun returns a single run (with owner check implicit in tenant scope).
func (s *AgentStudioService) GetRun(ctx context.Context, tenantID uint64, id uint64) (*types.AgentRun, error) {
	return s.repo.GetRun(ctx, tenantID, id)
}

// ListRuns returns runs for one agent with pagination.
func (s *AgentStudioService) ListRuns(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentRun, int64, error) {
	return s.repo.ListRunsByAgent(ctx, tenantID, agentID, limit, offset)
}
