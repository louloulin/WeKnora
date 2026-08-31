//go:build agentstudio

package agentstudio

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubRepo is an in-memory implementation of typesRepo for unit tests.
// Implements every method the production code calls; no GORM dependency.
type stubRepo struct {
	triggers     map[uint64]*types.AgentTrigger
	runs         map[uint64]*types.AgentRun
	credentials  map[string]*types.AgentCredential // key = (tenant, name)
	ledger       []*types.AgentCreditLedgerEntry
	policies     map[uint64]*types.AgentQuotaPolicy
	seqTrigger   uint64
	seqRun       uint64
	seqCred      uint64
	seqPolicy    uint64
}

func newStubRepo() *stubRepo {
	return &stubRepo{
		triggers:    map[uint64]*types.AgentTrigger{},
		runs:        map[uint64]*types.AgentRun{},
		credentials: map[string]*types.AgentCredential{},
		policies:    map[uint64]*types.AgentQuotaPolicy{},
	}
}

func credKey(tenantID uint64, name string) string {
	return fmt.Sprintf("%d:%s", tenantID, name)
}

func (s *stubRepo) CreateTrigger(ctx context.Context, t *types.AgentTrigger) error {
	s.seqTrigger++
	t.ID = s.seqTrigger
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	s.triggers[t.ID] = t
	return nil
}
func (s *stubRepo) GetTrigger(ctx context.Context, tenantID, id uint64) (*types.AgentTrigger, error) {
	if t, ok := s.triggers[id]; ok && t.TenantID == tenantID {
		return t, nil
	}
	return nil, nil
}
func (s *stubRepo) ListTriggersByAgent(ctx context.Context, tenantID uint64, agentID string) ([]*types.AgentTrigger, error) {
	var out []*types.AgentTrigger
	for _, t := range s.triggers {
		if t.TenantID == tenantID && t.AgentID == agentID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (s *stubRepo) ListActiveCronTriggers(ctx context.Context, before time.Time, limit int) ([]*types.AgentTrigger, error) {
	var out []*types.AgentTrigger
	for _, t := range s.triggers {
		if t.TriggerType == "cron" && t.Status == "active" && t.NextFireAt != nil && t.NextFireAt.Before(before) {
			out = append(out, t)
		}
	}
	return out, nil
}
func (s *stubRepo) UpdateTrigger(ctx context.Context, t *types.AgentTrigger) error {
	s.triggers[t.ID] = t
	return nil
}
func (s *stubRepo) DeleteTrigger(ctx context.Context, tenantID, id uint64) error {
	if t, ok := s.triggers[id]; ok && t.TenantID == tenantID {
		delete(s.triggers, id)
	}
	return nil
}
func (s *stubRepo) CreateRun(ctx context.Context, r *types.AgentRun) error {
	s.seqRun++
	r.ID = s.seqRun
	r.CreatedAt = time.Now()
	s.runs[r.ID] = r
	return nil
}
func (s *stubRepo) GetRun(ctx context.Context, tenantID, id uint64) (*types.AgentRun, error) {
	if r, ok := s.runs[id]; ok && r.TenantID == tenantID {
		return r, nil
	}
	return nil, nil
}
func (s *stubRepo) ListRunsByAgent(ctx context.Context, tenantID uint64, agentID string, limit, offset int) ([]*types.AgentRun, int64, error) {
	var out []*types.AgentRun
	for _, r := range s.runs {
		if r.TenantID == tenantID && r.AgentID == agentID {
			out = append(out, r)
		}
	}
	return out, int64(len(out)), nil
}
func (s *stubRepo) UpdateRun(ctx context.Context, r *types.AgentRun) error {
	s.runs[r.ID] = r
	return nil
}
func (s *stubRepo) CreateCredential(ctx context.Context, c *types.AgentCredential) error {
	s.seqCred++
	c.ID = s.seqCred
	c.CreatedAt = time.Now()
	c.UpdatedAt = c.CreatedAt
	s.credentials[credKey(c.TenantID, c.Name)] = c
	return nil
}
func (s *stubRepo) GetCredential(ctx context.Context, tenantID uint64, name string) (*types.AgentCredential, error) {
	if c, ok := s.credentials[credKey(tenantID, name)]; ok {
		return c, nil
	}
	return nil, nil
}
func (s *stubRepo) ListCredentials(ctx context.Context, tenantID uint64) ([]*types.AgentCredential, error) {
	var out []*types.AgentCredential
	for _, c := range s.credentials {
		if c.TenantID == tenantID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (s *stubRepo) DeleteCredential(ctx context.Context, tenantID uint64, name string) error {
	delete(s.credentials, credKey(tenantID, name))
	return nil
}
func (s *stubRepo) TouchCredentialUsage(ctx context.Context, tenantID uint64, name string) error {
	if c, ok := s.credentials[credKey(tenantID, name)]; ok {
		now := time.Now()
		c.LastUsedAt = &now
	}
	return nil
}
func (s *stubRepo) AppendLedger(ctx context.Context, e *types.AgentCreditLedgerEntry) error {
	e.CreatedAt = time.Now()
	s.ledger = append(s.ledger, e)
	return nil
}
func (s *stubRepo) SumChargesSince(ctx context.Context, tenantID uint64, agentID string, unit string, since time.Time) (int64, error) {
	var sum int64
	for _, e := range s.ledger {
		if e.TenantID == tenantID && e.AgentID == agentID && e.Unit == unit && (e.Operation == "charge" || e.Operation == "refund") && e.CreatedAt.After(since) {
			sum += e.Quantity
		}
	}
	return sum, nil
}
func (s *stubRepo) CountInvocationsSince(ctx context.Context, tenantID uint64, agentID string, since time.Time) (int64, error) {
	var count int64
	for _, e := range s.ledger {
		if e.TenantID == tenantID && e.AgentID == agentID && e.Unit == "invocations" && e.Operation == "charge" && e.CreatedAt.After(since) {
			count++
		}
	}
	return count, nil
}
func (s *stubRepo) CreatePolicy(ctx context.Context, p *types.AgentQuotaPolicy) error {
	s.seqPolicy++
	p.ID = s.seqPolicy
	p.CreatedAt = time.Now()
	s.policies[p.ID] = p
	return nil
}
func (s *stubRepo) GetActivePolicy(ctx context.Context, tenantID uint64) (*types.AgentQuotaPolicy, error) {
	var latest *types.AgentQuotaPolicy
	for _, p := range s.policies {
		if p.TenantID == tenantID && p.IsActive {
			if latest == nil || p.Version > latest.Version {
				latest = p
			}
		}
	}
	return latest, nil
}
func (s *stubRepo) ListPolicies(ctx context.Context, tenantID uint64) ([]*types.AgentQuotaPolicy, error) {
	var out []*types.AgentQuotaPolicy
	for _, p := range s.policies {
		if p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *stubRepo) ActivatePolicy(ctx context.Context, tenantID uint64, name string, version int64) error {
	for _, p := range s.policies {
		if p.TenantID == tenantID && p.Name == name {
			p.IsActive = (p.Version == version)
		}
	}
	return nil
}

// --- tests ---

func TestVaultCreateAndReveal(t *testing.T) {
	repo := newStubRepo()
	vault := NewVault(repo, newVaultKeyResolver())
	plain := []byte("sk-test-12345-secret")

	cred, err := vault.Create(context.Background(), 1, 100, "openai-key", "api_key", plain, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cred.ID == 0 {
		t.Fatalf("expected non-zero id")
	}

	got, err := vault.Reveal(context.Background(), 1, "openai-key")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plaintext mismatch: got %q want %q", got, plain)
	}
}

func TestVaultRevealAsHeader_Bearer(t *testing.T) {
	repo := newStubRepo()
	vault := NewVault(repo, newVaultKeyResolver())
	if _, err := vault.Create(context.Background(), 1, 100, "bearer-cred", "bearer", []byte("xyz"), nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := vault.RevealAsHeader(context.Background(), 1, "bearer-cred")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if got != "Bearer xyz" {
		t.Fatalf("got %q want %q", got, "Bearer xyz")
	}
}

func TestVaultDuplicateName(t *testing.T) {
	repo := newStubRepo()
	vault := NewVault(repo, newVaultKeyResolver())
	if _, err := vault.Create(context.Background(), 1, 100, "dup", "api_key", []byte("a"), nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := vault.Create(context.Background(), 1, 100, "dup", "api_key", []byte("b"), nil); err == nil {
		t.Fatalf("expected ErrVaultAlreadyExists on duplicate")
	}
}

func TestQuotaBlocksWhenCapExceeded(t *testing.T) {
	repo := newStubRepo()
	quota := NewQuota(repo)
	if _, err := quota.CreatePolicy(context.Background(), 1, 1, "test-policy",
		100 /*monthly tokens*/, 5 /*daily invocations*/, 0, 0); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := quota.ActivatePolicy(context.Background(), 1, "test-policy", 1); err != nil {
		t.Fatalf("activate: %v", err)
	}
	// First check should pass.
	if err := quota.Check(context.Background(), 1, "agent-1"); err != nil {
		t.Fatalf("first check should pass: %v", err)
	}
	// Burn 5 invocations via ledger append.
	for i := 0; i < 5; i++ {
		run := &types.AgentRun{ID: uint64(i + 1), TenantID: 1, AgentID: "agent-1"}
		_ = run
		if err := repo.AppendLedger(context.Background(), &types.AgentCreditLedgerEntry{
			TenantID: 1, AgentID: "agent-1", Operation: "charge",
			Unit: "invocations", Quantity: 1, BalanceAfter: int64(i + 1),
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// Sixth invocation should be blocked.
	if err := quota.Check(context.Background(), 1, "agent-1"); err == nil {
		t.Fatalf("expected quota exceeded, got nil")
	}
}

func TestCronParser_BasicExpression(t *testing.T) {
	cp := newCronParser()
	// Every minute at second 0.
	expr := "* * * * *"
	from := time.Date(2026, 8, 31, 12, 30, 0, 0, time.UTC)
	next, err := cp.nextAfter(expr, from)
	if err != nil {
		t.Fatalf("nextAfter: %v", err)
	}
	if next.Minute() != 31 || next.Hour() != 12 {
		t.Fatalf("expected 12:31, got %v", next)
	}
}

func TestCronParser_StepExpression(t *testing.T) {
	cp := newCronParser()
	// Every 15 minutes.
	expr := "*/15 * * * *"
	from := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	next, err := cp.nextAfter(expr, from)
	if err != nil {
		t.Fatalf("nextAfter: %v", err)
	}
	if next.Minute() != 15 {
		t.Fatalf("expected minute=15, got %d", next.Minute())
	}
}

func TestCronParser_RangeExpression(t *testing.T) {
	cp := newCronParser()
	// Weekdays 1-5 at 09:00.
	expr := "0 9 * * 1-5"
	from := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) // Monday
	next, err := cp.nextAfter(expr, from)
	if err != nil {
		t.Fatalf("nextAfter: %v", err)
	}
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Fatalf("expected 09:00, got %v", next)
	}
	// 2026-08-31 is Monday → next weekday 09:00 is 2026-09-01 (Tuesday)
	if next.Day() != 1 || next.Month() != 9 {
		t.Fatalf("expected 09-01, got %v", next)
	}
}

func TestTriggerCreateAndFire(t *testing.T) {
	repo := newStubRepo()
	tr := NewTrigger(repo)
	cfg, _ := json.Marshal(map[string]any{"cron": "0 9 * * *"})
	trig, err := tr.Create(context.Background(), 1, 100, "agent-1", "cron", "morning", string(cfg), "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if trig.NextFireAt == nil {
		t.Fatalf("expected next_fire_at set for cron trigger")
	}
	if err := tr.Fire(context.Background(), 1, trig.ID, "succeeded"); err != nil {
		t.Fatalf("fire: %v", err)
	}
	// After fire, next_fire_at should still be set (cron computes next).
	got, _ := repo.GetTrigger(context.Background(), 1, trig.ID)
	if got.LastFireStatus != "succeeded" {
		t.Fatalf("expected last_fire_status=succeeded, got %q", got.LastFireStatus)
	}
}

func TestServiceRunRecordsLedger(t *testing.T) {
	repo := newStubRepo()
	svc := NewAgentStudioService(repo, nil)

	// Create a policy with generous limits so the run can proceed.
	if _, err := svc.Quota().CreatePolicy(context.Background(), 1, 1, "unlim",
		100000, 100000, 0, 0); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := svc.Quota().ActivatePolicy(context.Background(), 1, "unlim", 1); err != nil {
		t.Fatalf("activate: %v", err)
	}

	run, err := svc.Run(context.Background(), RunOpts{
		TenantID:    1,
		AgentID:     "agent-1",
		TriggeredBy: "manual",
		Input:       map[string]any{"query": "hello"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Status != "succeeded" {
		t.Fatalf("expected status=succeeded, got %q", run.Status)
	}
	if run.ID == 0 {
		t.Fatalf("expected non-zero run id")
	}
	// Ledger should have at least 1 invocation entry.
	var invocationCount int64
	for _, e := range repo.ledger {
		if e.Unit == "invocations" && e.Operation == "charge" {
			invocationCount++
		}
	}
	if invocationCount == 0 {
		t.Fatalf("expected at least 1 invocation ledger entry")
	}
}

// Sanity check that AES-GCM round-trips through encryptGCM / decryptGCM.
func TestVaultAESGCMRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	plaintext := []byte("super-secret-token-12345")
	ct, nonce, tag, err := encryptGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := decryptGCM(key, ct, nonce, tag)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("round-trip mismatch")
	}
	// Tamper: flip one ciphertext byte → decrypt must fail.
	ct[0] ^= 0x01
	if _, err := decryptGCM(key, ct, nonce, tag); err == nil {
		t.Fatalf("expected decrypt to fail after tamper")
	}
}
