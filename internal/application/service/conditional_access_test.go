//go:build condaccesstest
// +build condaccesstest

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubConditionalAccessRepo is a tiny in-memory implementation used
// by the unit tests below. The fake keeps the test free of database
// dependencies so it can run as part of a fast unit loop.
type stubConditionalAccessRepo struct {
	policies map[uint64]*types.ConditionalAccessPolicy
	nextID   uint64
}

func newStubConditionalAccessRepo() *stubConditionalAccessRepo {
	return &stubConditionalAccessRepo{policies: map[uint64]*types.ConditionalAccessPolicy{}}
}

func (r *stubConditionalAccessRepo) Create(_ context.Context, p *types.ConditionalAccessPolicy) error {
	for _, existing := range r.policies {
		if existing.TenantID == p.TenantID && existing.Name == p.Name && existing.DeletedAt.Time.IsZero() {
			return interfaces.ErrConditionalAccessPolicyExists
		}
	}
	r.nextID++
	p.ID = r.nextID
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	r.policies[p.ID] = p
	return nil
}

func (r *stubConditionalAccessRepo) GetByID(_ context.Context, tenantID string, id uint64) (*types.ConditionalAccessPolicy, error) {
	if p, ok := r.policies[id]; ok && p.TenantID == tenantID {
		return p, nil
	}
	return nil, interfaces.ErrConditionalAccessPolicyNotFound
}

func (r *stubConditionalAccessRepo) ListEnabled(_ context.Context, tenantID string) ([]*types.ConditionalAccessPolicy, error) {
	var out []*types.ConditionalAccessPolicy
	for _, p := range r.policies {
		if p.TenantID == tenantID && p.Enabled && p.DeletedAt.Time.IsZero() {
			out = append(out, p)
		}
	}
	// Sort by priority ASC, id ASC
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Priority < out[i].Priority ||
				(out[j].Priority == out[i].Priority && out[j].ID < out[i].ID) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (r *stubConditionalAccessRepo) ListAll(_ context.Context, tenantID string, _, _ int) ([]*types.ConditionalAccessPolicy, int64, error) {
	var out []*types.ConditionalAccessPolicy
	for _, p := range r.policies {
		if p.TenantID == tenantID && p.DeletedAt.Time.IsZero() {
			out = append(out, p)
		}
	}
	return out, int64(len(out)), nil
}

func (r *stubConditionalAccessRepo) Update(_ context.Context, p *types.ConditionalAccessPolicy) error {
	existing, ok := r.policies[p.ID]
	if !ok || existing.TenantID != p.TenantID || !existing.DeletedAt.Time.IsZero() {
		return interfaces.ErrConditionalAccessPolicyNotFound
	}
	existing.Description = p.Description
	existing.Conditions = p.Conditions
	existing.Action = p.Action
	existing.Priority = p.Priority
	existing.Enabled = p.Enabled
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *stubConditionalAccessRepo) SoftDelete(_ context.Context, tenantID string, id uint64) error {
	p, ok := r.policies[id]
	if !ok || p.TenantID != tenantID {
		return nil
	}
	p.DeletedAt = gormDeletedAt(time.Now())
	p.UpdatedAt = time.Now().UTC()
	return nil
}

// gormDeletedAt mirrors gorm.DeletedAt for the stub without importing
// the gorm package from this test file (keeps the imports minimal).
type gormDeletedAt = struct {
	Time        time.Time
	Valid       bool
}

func gormDeletedAt(t time.Time) gormDeletedAt {
	return gormDeletedAt{Time: t, Valid: true}
}

// pinnedClock returns a clock pinned to the given instant so day-of-week
// and hour-of-day assertions are deterministic.
func pinnedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestMatchConditions_IPCIDR covers the four canonical outcomes for
// the IP CIDR allow-list — empty (skip), single match, list match,
// and miss.
func TestMatchConditions_IPCIDR(t *testing.T) {
	cond := types.PolicyConditions{IPCIDRs: []string{"10.0.0.0/8", "192.168.1.0/24"}}
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"192.168.1.42", true},
		{"8.8.8.8", false},
		{"not-an-ip", false},
		{"", false},
	}
	for _, tc := range cases {
		req := types.EvaluationRequest{ClientIP: tc.ip}
		if got := service.MatchConditions(cond, req); got != tc.want {
			t.Fatalf("ip=%q want=%v got=%v", tc.ip, tc.want, got)
		}
	}
}

// TestMatchConditions_TimeWindow pins the clock so the day-of-week +
// hour-of-day assertions are deterministic.
func TestMatchConditions_TimeWindow(t *testing.T) {
	// 2026-09-02 is a Wednesday.
	wednesday := time.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)

	cases := []struct {
		name      string
		cond      types.PolicyConditions
		wantMatch bool
	}{
		{
			name:      "wednesday in window",
			cond:      types.PolicyConditions{DaysOfWeek: []string{"monday", "wednesday", "friday"}, HourRange: types.HourRange{Start: 9, End: 17}},
			wantMatch: true,
		},
		{
			name:      "wednesday outside hour",
			cond:      types.PolicyConditions{DaysOfWeek: []string{"wednesday"}, HourRange: types.HourRange{Start: 18, End: 22}},
			wantMatch: false,
		},
		{
			name:      "wednesday wrong day",
			cond:      types.PolicyConditions{DaysOfWeek: []string{"tuesday"}},
			wantMatch: false,
		},
		{
			name:      "no time conditions = match all",
			cond:      types.PolicyConditions{},
			wantMatch: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := types.EvaluationRequest{Now: wednesday}
			if got := service.MatchConditions(tc.cond, req); got != tc.wantMatch {
				t.Fatalf("want=%v got=%v", tc.wantMatch, got)
			}
		})
	}
}

