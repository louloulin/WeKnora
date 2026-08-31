// Package webhooks — Build #46.x outbound webhook delivery service.
//
// Owns the CRUD over the webhooks table plus the dispatch path:
// when something emits a typed event (e.g. collab.doc.created), the
// service looks up every active subscription that registered for
// that event, signs each payload with HMAC-SHA256 using the
// subscription's secret, and hands the body to a WebhookDispatcher.
//
// The dispatcher interface is the seam for tests: production wires
// an HTTPS POST, tests inject an in-memory recorder. See
// webhookDispatcher below for the production impl.
package webhooks

import (
	"context"
	"fmt"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Maximum dispatch attempts before a delivery is marked failed.
const maxAttempts = 3

// deliveryTimeout bounds each HTTPS attempt.
const deliveryTimeout = 10 * time.Second

// WebhookService is the application-level facade.
type WebhookService struct {
	repo      interfaces.WebhookRepository
	dispatcher interfaces.WebhookDispatcher
	now       func() time.Time
	newID     func() string
}

// NewWebhookService wires the service. The dispatcher may be nil —
// a default HTTPS dispatcher is created on first Publish call.
func NewWebhookService(repo interfaces.WebhookRepository, dispatcher interfaces.WebhookDispatcher) *WebhookService {
	s := &WebhookService{
		repo:       repo,
		dispatcher: dispatcher,
		now:        time.Now,
		newID:      newID,
	}
	if s.dispatcher == nil {
		s.dispatcher = &httpWebhookDispatcher{client: &http.Client{Timeout: deliveryTimeout}}
	}
	return s
}

// SetClock lets tests inject a deterministic clock. Returns the
// service for chaining.
func (s *WebhookService) SetClock(now func() time.Time) *WebhookService {
	s.now = now
	return s
}

// SetIDFunc lets tests inject a deterministic ID generator.
func (s *WebhookService) SetIDFunc(f func() string) *WebhookService {
	s.newID = f
	return s
}

// --- CRUD ---

func (s *WebhookService) Create(ctx context.Context, tenantID, userID uint64, req *types.CreateWebhookRequest) (*types.Webhook, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	h := &types.Webhook{
		ID:        s.newID(),
		TenantID:  tenantID,
		Name:      req.Name,
		URL:       req.URL,
		Events:    req.Events,
		Secret:    req.Secret,
		Active:    active,
		CreatedBy: userID,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	}
	if err := s.repo.Create(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

func (s *WebhookService) Get(ctx context.Context, tenantID uint64, id string) (*types.Webhook, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *WebhookService) List(ctx context.Context, tenantID uint64, filter types.ListWebhooksFilter) ([]*types.Webhook, error) {
	return s.repo.List(ctx, tenantID, filter)
}

func (s *WebhookService) Update(ctx context.Context, tenantID uint64, id string, patch types.UpdateWebhookRequest) (*types.Webhook, error) {
	h, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		h.Name = *patch.Name
	}
	if patch.URL != nil {
		h.URL = *patch.URL
	}
	if patch.Events != nil {
		for _, e := range *patch.Events {
			if !types.ValidWebhookEvents[e] {
				return nil, fmt.Errorf("webhook: unknown event %s", e)
			}
		}
		h.Events = *patch.Events
	}
	if patch.Active != nil {
		h.Active = *patch.Active
	}
	if patch.Secret != nil && *patch.Secret != "" {
		if len(*patch.Secret) < 16 {
			return nil, fmt.Errorf("webhook: secret must be at least 16 chars")
		}
		h.Secret = *patch.Secret
	}
	h.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

func (s *WebhookService) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.Delete(ctx, tenantID, id)
}

// --- Event emission ---

// PublishEvent enqueues a delivery for every active subscription that
// has registered for the given event. PublishEvent is non-blocking:
// the actual HTTP work happens on the goroutine returned by
// Dispatch — callers should `go service.PublishEvent(...)` if they
// care about latency.
func (s *WebhookService) PublishEvent(ctx context.Context, tenantID uint64, event types.WebhookEvent, payload map[string]any) error {
	if !types.ValidWebhookEvents[event] {
		return fmt.Errorf("webhook: unknown event %s", event)
	}
	hooks, err := s.repo.ListByEvent(ctx, tenantID, event)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"event":      event,
		"tenant_id":  tenantID,
		"delivered":  s.now().UTC().Format(time.RFC3339),
		"data":       payload,
	})
	if err != nil {
		return err
	}
	for _, h := range hooks {
		if !h.Active {
			continue
		}
		delivery := &types.WebhookDelivery{
			ID:        s.newID(),
			WebhookID: h.ID,
			TenantID:  tenantID,
			Event:     event,
			Payload:   types.JSON(body),
			Status:    types.WebhookDeliveryStatusPending,
			CreatedAt: s.now(),
			UpdatedAt: s.now(),
		}
		if err := s.repo.CreateDelivery(ctx, delivery); err != nil {
			logger.Warnf(ctx, "[webhook] enqueue delivery failed: %v", err)
			continue
		}
		go s.dispatchOne(h, delivery, body)
	}
	return nil
}

