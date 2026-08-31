//go:build dlpauthz

package authzadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubAuthZRepo is an in-memory implementation of typesRepo for unit tests.
type stubAuthZRepo struct {
	rows     map[uint64]*types.AuthZPolicyVersion
	byKey    map[string][]*types.AuthZPolicyVersion
	seqRow   uint64
	modified map[uint64]bool // tracks any post-publish mutation for immutability test
}

func newStubAuthZRepo() *stubAuthZRepo {
	return &stubAuthZRepo{
		rows:     map[uint64]*types.AuthZPolicyVersion{},
		byKey:    map[string][]*types.AuthZPolicyVersion{},
		modified: map[uint64]bool{},
	}
}

// errImmutable is a sentinel returned when a caller tries to overwrite an
// existing immutable row.
var errImmutable = errors.New("authz_policy_versions rows are immutable")

func (s *stubAuthZRepo) CreateAuthZPolicyVersion(_ context.Context, p *types.AuthZPolicyVersion) error {
	if p.ID != 0 {
		if _, exists := s.rows[p.ID]; exists {
			return errImmutable
		}
	}
	s.seqRow++
	p.ID = s.seqRow
	p.CreatedAt = time.Now()
	s.rows[p.ID] = p
	s.byKey[p.PolicyKey] = append(s.byKey[p.PolicyKey], p)
	return nil
}

func (s *stubAuthZRepo) GetAuthZPolicyVersion(_ context.Context, _, id uint64) (*types.AuthZPolicyVersion, error) {
	if v, ok := s.rows[id]; ok {
		return v, nil
	}
	return nil, nil
}

func (s *stubAuthZRepo) GetLatestAuthZPolicy(_ context.Context, _ uint64, key string) (*types.AuthZPolicyVersion, error) {
	rows := s.byKey[key]
	if len(rows) == 0 {
		return nil, nil
	}
	latest := rows[0]
	for _, r := range rows[1:] {
		if r.Version > latest.Version {
			latest = r
		}
	}
	return latest, nil
}

func (s *stubAuthZRepo) ListAuthZPolicyVersions(_ context.Context, _ uint64, key string) ([]*types.AuthZPolicyVersion, error) {
	rows := s.byKey[key]
	out := make([]*types.AuthZPolicyVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubAuthZRepo) ListAuthZPolicyKeys(_ context.Context, _ uint64) ([]string, error) {
	out := make([]string, 0, len(s.byKey))
	for k := range s.byKey {
		out = append(out, k)
	}
	return out, nil
}

func TestPublishPolicy_VersionBump(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	v1, err := svc.PublishPolicy(ctx, 1, 42, "kb.read", `actor.role == "viewer"`, types.AuthZDecisionAllow, `{}`)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	if v1.Version != 1 {
		t.Fatalf("v1.Version = %d, want 1", v1.Version)
	}

	v2, err := svc.PublishPolicy(ctx, 1, 42, "kb.read", `actor.role == "editor"`, types.AuthZDecisionAllow, `{}`)
	if err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("v2.Version = %d, want 2", v2.Version)
	}
	if v1.ID == v2.ID {
		t.Fatalf("v1 and v2 share ID: %d", v1.ID)
	}
}

func TestPublishPolicy_ImmutablePrevious(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	v1, err := svc.PublishPolicy(ctx, 1, 42, "kb.read", `actor.role == "viewer"`, types.AuthZDecisionAllow, `{}`)
	if err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	// Attempt to overwrite v1 by reusing its ID. The repo must reject.
	err = repo.CreateAuthZPolicyVersion(ctx, &types.AuthZPolicyVersion{
		ID:         v1.ID,
		TenantID:   1,
		PolicyKey:  "kb.read",
		Version:    1,
		Expression: "tampered",
		Decision:   types.AuthZDecisionDeny,
		CreatedBy:  99,
	})
	if err == nil {
		t.Fatalf("expected immutability error on overwrite of v1, got nil")
	}
	if !errors.Is(err, errImmutable) {
		t.Fatalf("expected errImmutable, got %v", err)
	}

	// Sanity: v1 lookup still returns the original expression.
	gv, _ := svc.GetVersion(ctx, 1, v1.ID)
	if gv.Expression != v1.Expression {
		t.Fatalf("v1 expression changed: got %q, want %q", gv.Expression, v1.Expression)
	}
	if gv.Decision != v1.Decision {
		t.Fatalf("v1 decision changed: got %q, want %q", gv.Decision, v1.Decision)
	}
}