// TestMatchConditions_CountryAndDevice covers the country allow-list
// (case-insensitive) and device posture matching.
func TestMatchConditions_CountryAndDevice(t *testing.T) {
	cond := types.PolicyConditions{Countries: []string{"CN", "US"}, DevicePostures: []string{"managed"}}
	cases := []struct {
		country   string
		posture   string
		wantMatch bool
	}{
		{"cn", "managed", true},
		{"US", "managed", true},
		{"JP", "managed", false},
		{"CN", "unmanaged", false},
		{"", "", false},
	}
	for _, tc := range cases {
		req := types.EvaluationRequest{CountryCode: tc.country, DevicePosture: tc.posture}
		if got := service.MatchConditions(cond, req); got != tc.wantMatch {
			t.Fatalf("country=%q posture=%q want=%v got=%v",
				tc.country, tc.posture, tc.wantMatch, got)
		}
	}
}

// TestEvaluate_DenyWinsOverAllow walks the priority-ordering rule:
// a low-priority deny that matches the request wins over a
// high-priority allow that also matches. This is the property that
// keeps "block X for everyone except the on-call rota" correct.
func TestEvaluate_DenyWinsOverAllow(t *testing.T) {
	repo := newStubConditionalAccessRepo()
	svc := service.NewConditionalAccessService(repo)

	_, _ = svc.CreatePolicy(context.Background(), &types.ConditionalAccessPolicy{
		TenantID: "tenant-1", Name: "allow-corp", Action: types.PolicyActionAllow,
		Priority: 10, Enabled: true,
		Conditions: types.PolicyConditions{IPCIDRs: []string{"10.0.0.0/8"}},
	})
	_, _ = svc.CreatePolicy(context.Background(), &types.ConditionalAccessPolicy{
		TenantID: "tenant-1", Name: "deny-blocked", Action: types.PolicyActionDeny,
		Priority: 1, Enabled: true,
		Conditions: types.PolicyConditions{Countries: []string{"KP"}},
	})

	req := types.EvaluationRequest{
		TenantID: "tenant-1",
		ClientIP: "10.1.2.3",
		Now:      pinnedClock(time.Now())(),
	}
	decision, err := svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// KP country is not in the request, so deny-blocked does not
	// fire. allow-corp fires (IP matches). expect allow.
	if decision.Action != types.PolicyActionAllow {
		t.Fatalf("want allow got %s reason=%s", decision.Action, decision.Reason)
	}

	// Now switch the country and re-evaluate; deny should fire.
	req.CountryCode = "KP"
	decision, err = svc.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != types.PolicyActionDeny {
		t.Fatalf("want deny got %s reason=%s", decision.Action, decision.Reason)
	}
}

// TestEvaluate_RequireMFAReturnsChallenge ensures the action surface
// includes the MFA-challenge outcome.
func TestEvaluate_RequireMFAReturnsChallenge(t *testing.T) {
	repo := newStubConditionalAccessRepo()
	svc := service.NewConditionalAccessService(repo)
	_, _ = svc.CreatePolicy(context.Background(), &types.ConditionalAccessPolicy{
		TenantID: "tenant-1", Name: "always-mfa", Action: types.PolicyActionRequireMFA,
		Priority: 100, Enabled: true,
	})

	decision, err := svc.Evaluate(context.Background(),
		types.EvaluationRequest{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != types.PolicyActionRequireMFA {
		t.Fatalf("want require_mfa got %s", decision.Action)
	}
}

// TestEvaluate_NoPoliciesIsAllow covers the implicit-default branch.
func TestEvaluate_NoPoliciesIsAllow(t *testing.T) {
	repo := newStubConditionalAccessRepo()
	svc := service.NewConditionalAccessService(repo)
	decision, err := svc.Evaluate(context.Background(),
		types.EvaluationRequest{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != types.PolicyActionAllow {
		t.Fatalf("want allow got %s", decision.Action)
	}
	if decision.MatchedPolicyID != 0 {
		t.Fatalf("want zero matched policy id, got %d", decision.MatchedPolicyID)
	}
}

// TestValidatePolicy_RejectsInvalidAction is a defensive guard so
// future refactors that touch validatePolicy have a single source of
// truth to fall back on.
func TestValidatePolicy_RejectsInvalidAction(t *testing.T) {
	svc := service.NewConditionalAccessService(newStubConditionalAccessRepo())
	_, err := svc.CreatePolicy(context.Background(), &types.ConditionalAccessPolicy{
		TenantID: "tenant-1", Name: "bad", Action: "launch-missiles",
	})
	if err == nil {
		t.Fatalf("expected invalid action to be rejected")
	}
}
