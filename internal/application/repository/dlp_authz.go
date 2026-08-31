package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// dlpAuthZRepository implements interfaces.DLPAuthZRepository using raw
// SQL with cross-driver branching (Postgres ON CONFLICT / SQLite INSERT
// OR REPLACE). Auto-migrate isn't used for the v0.7.22 tables because
// they were added after the initial schema.
type dlpAuthZRepository struct {
	db *gorm.DB
}

// NewDLPAuthZRepository wires the repo to the shared *gorm.DB.
func NewDLPAuthZRepository(db *gorm.DB) *dlpAuthZRepository {
	return &dlpAuthZRepository{db: db}
}

// dialectFor returns "postgres" or "sqlite" based on the registered
// driver. Falls back to sqlite to keep dev profiles working.
func (r *dlpAuthZRepository) dialectFor() string {
	if r.db.Dialector != nil {
		name := r.db.Dialector.Name()
		if strings.Contains(name, "postgres") {
			return "postgres"
		}
	}
	return "sqlite"
}

// ---------------------------------------------------------------------------
// DLP policies
// ---------------------------------------------------------------------------

func (r *dlpAuthZRepository) CreateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO dlp_policies
				(tenant_id, name, version, resource_scope, severity, action, is_active,
				 description, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, p.TenantID, p.Name, p.Version, p.ResourceScope, p.Severity, p.Action, p.IsActive,
			p.Description, p.CreatedBy).
			Scan(p).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO dlp_policies
			(tenant_id, name, version, resource_scope, severity, action, is_active,
			 description, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.TenantID, p.Name, p.Version, p.ResourceScope, p.Severity, p.Action, p.IsActive,
		p.Description, p.CreatedBy).Error
}

func (r *dlpAuthZRepository) GetDLPPolicy(ctx context.Context, tenantID uint64, id uint64) (*types.DLPPolicy, error) {
	var p types.DLPPolicy
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, version, resource_scope, severity, action, is_active,
		        description, created_by, created_at
		 FROM dlp_policies WHERE tenant_id = ? AND id = ?`,
		tenantID, id,
	).Scan(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *dlpAuthZRepository) ListDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error) {
	var out []*types.DLPPolicy
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, version, resource_scope, severity, action, is_active,
		        description, created_by, created_at
		 FROM dlp_policies WHERE tenant_id = ? ORDER BY name, version DESC`,
		tenantID,
	).Scan(&out).Error
	return out, err
}

func (r *dlpAuthZRepository) ListActiveDLPPolicies(ctx context.Context, tenantID uint64) ([]*types.DLPPolicy, error) {
	var out []*types.DLPPolicy
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, name, version, resource_scope, severity, action, is_active,
		        description, created_by, created_at
		 FROM dlp_policies WHERE tenant_id = ? AND is_active = TRUE ORDER BY name`,
		tenantID,
	).Scan(&out).Error
	return out, err
}

func (r *dlpAuthZRepository) UpdateDLPPolicy(ctx context.Context, p *types.DLPPolicy) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE dlp_policies SET
			resource_scope = ?, severity = ?, action = ?, is_active = ?,
			description = ?
		WHERE tenant_id = ? AND id = ?`,
		p.ResourceScope, p.Severity, p.Action, p.IsActive, p.Description, p.TenantID, p.ID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("dlp_policies: no row updated for id=%d", p.ID)
	}
	return nil
}

func (r *dlpAuthZRepository) NextDLPPolicyVersion(ctx context.Context, tenantID uint64, name string) (int64, error) {
	var max sql.NullInt64
	err := r.db.WithContext(ctx).Raw(
		`SELECT MAX(version) FROM dlp_policies WHERE tenant_id = ? AND name = ?`,
		tenantID, name,
	).Scan(&max).Error
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 1, nil
	}
	return max.Int64 + 1, nil
}

// ---------------------------------------------------------------------------
// DLP rules
// ---------------------------------------------------------------------------

func (r *dlpAuthZRepository) CreateDLPRule(ctx context.Context, x *types.DLPRule) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO dlp_rules
				(policy_id, tenant_id, pattern_type, pattern_value, severity, enabled, description)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, x.PolicyID, x.TenantID, x.PatternType, x.PatternValue, x.Severity, x.Enabled, x.Description).
			Scan(x).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO dlp_rules
			(policy_id, tenant_id, pattern_type, pattern_value, severity, enabled, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, x.PolicyID, x.TenantID, x.PatternType, x.PatternValue, x.Severity, x.Enabled, x.Description).Error
}

func (r *dlpAuthZRepository) ListDLPRulesByPolicy(ctx context.Context, tenantID uint64, policyID uint64) ([]*types.DLPRule, error) {
	var out []*types.DLPRule
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, policy_id, tenant_id, pattern_type, pattern_value, severity, enabled,
		        description, created_at
		 FROM dlp_rules WHERE tenant_id = ? AND policy_id = ? ORDER BY id`,
		tenantID, policyID,
	).Scan(&out).Error
	return out, err
}

func (r *dlpAuthZRepository) DeleteDLPRule(ctx context.Context, tenantID uint64, ruleID uint64) error {
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM dlp_rules WHERE tenant_id = ? AND id = ?`, tenantID, ruleID,
	).Error
}

