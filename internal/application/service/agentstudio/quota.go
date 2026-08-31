package agentstudio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Quota errors.
var (
	ErrorQuotaExceeded      = errors.New("agentstudio.quota: quota exceeded")
	ErrorQuotaUnknownOp    = errors.New("agentstudio.quota: unknown operation")
)

// Quota enforces the active quota policy for a tenant + agent. Every
// charge appends an entry to the immutable agent_credit_ledger so the
// tenant can audit consumption across runs.
type Quota struct {
	repo typesRepo
	// clock lets tests freeze "now". Real callers leave it nil.
	now func() time.Time
}

// NewQuota wires the quota service to the repo.
func NewQuota(repo typesRepo) *Quota {
	return &Quota{repo: repo, now: time.Now}
}

// Check runs before a run starts. Returns nil if all limits allow the run.
// For zero-valued caps the corresponding dimension is unlimited.
func (q *Quota) Check(ctx context.Context, tenantID uint64, agentID string) error {
	policy, err := q.repo.GetActivePolicy(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("quota.get_active: %w", err)
	}
	if policy == nil {
		return nil // no policy = unlimited
	}
	now := q.now()
	// Monthly tokens: charge window is the rolling 30 days from now.
	if policy.MonthlyTokens > 0 {
		since := now.Add(-30 * 24 * time.Hour)
		used, err := q.repo.SumChargesSince(ctx, tenantID, agentID, types.AgentUnitTokens, since)
		if err != nil {
			return fmt.Errorf("quota.monthly_tokens: %w", err)
		}
		if used >= policy.MonthlyTokens {
			return fmt.Errorf("%w: monthly tokens used=%d cap=%d", ErrorQuotaExceeded, used, policy.MonthlyTokens)
		}
	}
	// Daily invocations: today since UTC midnight.
	if policy.DailyInvocations > 0 {
		since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		used, err := q.repo.CountInvocationsSince(ctx, tenantID, agentID, since)
		if err != nil {
			return fmt.Errorf("quota.daily_invocations: %w", err)
		}
		if used >= policy.DailyInvocations {
			return fmt.Errorf("%w: daily invocations used=%d cap=%d", ErrorQuotaExceeded, used, policy.DailyInvocations)
		}
	}
	return nil
}

// ChargeForRun appends a single ledger entry covering the run's total
// token consumption + invocation count + cost. Safe to call at run
// completion (idempotency-by-run-id not yet implemented; future work).
func (q *Quota) ChargeForRun(ctx context.Context, run *types.AgentRun) error {
	if run == nil {
		return ErrorQuotaUnknownOp
	}
	policy, err := q.repo.GetActivePolicy(ctx, run.TenantID)
	policyVersion := int64(1)
	if err == nil && policy != nil {
		policyVersion = policy.Version
	}
	if run.TokensUsed > 0 {
		if err := q.appendEntry(ctx, run, types.AgentUnitTokens, run.TokensUsed,
			types.AgentLedgerOpCharge, policyVersion, "run tokens"); err != nil {
			return err
		}
	}
	if run.CostMicros > 0 {
		if err := q.appendEntry(ctx, run, types.AgentUnitCostMicros, run.CostMicros,
			types.AgentLedgerOpCharge, policyVersion, "run cost (micro-USD)"); err != nil {
			return err
		}
	}
	// Always charge 1 invocation — even failed runs count for the daily cap.
	if err := q.appendEntry(ctx, run, types.AgentUnitInvocations, 1,
		types.AgentLedgerOpCharge, policyVersion, "run invocation"); err != nil {
		return err
	}
	return nil
}

func (q *Quota) appendEntry(ctx context.Context, run *types.AgentRun,
	unit string, qty int64, op string, policyVer int64, note string,
) error {
	// Compute running balance: read existing sum and add qty.
	prev, err := q.repo.SumChargesSince(ctx, run.TenantID, run.AgentID, unit, time.Unix(0, 0))
	if err != nil {
		return err
	}
	balance := prev + qty
	entry := &types.AgentCreditLedgerEntry{
		TenantID:      run.TenantID,
		AgentID:       run.AgentID,
		RunID:         &run.ID,
		Operation:     op,
		Unit:          unit,
		Quantity:      qty,
		BalanceAfter:  balance,
		PolicyVersion: policyVer,
		Notes:         note,
	}
	return q.repo.AppendLedger(ctx, entry)
}

// ActivatePolicy promotes the named policy version to active. The
// previous active version (if any) is deactivated atomically.
func (q *Quota) ActivatePolicy(ctx context.Context, tenantID uint64, name string, version int64) error {
	return q.repo.ActivatePolicy(ctx, tenantID, name, version)
}

// CreatePolicy versions up by 1 over the latest version of the same name.
func (q *Quota) CreatePolicy(ctx context.Context, tenantID, createdBy uint64, name string,
	monthlyTokens, dailyInvocations, perRunCostCap int64, perAgentConcurrency int,
) (*types.AgentQuotaPolicy, error) {
	existing, err := q.repo.ListPolicies(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	version := int64(1)
	for _, p := range existing {
		if p.Name == name && p.Version >= version {
			version = p.Version + 1
		}
	}
	p := &types.AgentQuotaPolicy{
		TenantID:            tenantID,
		Name:                name,
		Version:             version,
		MonthlyTokens:       monthlyTokens,
		DailyInvocations:    dailyInvocations,
		PerRunCostCapMicros: perRunCostCap,
		PerAgentConcurrency: perAgentConcurrency,
		IsActive:            false,
		CreatedBy:           createdBy,
	}
	if err := q.repo.CreatePolicy(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
