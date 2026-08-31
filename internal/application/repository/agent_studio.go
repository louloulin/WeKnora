package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// agentStudioRepository implements interfaces.AgentStudioRepository using
// raw SQL with cross-driver branching (Postgres ON CONFLICT / SQLite
// INSERT OR REPLACE). GORM's auto-migrate doesn't cover the new v0.7.21
// tables because they were added after the initial schema.
type agentStudioRepository struct {
	db *gorm.DB
}

// NewAgentStudioRepository wires the repo to the shared *gorm.DB.
func NewAgentStudioRepository(db *gorm.DB) *agentStudioRepository {
	return &agentStudioRepository{db: db}
}

// dialectFor returns the SQL dialect marker ("postgres" or "sqlite").
// Falls back to SQLite to keep dev profiles working when the driver
// name doesn't match (some wrappers register as "sqlite3").
func (r *agentStudioRepository) dialectFor() string {
	if r.db.Dialector != nil {
		name := r.db.Dialector.Name()
		if strings.Contains(name, "postgres") {
			return "postgres"
		}
	}
	return "sqlite"
}

// ---------------------------------------------------------------------------
// Trigger CRUD
// ---------------------------------------------------------------------------

func (r *agentStudioRepository) CreateTrigger(ctx context.Context, t *types.AgentTrigger) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO agent_triggers
				(tenant_id, agent_id, trigger_type, name, trigger_config, payload_template,
				 status, last_fired_at, last_fire_status, next_fire_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at, updated_at
		`, t.TenantID, t.AgentID, t.TriggerType, t.Name, t.TriggerConfig, t.PayloadTemplate,
			t.Status, t.LastFiredAt, t.LastFireStatus, t.NextFireAt, t.CreatedBy).
			Scan(t).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_triggers
			(tenant_id, agent_id, trigger_type, name, trigger_config, payload_template,
			 status, last_fired_at, last_fire_status, next_fire_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.TenantID, t.AgentID, t.TriggerType, t.Name, t.TriggerConfig, t.PayloadTemplate,
		t.Status, t.LastFiredAt, t.LastFireStatus, t.NextFireAt, t.CreatedBy).Error
}

func (r *agentStudioRepository) GetTrigger(ctx context.Context, tenantID uint64, id uint64) (*types.AgentTrigger, error) {
	var t types.AgentTrigger
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, agent_id, trigger_type, name, trigger_config, payload_template,
		        status, last_fired_at, last_fire_status, next_fire_at, created_by, created_at, updated_at
		 FROM agent_triggers WHERE tenant_id = ? AND id = ?`,
		tenantID, id,
	).Scan(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *agentStudioRepository) ListTriggersByAgent(ctx context.Context, tenantID uint64, agentID string) ([]*types.AgentTrigger, error) {
	var out []*types.AgentTrigger
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, agent_id, trigger_type, name, trigger_config, payload_template,
		        status, last_fired_at, last_fire_status, next_fire_at, created_by, created_at, updated_at
		 FROM agent_triggers WHERE tenant_id = ? AND agent_id = ? ORDER BY id`,
		tenantID, agentID,
	).Scan(&out).Error
	return out, err
}

func (r *agentStudioRepository) ListActiveCronTriggers(ctx context.Context, before time.Time, limit int) ([]*types.AgentTrigger, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []*types.AgentTrigger
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, agent_id, trigger_type, name, trigger_config, payload_template,
		        status, last_fired_at, last_fire_status, next_fire_at, created_by, created_at, updated_at
		 FROM agent_triggers
		 WHERE trigger_type = 'cron' AND status = 'active' AND next_fire_at IS NOT NULL AND next_fire_at <= ?
		 ORDER BY next_fire_at ASC LIMIT ?`,
		before, limit,
	).Scan(&out).Error
	return out, err
}

func (r *agentStudioRepository) UpdateTrigger(ctx context.Context, t *types.AgentTrigger) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE agent_triggers SET
			trigger_config = ?, payload_template = ?, status = ?,
			last_fired_at = ?, last_fire_status = ?, next_fire_at = ?, updated_at = NOW()
		WHERE tenant_id = ? AND id = ?`,
		t.TriggerConfig, t.PayloadTemplate, t.Status,
		t.LastFiredAt, t.LastFireStatus, t.NextFireAt, t.TenantID, t.ID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("agent_triggers: no row updated for id=%d", t.ID)
	}
	return nil
}

func (r *agentStudioRepository) DeleteTrigger(ctx context.Context, tenantID uint64, id uint64) error {
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM agent_triggers WHERE tenant_id = ? AND id = ?`, tenantID, id,
	).Error
}

// ---------------------------------------------------------------------------
// Run CRUD
// ---------------------------------------------------------------------------