// ---------------------------------------------------------------------------
// DLP violations
// ---------------------------------------------------------------------------

func (r *dlpAuthZRepository) CreateDLPViolation(ctx context.Context, x *types.DLPViolation) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO dlp_violations
				(tenant_id, policy_id, rule_id, resource, resource_id, actor_id,
				 matched_value, context, matched_pattern, action_taken, severity, audit_log_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, x.TenantID, x.PolicyID, x.RuleID, x.Resource, x.ResourceID, x.ActorID,
			x.MatchedValue, x.Context, x.MatchedPattern, x.ActionTaken, x.Severity, x.AuditLogID).
			Scan(x).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO dlp_violations
			(tenant_id, policy_id, rule_id, resource, resource_id, actor_id,
			 matched_value, context, matched_pattern, action_taken, severity, audit_log_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, x.TenantID, x.PolicyID, x.RuleID, x.Resource, x.ResourceID, x.ActorID,
		x.MatchedValue, x.Context, x.MatchedPattern, x.ActionTaken, x.Severity, x.AuditLogID).Error
}

func (r *dlpAuthZRepository) ListDLPViolations(ctx context.Context, tenantID uint64, resource string, limit, offset int) ([]*types.DLPViolation, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []*types.DLPViolation
	q := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, policy_id, rule_id, resource, resource_id, actor_id,
		        matched_value, context, matched_pattern, action_taken, severity,
		        audit_log_id, created_at
		 FROM dlp_violations WHERE tenant_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		tenantID, limit, offset,
	)
	if resource != "" {
		q = r.db.WithContext(ctx).Raw(
			`SELECT id, tenant_id, policy_id, rule_id, resource, resource_id, actor_id,
			        matched_value, context, matched_pattern, action_taken, severity,
			        audit_log_id, created_at
			 FROM dlp_violations WHERE tenant_id = ? AND resource = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
			tenantID, resource, limit, offset,
		)
	}
	if err := q.Scan(&out).Error; err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM dlp_violations WHERE tenant_id = ?`,
		tenantID,
	).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ---------------------------------------------------------------------------
// AuthZ policy versions
// ---------------------------------------------------------------------------

func (r *dlpAuthZRepository) CreateAuthZPolicyVersion(ctx context.Context, x *types.AuthZPolicyVersion) error {
	if r.dialectFor() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO authz_policy_versions
				(tenant_id, policy_key, version, expression, decision, metadata, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			RETURNING id, created_at
		`, x.TenantID, x.PolicyKey, x.Version, x.Expression, x.Decision, x.Metadata, x.CreatedBy).
			Scan(x).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO authz_policy_versions
			(tenant_id, policy_key, version, expression, decision, metadata, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, x.TenantID, x.PolicyKey, x.Version, x.Expression, x.Decision, x.Metadata, x.CreatedBy).Error
}

func (r *dlpAuthZRepository) GetAuthZPolicyVersion(ctx context.Context, tenantID uint64, id uint64) (*types.AuthZPolicyVersion, error) {
	var x types.AuthZPolicyVersion
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, policy_key, version, expression, decision, metadata,
		        created_by, created_at
		 FROM authz_policy_versions WHERE tenant_id = ? AND id = ?`,
		tenantID, id,
	).Scan(&x).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &x, nil
}

func (r *dlpAuthZRepository) GetLatestAuthZPolicy(ctx context.Context, tenantID uint64, policyKey string) (*types.AuthZPolicyVersion, error) {
	var x types.AuthZPolicyVersion
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, policy_key, version, expression, decision, metadata,
		        created_by, created_at
		 FROM authz_policy_versions WHERE tenant_id = ? AND policy_key = ?
		 ORDER BY version DESC LIMIT 1`,
		tenantID, policyKey,
	).Scan(&x).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &x, nil
}

func (r *dlpAuthZRepository) ListAuthZPolicyVersions(ctx context.Context, tenantID uint64, policyKey string) ([]*types.AuthZPolicyVersion, error) {
	var out []*types.AuthZPolicyVersion
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, tenant_id, policy_key, version, expression, decision, metadata,
		        created_by, created_at
		 FROM authz_policy_versions WHERE tenant_id = ? AND policy_key = ?
		 ORDER BY version DESC`,
		tenantID, policyKey,
	).Scan(&out).Error
	return out, err
}

func (r *dlpAuthZRepository) ListAuthZPolicyKeys(ctx context.Context, tenantID uint64) ([]string, error) {
	var out []string
	err := r.db.WithContext(ctx).Raw(
		`SELECT DISTINCT policy_key FROM authz_policy_versions WHERE tenant_id = ? ORDER BY policy_key`,
		tenantID,
	).Scan(&out).Error
	return out, err
}
