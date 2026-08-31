// Package interfaces — Webhook storage + outbound delivery contracts.
//
// WebhookRepository is the persistence primitive for the
// webhooks / webhook_deliveries tables. WebhookDispatcher is the
// abstraction over "deliver this payload to this URL right now" —
// the production implementation POSTs over HTTPS; tests inject an
// in-process recorder so delivery paths can be verified without
// standing up a real receiver.
package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type WebhookRepository interface {
	Create(ctx context.Context, hook *types.Webhook) error
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.Webhook, error)
	List(ctx context.Context, tenantID uint64, filter types.ListWebhooksFilter) ([]*types.Webhook, error)
	Update(ctx context.Context, hook *types.Webhook) error
	Delete(ctx context.Context, tenantID uint64, id string) error

	// ListByEvent returns every active subscription for tenantID that
	// has registered for the given event. Cheap; called once per
	// event from the dispatch path.
	ListByEvent(ctx context.Context, tenantID uint64, event types.WebhookEvent) ([]*types.Webhook, error)

	// Delivery helpers.
	CreateDelivery(ctx context.Context, delivery *types.WebhookDelivery) error
	UpdateDelivery(ctx context.Context, delivery *types.WebhookDelivery) error
	ListPendingDeliveries(ctx context.Context, now time.Time, limit int) ([]*types.WebhookDelivery, error)
	ListDeliveriesByHook(ctx context.Context, webhookID string, limit int) ([]*types.WebhookDelivery, error)
	ListDeliveriesByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.WebhookDelivery, error)
}

// WebhookDeliveryResult is the per-attempt outcome returned by a
// WebhookDispatcher.
type WebhookDeliveryResult struct {
	StatusCode int
	Body       string
	Error      error
}

// WebhookDispatcher delivers a single payload to a single URL. The
// production implementation performs an HMAC-signed HTTPS POST and
// enforces a short per-request timeout. Tests inject an in-memory
// recorder so dispatch can be asserted without networking.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, hook *types.Webhook, event types.WebhookEvent, payload []byte) WebhookDeliveryResult
}



// WebhookService is the application-level contract the handler
// consumes. The concrete implementation lives in
// internal/application/service/webhooks/webhooks.go.
type WebhookService interface {
	Create(ctx context.Context, tenantID, userID uint64, req *types.CreateWebhookRequest) (*types.Webhook, error)
	Get(ctx context.Context, tenantID uint64, id string) (*types.Webhook, error)
	List(ctx context.Context, tenantID uint64, filter types.ListWebhooksFilter) ([]*types.Webhook, error)
	Update(ctx context.Context, tenantID uint64, id string, patch types.UpdateWebhookRequest) (*types.Webhook, error)
	Delete(ctx context.Context, tenantID uint64, id string) error

	ListDeliveriesByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.WebhookDelivery, error)
	ListDeliveriesByHook(ctx context.Context, webhookID string, limit int) ([]*types.WebhookDelivery, error)

	PublishEvent(ctx context.Context, tenantID uint64, event types.WebhookEvent, payload map[string]any) error
}
