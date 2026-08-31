// Package repository — Build #46.x Webhook persistence.
//
// WebhookRepository is the only contract the application service
// consumes; the gorm.DB field is unexported so a future swap to a
// different backing store does not ripple through the package.
//
// Two tables are involved: webhooks (subscription config) and
// webhook_deliveries (per-attempt status). The Events column is
// stored as TEXT (JSON array); the application layer keeps the
// representation consistent through types.MarshalEvents /
// UnmarshalEvents.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type webhookRepository struct {
	db *gorm.DB
}

// NewWebhookRepository wires the repo to the GORM handle.
func NewWebhookRepository(db *gorm.DB) interfaces.WebhookRepository {
	return &webhookRepository{db: db}
}

func (r *webhookRepository) Create(ctx context.Context, hook *types.Webhook) error {
	if hook.ID == "" {
		return errors.New("webhook: id required")
	}
	if hook.TenantID == 0 {
		return errors.New("webhook: tenant_id required")
	}
	now := time.Now().UTC()
	if hook.CreatedAt.IsZero() {
		hook.CreatedAt = now
	}
	hook.UpdatedAt = now
	return r.db.WithContext(ctx).Create(hook).Error
}

func (r *webhookRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Webhook, error) {
	var h types.Webhook
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&h).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("webhook get: %w", err)
	}
	return &h, nil
}

func (r *webhookRepository) List(ctx context.Context, tenantID uint64, filter types.ListWebhooksFilter) ([]*types.Webhook, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if filter.ActiveOnly {
		q = q.Where("active = ?", true)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if filter.AfterID != "" {
		q = q.Where("id > ?", filter.AfterID)
	}
	q = q.Order("created_at DESC").Limit(limit)
	var rows []*types.Webhook
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("webhook list: %w", err)
	}
	return rows, nil
}

func (r *webhookRepository) Update(ctx context.Context, hook *types.Webhook) error {
	hook.UpdatedAt = time.Now().UTC()
	res := r.db.WithContext(ctx).Save(hook)
	return res.Error
}

func (r *webhookRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.Webhook{}).Error
}

func (r *webhookRepository) ListByEvent(ctx context.Context, tenantID uint64, event types.WebhookEvent) ([]*types.Webhook, error) {
	var rows []*types.Webhook
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND active = ?", tenantID, true).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhook list_by_event: %w", err)
	}
	out := make([]*types.Webhook, 0, len(rows))
	for _, h := range rows {
		for _, e := range h.Events {
			if e == event {
				out = append(out, h)
				break
			}
		}
	}
	return out, nil
}

func (r *webhookRepository) CreateDelivery(ctx context.Context, delivery *types.WebhookDelivery) error {
	if delivery.ID == "" {
		return errors.New("webhook_delivery: id required")
	}
	now := time.Now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	delivery.UpdatedAt = now
	return r.db.WithContext(ctx).Create(delivery).Error
}

func (r *webhookRepository) UpdateDelivery(ctx context.Context, delivery *types.WebhookDelivery) error {
	delivery.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Save(delivery).Error
}

func (r *webhookRepository) ListPendingDeliveries(ctx context.Context, now time.Time, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []*types.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?", types.WebhookDeliveryStatusPending, now).
		Order("next_retry_at ASC").Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhook list_pending: %w", err)
	}
	return rows, nil
}

func (r *webhookRepository) ListDeliveriesByHook(ctx context.Context, webhookID string, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []*types.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC").Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhook deliveries by hook: %w", err)
	}
	return rows, nil
}

func (r *webhookRepository) ListDeliveriesByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []*types.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("webhook deliveries by tenant: %w", err)
	}
	return rows, nil
}