func (r *agentStudioRepository) CreateRun(ctx context.Context, x *types.AgentRun) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO agent_runs
				(tenant_id, agent_id, trigger_id, triggered_by, triggered_user,
				 status, input_payload, output_payload, error_message,
				 steps_count, tokens_used, cost_micros, started_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at, duration_ms
		`, x.TenantID, x.AgentID, x.TriggerID, x.TriggeredBy, x.TriggeredUser,
			x.Status, x.InputPayload, x.OutputPayload, x.ErrorMessage,
			x.StepsCount, x.TokensUsed, x.CostMicros, x.StartedAt).
			Scan(x).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_runs
			(tenant_id, agent_id, trigger_id, triggered_by, triggered_user,
			 status, input_payload, output_payload, error_message,
			 steps_count, tokens_used, cost_micros, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, x.TenantID, x.AgentID, x.TriggerID, x.TriggeredBy, x.TriggeredUser,
		x.Status, x.InputPayload, x.OutputPayload, x.ErrorMessage,
		x.StepsCount, x.TokensUsed, x.CostMicros, x.StartedAt).Error
}

func (r *agentStudioRepository) GetRun(ctx context.Context, tenantID uint64, id uint64) (*types.AgentRun, error) {
	var x types.AgentRun
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, agent_id, trigger_id, triggered_by, triggered_user,
		        status, input_payload, output_payload, error_message,
		        steps_count, tokens_used, cost_micros, started_at, finished_at, duration_ms, created_at
		 FROM agent_runs WHERE tenant_id = ? AND id = ?`,
		tenantID, id,
	).Scan(&x).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &x, nil
}

func (r *agentStudioRepository) ListRunsByAgent(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentRun, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []*types.AgentRun
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, agent_id, trigger_id, triggered_by, triggered_user,
		        status, input_payload, output_payload, error_message,
		        steps_count, tokens_used, cost_micros, started_at, finished_at, duration_ms, created_at
		 FROM agent_runs WHERE tenant_id = ? AND agent_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		tenantID, agentID, limit, offset,
	).Scan(&out).Error
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM agent_runs WHERE tenant_id = ? AND agent_id = ?`,
		tenantID, agentID,
	).Scan(&total).Error; err != nil {
		logger.Warnf(ctx, "[AgentStudio] count failed: %v", err)
	}
	return out, total, nil
}

func (r *agentStudioRepository) UpdateRun(ctx context.Context, x *types.AgentRun) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE agent_runs SET
			status = ?, output_payload = ?, error_message = ?,
			steps_count = ?, tokens_used = ?, cost_micros = ?,
			started_at = ?, finished_at = ?, duration_ms = ?
		WHERE tenant_id = ? AND id = ?`,
		x.Status, x.OutputPayload, x.ErrorMessage,
		x.StepsCount, x.TokensUsed, x.CostMicros,
		x.StartedAt, x.FinishedAt, x.DurationMs, x.TenantID, x.ID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("agent_runs: no row updated for id=%d", x.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Credential Vault
// ---------------------------------------------------------------------------

func (r *agentStudioRepository) CreateCredential(ctx context.Context, c *types.AgentCredential) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO agent_credentials
				(tenant_id, name, credential_type, ciphertext, nonce, auth_tag,
				 enc_meta, created_by, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at, updated_at
		`, c.TenantID, c.Name, c.CredentialType, c.Ciphertext, c.Nonce, c.AuthTag,
			c.EncMeta, c.CreatedBy, c.ExpiresAt).
			Scan(c).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_credentials
			(tenant_id, name, credential_type, ciphertext, nonce, auth_tag,
			 enc_meta, created_by, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.TenantID, c.Name, c.CredentialType, c.Ciphertext, c.Nonce, c.AuthTag,
		c.EncMeta, c.CreatedBy, c.ExpiresAt).Error
}

func (r *agentStudioRepository) GetCredential(ctx context.Context, tenantID uint64, name string) (*types.AgentCredential, error) {
	var c types.AgentCredential
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, credential_type, ciphertext, nonce, auth_tag,
		        enc_meta, created_by, last_used_at, expires_at, created_at, updated_at
		 FROM agent_credentials WHERE tenant_id = ? AND name = ?`,
		tenantID, name,
	).Scan(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *agentStudioRepository) ListCredentials(ctx context.Context, tenantID uint64) ([]*types.AgentCredential, error) {
	var out []*types.AgentCredential
	// Strip ciphertext from the wire — caller can request it explicitly.
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, credential_type, enc_meta,
		        created_by, last_used_at, expires_at, created_at, updated_at
		 FROM agent_credentials WHERE tenant_id = ? ORDER BY id`,
		tenantID,
	).Scan(&out).Error
	return out, err
}

func (r *agentStudioRepository) DeleteCredential(ctx context.Context, tenantID uint64, name string) error {
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM agent_credentials WHERE tenant_id = ? AND name = ?`, tenantID, name,
	).Error
}