func (s *WebhookService) dispatchOne(h *types.Webhook, delivery *types.WebhookDelivery, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()
	res := s.dispatcher.Dispatch(ctx, h, delivery.Event, body)
	now := s.now()
	delivery.Attempts++
	delivery.UpdatedAt = now
	delivery.ResponseCode = res.StatusCode
	if len(res.Body) > 2048 {
		res.Body = res.Body[:2048]
	}
	delivery.ResponseBody = res.Body

	success := res.Error == nil && res.StatusCode >= 200 && res.StatusCode < 300
	if success {
		delivery.Status = types.WebhookDeliveryStatusSuccess
		delivery.DeliveredAt = &now
		h.LastDeliveryAt = &now
		h.LastError = ""
	} else {
		errMsg := ""
		if res.Error != nil {
			errMsg = res.Error.Error()
		} else {
			errMsg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		if delivery.Attempts >= maxAttempts {
			delivery.Status = types.WebhookDeliveryStatusFailed
			h.LastError = errMsg
		} else {
			// Schedule next attempt with exponential backoff.
			next := now.Add(backoffFor(delivery.Attempts))
			delivery.NextRetryAt = &next
			h.LastError = errMsg
		}
	}

	if err := s.repo.UpdateDelivery(ctx, delivery); err != nil {
		logger.Warnf(ctx, "[webhook] update delivery failed: %v", err)
	}
	if success || delivery.Status == types.WebhookDeliveryStatusFailed {
		if err := s.repo.Update(ctx, h); err != nil {
			logger.Warnf(ctx, "[webhook] update hook last_delivery_at failed: %v", err)
		}
	}
}

func backoffFor(attempts int) time.Duration {
	switch attempts {
	case 1:
		return 1 * time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// --- HTTP dispatcher (production) ---

type httpWebhookDispatcher struct {
	client *http.Client
}

func (d *httpWebhookDispatcher) Dispatch(ctx context.Context, hook *types.Webhook, event types.WebhookEvent, body []byte) interfaces.WebhookDeliveryResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, newReaderFor(body))
	if err != nil {
		return interfaces.WebhookDeliveryResult{Error: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", string(event))
	req.Header.Set("X-Webhook-Delivery", hook.ID)
	if hook.Secret != "" {
		req.Header.Set("X-Webhook-Signature", signBody(hook.Secret, body))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return interfaces.WebhookDeliveryResult{Error: err}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return interfaces.WebhookDeliveryResult{StatusCode: resp.StatusCode, Body: string(b)}
}

// signBody returns the hex HMAC-SHA256 signature the receiver uses to
// verify the body originated from us. Format mirrors GitHub's
// X-Hub-Signature-256 so existing webhook-receiver libraries work
// unchanged.
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// newReaderFor avoids an extra allocation per request; body is
// already a fully buffered slice.
func newReaderFor(b []byte) *bytesReader { return &bytesReader{b: b} }

type bytesReader struct {
	b   []byte
	off int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

// --- helpers ---

var counterMu sync.Mutex
var counter int64

func newID() string {
	counterMu.Lock()
	counter++
	n := counter
	counterMu.Unlock()
	return fmt.Sprintf("whk-%d-%d", time.Now().UnixNano(), n)
}

// --- delivery inspection ---

func (s *WebhookService) ListDeliveriesByHook(ctx context.Context, webhookID string, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListDeliveriesByHook(ctx, webhookID, limit)
}

func (s *WebhookService) ListDeliveriesByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.WebhookDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListDeliveriesByTenant(ctx, tenantID, limit)
}
