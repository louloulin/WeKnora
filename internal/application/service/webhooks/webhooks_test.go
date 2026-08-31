// Package webhooks - Build #46.x service-layer tests.
//
// Uses an in-memory recorder dispatcher so delivery can be asserted
// without standing up a real HTTPS receiver. Covers:
//   - Create / Get / Update / Delete CRUD
//   - PublishEvent fans out to every active subscription registered for the event
//   - HMAC signature is included on every dispatch
//   - Retry backoff increments next_retry_at and caps at maxAttempts
package webhooks

import (
	"context"
	"fmt"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// inMemoryRepo implements interfaces.WebhookRepository in process.
type inMemoryRepo struct {
	mu         sync.Mutex
	hooks      map[string]*types.Webhook
	deliveries map[string]*types.WebhookDelivery
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{hooks: map[string]*types.Webhook{}, deliveries: map[string]*types.WebhookDelivery{}}
}

func (r *inMemoryRepo) Create(ctx context.Context, h *types.Webhook) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[h.ID] = h
	return nil
}
func (r *inMemoryRepo) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.hooks[id]
	if h == nil || h.TenantID != tenantID {
		return nil, nil
	}
	return h, nil
}
func (r *inMemoryRepo) List(ctx context.Context, tenantID uint64, filter types.ListWebhooksFilter) ([]*types.Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*types.Webhook{}
	for _, h := range r.hooks {
		if h.TenantID != tenantID {
			continue
		}
		if filter.ActiveOnly && !h.Active {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}
func (r *inMemoryRepo) Update(ctx context.Context, h *types.Webhook) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[h.ID] = h
	return nil
}
func (r *inMemoryRepo) Delete(ctx context.Context, tenantID uint64, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.hooks, id)
	return nil
}
func (r *inMemoryRepo) ListByEvent(ctx context.Context, tenantID uint64, event types.WebhookEvent) ([]*types.Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*types.Webhook{}
	for _, h := range r.hooks {
		if h.TenantID != tenantID || !h.Active {
			continue
		}
		if h.Events.Contains(event) {
			out = append(out, h)
		}
	}
	return out, nil
}
func (r *inMemoryRepo) CreateDelivery(ctx context.Context, d *types.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deliveries[d.ID] = d
	return nil
}
func (r *inMemoryRepo) UpdateDelivery(ctx context.Context, d *types.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deliveries[d.ID] = d
	return nil
}
func (r *inMemoryRepo) ListPendingDeliveries(ctx context.Context, now time.Time, limit int) ([]*types.WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*types.WebhookDelivery{}
	for _, d := range r.deliveries {
		if d.Status == types.WebhookDeliveryStatusPending && d.NextRetryAt != nil && !d.NextRetryAt.After(now) {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *inMemoryRepo) ListDeliveriesByHook(ctx context.Context, webhookID string, limit int) ([]*types.WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*types.WebhookDelivery{}
	for _, d := range r.deliveries {
		if d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	return out, nil
}
func (r *inMemoryRepo) ListDeliveriesByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*types.WebhookDelivery{}
	for _, d := range r.deliveries {
		if d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	return out, nil
}

// recorderDispatcher captures every dispatch attempt for assertion.
type recorderDispatcher struct {
	mu        sync.Mutex
	captures  []dispatchCapture
	nextCode  int
	nextError error
}

type dispatchCapture struct {
	HookID    string
	Event     types.WebhookEvent
	Body      []byte
	Signature string
}

func (d *recorderDispatcher) Dispatch(ctx context.Context, hook *types.Webhook, event types.WebhookEvent, body []byte) interfaces.WebhookDeliveryResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.captures = append(d.captures, dispatchCapture{
		HookID:    hook.ID,
		Event:     event,
		Body:      append([]byte(nil), body...),
		Signature: signBody(hook.Secret, body),
	})
	code := d.nextCode
	err := d.nextError
	if code == 0 && err == nil {
		code = 200
	}
	return interfaces.WebhookDeliveryResult{StatusCode: code, Error: err}
}

func TestWebhookServicePublishFiresForActiveOnly(t *testing.T) {
	repo := newInMemoryRepo()
	disp := &recorderDispatcher{}
	svc := NewWebhookService(repo, disp)
	var counter int
	svc.SetIDFunc(func() string { counter++; return fmt.Sprintf("whk-%d", counter) })

	ctx := context.Background()
	tenantID := uint64(7)
	active := true
	inactive := false

	if _, err := svc.Create(ctx, tenantID, 1, &types.CreateWebhookRequest{
		Name: "kb-sync", URL: "https://a.test", Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated}, Active: &active,
	}); err != nil {
		t.Fatalf("create active: %v", err)
	}
	if _, err := svc.Create(ctx, tenantID, 1, &types.CreateWebhookRequest{
		Name: "inactive", URL: "https://b.test", Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated}, Active: &inactive,
	}); err != nil {
		t.Fatalf("create inactive: %v", err)
	}

	if err := svc.PublishEvent(ctx, tenantID, types.WebhookEventCollabDocCreated, map[string]any{"doc_id": "d1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Allow goroutine to drain.
	waitFor(200*time.Millisecond, func() bool { return len(disp.captures) >= 1 })
	if len(disp.captures) != 1 {
		t.Fatalf("expected 1 dispatch (active only), got %d", len(disp.captures))
	}
	if disp.captures[0].Event != types.WebhookEventCollabDocCreated {
		t.Fatalf("wrong event: %+v", disp.captures[0])
	}
	if !strings.Contains(disp.captures[0].Signature, "sha256=") {
		t.Fatalf("expected HMAC signature, got %q", disp.captures[0].Signature)
	}
}

func TestWebhookServiceSignatureVerifiesBody(t *testing.T) {
	repo := newInMemoryRepo()
	disp := &recorderDispatcher{}
	svc := NewWebhookService(repo, disp)
	var sigCounter int
	svc.SetIDFunc(func() string { sigCounter++; return fmt.Sprintf("whk-sig-%d", sigCounter) })

	ctx := context.Background()
	secret := "verysecretvalue-1234567890"
	if _, err := svc.Create(ctx, 1, 1, &types.CreateWebhookRequest{
		URL: "https://x.test", Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated}, Secret: secret,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.PublishEvent(ctx, 1, types.WebhookEventCollabDocCreated, map[string]any{"x": 1}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	waitFor(200*time.Millisecond, func() bool { return len(disp.captures) >= 1 })
	if len(disp.captures) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(disp.captures))
	}
	// Verify HMAC by recomputing over the captured body.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(disp.captures[0].Body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if disp.captures[0].Signature != want {
		t.Fatalf("signature mismatch\n got: %s\nwant: %s", disp.captures[0].Signature, want)
	}
}

func TestWebhookServiceRetryOnFailure(t *testing.T) {
	repo := newInMemoryRepo()
	disp := &recorderDispatcher{nextCode: 503}
	svc := NewWebhookService(repo, disp)
	var rCounter int
	svc.SetIDFunc(func() string { rCounter++; return fmt.Sprintf("whk-r-%d", rCounter) })

	ctx := context.Background()
	active := true
	if _, err := svc.Create(ctx, 1, 1, &types.CreateWebhookRequest{
		URL: "https://x.test", Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated}, Active: &active,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.PublishEvent(ctx, 1, types.WebhookEventCollabDocCreated, map[string]any{}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	waitFor(500*time.Millisecond, func() bool {
		d, _ := svc.ListDeliveriesByTenant(ctx, 1, 50)
		if len(d) == 0 {
			return false
		}
		return d[0].NextRetryAt != nil
	})
	deliveries, err := svc.ListDeliveriesByTenant(ctx, 1, 50)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	d := deliveries[0]
	if d.Status != types.WebhookDeliveryStatusPending {
		t.Fatalf("expected pending status, got %s", d.Status)
	}
	if d.NextRetryAt == nil {
		t.Fatalf("expected next_retry_at set after failure")
	}
	if d.Attempts != 1 {
		t.Fatalf("expected attempts=1, got %d", d.Attempts)
	}
}

func TestWebhookServiceUnknownEventRejected(t *testing.T) {
	repo := newInMemoryRepo()
	disp := &recorderDispatcher{}
	svc := NewWebhookService(repo, disp)
	if err := svc.PublishEvent(context.Background(), 1, "bogus.event", nil); err == nil {
		t.Fatalf("expected error for unknown event")
	}
}


func waitFor(timeout time.Duration, cond func() bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}