func TestGetLatest_CachesResult(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	if _, err := svc.PublishPolicy(ctx, 1, 42, "kb.write", `actor.role == "editor"`, types.AuthZDecisionAllow, `{}`); err != nil {
		t.Fatalf("publish: %v", err)
	}

	first, err := svc.GetLatest(ctx, 1, "kb.write")
	if err != nil {
		t.Fatalf("GetLatest #1: %v", err)
	}
	if first == nil || first.Version != 1 {
		t.Fatalf("GetLatest #1 returned %+v, want v1", first)
	}

	// Repeat call should hit the cache. We simulate a repo hiccup by
	// routing the next read through a getter that errors — cache must
	// still serve the prior result.
	stale := repo.byKey["kb.write"]
	repo.byKey["kb.write"] = nil
	second, err := svc.GetLatest(ctx, 1, "kb.write")
	if err != nil {
		t.Fatalf("GetLatest #2: %v", err)
	}
	if second == nil || second.ID != first.ID {
		t.Fatalf("cache miss: first=%v second=%v", first, second)
	}

	// Restore so the next publish can compute the next version.
	repo.byKey["kb.write"] = stale

	// After publishing again, the cache must be invalidated and the
	// new latest version must surface.
	if _, err := svc.PublishPolicy(ctx, 1, 42, "kb.write", `actor.role == "admin"`, types.AuthZDecisionAllow, `{}`); err != nil {
		t.Fatalf("publish v2: %v", err)
	}
	third, err := svc.GetLatest(ctx, 1, "kb.write")
	if err != nil {
		t.Fatalf("GetLatest #3: %v", err)
	}
	if third == nil {
		t.Fatalf("GetLatest #3 returned nil")
	}
	if third.Version != 2 {
		t.Fatalf("post-publish version = %d, want 2", third.Version)
	}
}

func TestDiff_DetectsChanges(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	v1, _ := svc.PublishPolicy(ctx, 1, 42, "dlp.upload", `actor.role == "viewer"`, types.AuthZDecisionDeny, `{}`)
	v2, _ := svc.PublishPolicy(ctx, 1, 42, "dlp.upload", `actor.role == "editor"`, types.AuthZDecisionAllow, `{}`)

	res, err := svc.Diff(ctx, 1, v1.ID, v2.ID)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !res.ExpressionChanged {
		t.Errorf("expected ExpressionChanged=true")
	}
	if !res.DecisionChanged {
		t.Errorf("expected DecisionChanged=true")
	}
	if res.Summary == "" {
		t.Errorf("expected non-empty summary")
	}
	if res.FromVersion != 1 || res.ToVersion != 2 {
		t.Errorf("got from=%d to=%d, want 1,2", res.FromVersion, res.ToVersion)
	}
}

func TestSimulate_NoPolicyIsDeny(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	got, err := svc.Simulate(ctx, 1, SimulateInput{
		PolicyKey: "kb.unknown",
		Actor:     map[string]any{"role": "admin"},
		Resource:  map[string]any{"type": "knowledge_base"},
		Action:    "read",
	})
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if got != "deny" {
		t.Fatalf("got %q, want deny", got)
	}
}

func TestSimulate_ConditionalPasses(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	if _, err := svc.PublishPolicy(ctx, 1, 42, "dlp.export", `actor.mfa == true`, types.AuthZDecisionConditional, `{}`); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Conditional + missing flag → deny
	got, err := svc.Simulate(ctx, 1, SimulateInput{
		PolicyKey: "dlp.export",
		Actor:     map[string]any{"role": "editor"},
	})
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if got != "deny" {
		t.Fatalf("conditional without flag = %q, want deny", got)
	}

	// Conditional + flag → allow
	got, err = svc.Simulate(ctx, 1, SimulateInput{
		PolicyKey: "dlp.export",
		Actor:     map[string]any{"conditional_pass": true},
	})
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if got != "allow" {
		t.Fatalf("conditional with flag = %q, want allow", got)
	}
}

func TestSimulate_AllowByDefault(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	if _, err := svc.PublishPolicy(ctx, 1, 42, "kb.public", `true`, types.AuthZDecisionAllow, `{}`); err != nil {
		t.Fatalf("publish: %v", err)
	}

	got, err := svc.Simulate(ctx, 1, SimulateInput{PolicyKey: "kb.public", Actor: map[string]any{"role": "guest"}})
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if got != "allow" {
		t.Fatalf("got %q, want allow", got)
	}
}

func TestPublishPolicy_RejectsEmptyKey(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	_, err := svc.PublishPolicy(ctx, 1, 42, "", `x`, types.AuthZDecisionAllow, `{}`)
	if !errors.Is(err, ErrPolicyKeyRequired) {
		t.Fatalf("got %v, want ErrPolicyKeyRequired", err)
	}
}

func TestPublishPolicy_RejectsBadDecision(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	_, err := svc.PublishPolicy(ctx, 1, 42, "kb.read", `x`, "maybe", `{}`)
	if !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("got %v, want ErrInvalidDecision", err)
	}
}

func TestListKeys_Deduplicated(t *testing.T) {
	repo := newStubAuthZRepo()
	svc := NewAuthZAdmin(repo)
	ctx := context.Background()

	_, _ = svc.PublishPolicy(ctx, 1, 1, "kb.read", `a`, types.AuthZDecisionAllow, `{}`)
	_, _ = svc.PublishPolicy(ctx, 1, 1, "kb.read", `b`, types.AuthZDecisionAllow, `{}`)
	_, _ = svc.PublishPolicy(ctx, 1, 1, "kb.write", `c`, types.AuthZDecisionDeny, `{}`)

	keys, err := svc.ListKeys(ctx, 1)
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
}