func (r *agentStudioRepository) TouchCredentialUsage(ctx context.Context, tenantID uint64, name string) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE agent_credentials SET last_used_at = NOW() WHERE tenant_id = ? AND name = ?`,
		tenantID, name,
	).Error
}

// ---------------------------------------------------------------------------
// Credit Ledger
// ---------------------------------------------------------------------------

func (r *agentStudioRepository) AppendLedger(ctx context.Context, e *types.AgentCreditLedgerEntry) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO agent_credit_ledger
				(tenant_id, agent_id, run_id, operation, unit,
				 quantity, balance_after, policy_version, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, e.TenantID, e.AgentID, e.RunID, e.Operation, e.Unit,
			e.Quantity, e.BalanceAfter, e.PolicyVersion, e.Notes).
			Scan(e).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_credit_ledger
			(tenant_id, agent_id, run_id, operation, unit,
			 quantity, balance_after, policy_version, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.TenantID, e.AgentID, e.RunID, e.Operation, e.Unit,
		e.Quantity, e.BalanceAfter, e.PolicyVersion, e.Notes).Error
}

func (r *agentStudioRepository) SumChargesSince(ctx context.Context, tenantID uint64, agentID string, unit string, since time.Time) (int64, error) {
	var sum sql.NullInt64
	err := r.db.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(quantity), 0) FROM agent_credit_ledger
		 WHERE tenant_id = ? AND agent_id = ? AND unit = ? AND operation IN ('charge','refund') AND created_at >= ?`,
		tenantID, agentID, unit, since,
	).Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	if !sum.Valid {
		return 0, nil
	}
	return sum.Int64, nil
}

func (r *agentStudioRepository) CountInvocationsSince(ctx context.Context, tenantID uint64, agentID string, since time.Time) (int64, error) {
	var count sql.NullInt64
	err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM agent_credit_ledger
		 WHERE tenant_id = ? AND agent_id = ? AND unit = 'invocations' AND operation = 'charge' AND created_at >= ?`,
		tenantID, agentID, since,
	).Scan(&count).Error
	if err != nil {
		return 0, err
	}
	if !count.Valid {
		return 0, nil
	}
	return count.Int64, nil
}

// ---------------------------------------------------------------------------
// Quota Policies
// ---------------------------------------------------------------------------

func (r *agentStudioRepository) CreatePolicy(ctx context.Context, p *types.AgentQuotaPolicy) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO agent_quota_policies
				(tenant_id, name, version, monthly_tokens, daily_invocations,
				 per_run_cost_cap_micros, per_agent_concurrency, is_active, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, p.TenantID, p.Name, p.Version, p.MonthlyTokens, p.DailyInvocations,
			p.PerRunCostCapMicros, p.PerAgentConcurrency, p.IsActive, p.CreatedBy).
			Scan(p).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO agent_quota_policies
			(tenant_id, name, version, monthly_tokens, daily_invocations,
			 per_run_cost_cap_micros, per_agent_concurrency, is_active, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.TenantID, p.Name, p.Version, p.MonthlyTokens, p.DailyInvocations,
		p.PerRunCostCapMicros, p.PerAgentConcurrency, p.IsActive, p.CreatedBy).Error
}

func (r *agentStudioRepository) GetActivePolicy(ctx context.Context, tenantID uint64) (*types.AgentQuotaPolicy, error) {
	var p types.AgentQuotaPolicy
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, version, monthly_tokens, daily_invocations,
		        per_run_cost_cap_micros, per_agent_concurrency, is_active, created_by, created_at
		 FROM agent_quota_policies
		 WHERE tenant_id = ? AND is_active = TRUE
		 ORDER BY version DESC LIMIT 1`,
		tenantID,
	).Scan(&p).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *agentStudioRepository) ListPolicies(ctx context.Context, tenantID uint64) ([]*types.AgentQuotaPolicy, error) {
	var out []*types.AgentQuotaPolicy
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, version, monthly_tokens, daily_invocations,
		        per_run_cost_cap_micros, per_agent_concurrency, is_active, created_by, created_at
		 FROM agent_quota_policies WHERE tenant_id = ? ORDER BY version DESC`,
		tenantID,
	).Scan(&out).Error
	return out, err
}

func (r *agentStudioRepository) ActivatePolicy(ctx context.Context, tenantID uint64, name string, version int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Deactivate siblings — only one active per (tenant, name) family.
		if err := tx.Exec(
			`UPDATE agent_quota_policies SET is_active = FALSE
			 WHERE tenant_id = ? AND name = ?`,
			tenantID, name,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			`UPDATE agent_quota_policies SET is_active = TRUE
			 WHERE tenant_id = ? AND name = ? AND version = ?`,
			tenantID, name, version,
		).Error
	})
}
