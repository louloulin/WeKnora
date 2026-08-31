// Package repository - v0.7.38 Build #46.x Webhook repo tests.
package repository

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newWebhookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&types.Webhook{}, &types.WebhookDelivery{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestWebhookRepoCreateAndGet(t *testing.T) {
	db := newWebhookTestDB(t)
	r := NewWebhookRepository(db)
	h := &types.Webhook{
		ID:        "whk-1",
		TenantID:  42,
		Name:      "kb-sync",
		URL:       "https://example.com/hook",
		Events:    []types.WebhookEvent{types.WebhookEventCollabDocCreated},
		Secret:    "verysecretvalue-1234",
		Active:    true,
		CreatedBy: 7,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := r.Create(nil, h); err != nil { //nolint:staticcheck
		t.Fatalf("create: %v", err)
	}

	got, err := r.GetByID(nil, 42, "whk-1") //nolint:staticcheck
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Name != "kb-sync" {
		t.Fatalf("expected hook with name kb-sync, got %+v", got)
	}
	if len(got.Events) != 1 || got.Events[0] != types.WebhookEventCollabDocCreated {
		t.Fatalf("events round-trip wrong: %+v", got.Events)
	}
}

func TestWebhookRepoListByEvent(t *testing.T) {
	db := newWebhookTestDB(t)
	r := NewWebhookRepository(db)
	tenant := uint64(42)
	for i, ev := range []types.WebhookEvent{
		types.WebhookEventCollabDocCreated,
		types.WebhookEventCollabDocCreated,
		types.WebhookEventCollabCommentAdded,
	} {
		_ = r.Create(nil, &types.Webhook{ //nolint:staticcheck
			ID:        "whk-" + string(rune('a'+i)),
			TenantID:  tenant,
			URL:       "https://x.test/" + string(rune('a'+i)),
			Events:    []types.WebhookEvent{ev},
			Active:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	hooks, err := r.ListByEvent(nil, tenant, types.WebhookEventCollabDocCreated) //nolint:staticcheck
	if err != nil {
		t.Fatalf("list_by_event: %v", err)
	}
	if len(hooks) != 2 {
		t.Fatalf("expected 2 collab.doc.created subscriptions, got %d", len(hooks))
	}
}

func TestWebhookRepoDeliveryLifecycle(t *testing.T) {
	db := newWebhookTestDB(t)
	r := NewWebhookRepository(db)
	tenant := uint64(1)
	hook := &types.Webhook{
		ID: "whk-d", TenantID: tenant, URL: "https://x.test",
		Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated},
		Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := r.Create(nil, hook); err != nil { //nolint:staticcheck
		t.Fatalf("create hook: %v", err)
	}

	delivery := &types.WebhookDelivery{
		ID:        "del-1",
		WebhookID: hook.ID,
		TenantID:  tenant,
		Event:     types.WebhookEventCollabDocCreated,
		Payload:   types.JSON(`{"event":"collab.doc.created"}`),
		Status:    types.WebhookDeliveryStatusPending,
		Attempts:  1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := r.CreateDelivery(nil, delivery); err != nil { //nolint:staticcheck
		t.Fatalf("create delivery: %v", err)
	}

	next := time.Now().Add(time.Minute)
	delivery.Status = types.WebhookDeliveryStatusPending
	delivery.NextRetryAt = &next
	delivery.ResponseCode = 503
	delivery.ResponseBody = "service unavailable"
	if err := r.UpdateDelivery(nil, delivery); err != nil { //nolint:staticcheck
		t.Fatalf("update delivery: %v", err)
	}

	pending, err := r.ListPendingDeliveries(nil, time.Now().Add(time.Hour), 50) //nolint:staticcheck
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "del-1" {
		t.Fatalf("expected 1 pending delivery, got %+v", pending)
	}
}

func TestWebhookRepoDelete(t *testing.T) {
	db := newWebhookTestDB(t)
	r := NewWebhookRepository(db)
	tenant := uint64(1)
	if err := r.Create(nil, &types.Webhook{ //nolint:staticcheck
		ID: "del-hook", TenantID: tenant, URL: "https://x.test",
		Events: []types.WebhookEvent{types.WebhookEventCollabDocCreated},
		Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Delete(nil, tenant, "del-hook"); err != nil { //nolint:staticcheck
		t.Fatalf("delete: %v", err)
	}
	got, err := r.GetByID(nil, tenant, "del-hook") //nolint:staticcheck
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}
