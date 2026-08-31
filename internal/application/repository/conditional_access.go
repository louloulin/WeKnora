package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type conditionalAccessRepository struct {
	db *gorm.DB
}

// NewConditionalAccessRepository wires the GORM-backed implementation
// into the DI container. Tests pass an in-memory SQLite handle to
// exercise the full soft-delete + unique-constraint lifecycle.
func NewConditionalAccessRepository(db *gorm.DB) interfaces.ConditionalAccessRepository {
	return &conditionalAccessRepository{db: db}
}

func (r *conditionalAccessRepository) Create(ctx context.Context, p *types.ConditionalAccessPolicy) error {
	now := time.Now().UTC()
	p.UpdatedAt = now
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	// Default action to allow if unset so a malformed create can't
	// mint a "deny everything" policy by accident.
	if p.Action == "" {
		p.Action = types.PolicyActionAllow
	}
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		if condAccessIsUniqueViolation(err) {
			return interfaces.ErrConditionalAccessPolicyExists
		}
		return err
	}
	return nil
}

func (r *conditionalAccessRepository) GetByID(ctx context.Context, tenantID string, id uint64) (*types.ConditionalAccessPolicy, error) {
	var row types.ConditionalAccessPolicy
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, interfaces.ErrConditionalAccessPolicyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *conditionalAccessRepository) ListEnabled(ctx context.Context, tenantID string) ([]*types.ConditionalAccessPolicy, error) {
	var rows []*types.ConditionalAccessPolicy
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND enabled = ? AND deleted_at IS NULL", tenantID, true).
		Order("priority ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *conditionalAccessRepository) ListAll(ctx context.Context, tenantID string, limit, offset int) ([]*types.ConditionalAccessPolicy, int64, error) {
	q := r.db.WithContext(ctx).Model(&types.ConditionalAccessPolicy{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("priority ASC, id ASC")
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var rows []*types.ConditionalAccessPolicy
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *conditionalAccessRepository) Update(ctx context.Context, p *types.ConditionalAccessPolicy) error {
	p.UpdatedAt = time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&types.ConditionalAccessPolicy{}).
		Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", p.TenantID, p.ID).
		Updates(map[string]interface{}{
			"description": p.Description,
			"conditions":  p.Conditions,
			"action":      p.Action,
			"priority":    p.Priority,
			"enabled":     p.Enabled,
			"updated_at":  p.UpdatedAt,
		})
	if res.Error != nil {
		if condAccessIsUniqueViolation(res.Error) {
			return interfaces.ErrConditionalAccessPolicyExists
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return interfaces.ErrConditionalAccessPolicyNotFound
	}
	return nil
}

func (r *conditionalAccessRepository) SoftDelete(ctx context.Context, tenantID string, id uint64) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&types.ConditionalAccessPolicy{}).
		Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).
		Updates(map[string]interface{}{"deleted_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Idempotent — treat already-deleted as success; the
		// handler treats a 0-row outcome as "already gone" and
		// returns 204 either way.
		return nil
	}
	return nil
}

// condAccessIsUniqueViolation inspects the driver error for the unique-constraint
// marker. GORM does not surface a typed error for this so we sniff
// the message; both pg and sqlite are covered.
func condAccessIsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate key") ||
		contains(msg, "UNIQUE constraint failed") ||
		contains(msg, "unique constraint")
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
