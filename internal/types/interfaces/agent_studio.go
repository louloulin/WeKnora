package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// AgentStudioRepository bundles CRUD for the five new v0.7.21 tables:
// agent_triggers, agent_runs, agent_credentials, agent_credit_ledger,
// agent_quota_policies. The interface keeps the service layer free to
// swap in a mock for unit tests without depending on GORM.
type AgentStudioRepository interface {
	// Trigger CRUD
	CreateTrigger(ctx context.Context, t *types.AgentTrigger) error
	GetTrigger(ctx context.Context, tenantID uint64, id uint64) (*types.AgentTrigger, error)
	ListTriggersByAgent(ctx context.Context, tenantID uint64, agentID string) ([]*types.AgentTrigger, error)
	ListActiveCronTriggers(ctx context.Context, before time.Time, limit int) ([]*types.AgentTrigger, error)
	UpdateTrigger(ctx context.Context, t *types.AgentTrigger) error
	DeleteTrigger(ctx context.Context, tenantID uint64, id uint64) error

	// Run CRUD
	CreateRun(ctx context.Context, r *types.AgentRun) error
	GetRun(ctx context.Context, tenantID uint64, id uint64) (*types.AgentRun, error)
	ListRunsByAgent(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentRun, int64, error)
	UpdateRun(ctx context.Context, r *types.AgentRun) error

	// Credential vault CRUD
	CreateCredential(ctx context.Context, c *types.AgentCredential) error
	GetCredential(ctx context.Context, tenantID uint64, name string) (*types.AgentCredential, error)
	ListCredentials(ctx context.Context, tenantID uint64) ([]*types.AgentCredential, error)
	DeleteCredential(ctx context.Context, tenantID uint64, name string) error
	TouchCredentialUsage(ctx context.Context, tenantID uint64, name string) error

	// Credit ledger
	AppendLedger(ctx context.Context, e *types.AgentCreditLedgerEntry) error
	SumChargesSince(ctx context.Context, tenantID uint64, agentID string, unit string, since time.Time) (int64, error)
	CountInvocationsSince(ctx context.Context, tenantID uint64, agentID string, since time.Time) (int64, error)

	// Quota policies
	CreatePolicy(ctx context.Context, p *types.AgentQuotaPolicy) error
	GetActivePolicy(ctx context.Context, tenantID uint64) (*types.AgentQuotaPolicy, error)
	ListPolicies(ctx context.Context, tenantID uint64) ([]*types.AgentQuotaPolicy, error)
	ActivatePolicy(ctx context.Context, tenantID uint64, name string, version int64) error
}

// Compile-time interface assertion. The repo must implement this contract.
var _ AgentStudioRepository = (AgentStudioRepository)(nil)
