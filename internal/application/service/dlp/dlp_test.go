//go:build dlpauthz

package dlp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// stubRepo is an in-memory implementation of typesRepo for unit tests.
type stubRepo struct {
	policies   map[uint64]*types.DLPPolicy
	rules      map[uint64]*types.DLPRule
	violations []*types.DLPViolation
	seqPolicy  uint64
	seqRule    uint64
	seqViol    uint64
}

func newStubRepo() *stubRepo {
	return &stubRepo{
		policies: map[uint64]*types.DLPPolicy{},
		rules:    map[uint64]*types.DLPRule{},
	}
}

func (s *stubRepo) CreateDLPPolicy(_ context.Context, p *types.DLPPolicy) error {
	s.seqPolicy++
	p.ID = s.seqPolicy
	p.CreatedAt = time.Now()
	s.policies[p.ID] = p
	return nil
}
func (s *stubRepo) GetDLPPolicy(_ context.Context, _, id uint64) (*types.DLPPolicy, error) {
	if p, ok := s.policies[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (s *stubRepo) ListDLPPolicies(_ context.Context, _ uint64) ([]*types.DLPPolicy, error) {
	var out []*types.DLPPolicy
	for _, p := range s.policies {
		out = append(out, p)
	}
	return out, nil
}
func (s *stubRepo) ListActiveDLPPolicies(_ context.Context, _ uint64) ([]*types.DLPPolicy, error) {
	var out []*types.DLPPolicy
	for _, p := range s.policies {
		if p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *stubRepo) UpdateDLPPolicy(_ context.Context, p *types.DLPPolicy) error {
	s.policies[p.ID] = p
	return nil
}
func (s *stubRepo) NextDLPPolicyVersion(_ context.Context, _ uint64, name string) (int64, error) {
	var max int64
	for _, p := range s.policies {
		if p.Name == name && p.Version > max {
			max = p.Version
		}
	}
	return max + 1, nil
}
func (s *stubRepo) CreateDLPRule(_ context.Context, r *types.DLPRule) error {
	s.seqRule++
	r.ID = s.seqRule
	r.CreatedAt = time.Now()
	s.rules[r.ID] = r
	return nil
}
func (s *stubRepo) ListDLPRulesByPolicy(_ context.Context, _, pid uint64) ([]*types.DLPRule, error) {
	var out []*types.DLPRule
	for _, r := range s.rules {
		if r.PolicyID == pid {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *stubRepo) DeleteDLPRule(_ context.Context, _, id uint64) error {
	delete(s.rules, id)
	return nil
}
func (s *stubRepo) CreateDLPViolation(_ context.Context, v *types.DLPViolation) error {
	s.seqViol++
	v.ID = s.seqViol
	v.CreatedAt = time.Now()
	s.violations = append(s.violations, v)
	return nil
}
func (s *stubRepo) ListDLPViolations(_ context.Context, _ uint64, _ string, limit, offset int) ([]*types.DLPViolation, int64, error) {
	var out []*types.DLPViolation
	for _, v := range s.violations {
		out = append(out, v)
	}
	if offset >= len(out) {
		return nil, int64(len(out)), nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], int64(len(out)), nil
}

// --- tests ---

func TestBuiltinNames_AllRegistered(t *testing.T) {
	for _, name := range BuiltinNames() {
		if _, ok := builtinPatterns[name]; !ok {
			t.Fatalf("builtin name %q has no compiled regex", name)
		}
	}
}

func TestScanner_CreditCard(t *testing.T) {
	rules := []policyRule{{
		PolicyID: 1,
		Action:   "block",
		Rule: types.DLPRule{
			ID: 1, PolicyID: 1, PatternType: "builtin",
			PatternValue: "credit_card", Severity: "high", Enabled: true,
		},
	}}
	sc := newScanner(rules)
	text := "Found a card: 4111 1111 1111 1111 in the wallet. Also 5500000000000004."
	matches := sc.scan(text, 200)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(matches), matches)
	}
}

func TestScanner_EmailAndIP(t *testing.T) {
	rules := []policyRule{
		{PolicyID: 1, Action: "log", Rule: types.DLPRule{
			ID: 1, PatternType: "builtin", PatternValue: "email",
			Severity: "low", Enabled: true,
		}},
		{PolicyID: 1, Action: "log", Rule: types.DLPRule{
			ID: 2, PatternType: "builtin", PatternValue: "ip_addr",
			Severity: "medium", Enabled: true,
		}},
	}
	sc := newScanner(rules)
	text := "Email alice@example.com and IP 192.168.1.1 both detected."
	matches := sc.scan(text, 200)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestScanner_CustomRegex(t *testing.T) {
	rules := []policyRule{{
		PolicyID: 1, Action: "block",
		Rule: types.DLPRule{
			ID: 1, PatternType: "regex",
			PatternValue: `\b[A-Z]{3}-\d{4}\b`,
			Severity: "medium", Enabled: true,
		},
	}}
	sc := newScanner(rules)
	text := "Ticket ABC-1234 is yours. Another: XYZ-9876."
	matches := sc.scan(text, 200)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestScanner_DictionaryCaseInsensitive(t *testing.T) {
	rules := []policyRule{{
		PolicyID: 1, Action: "notify_dpo",
		Rule: types.DLPRule{
			ID: 1, PatternType: "dictionary",
			PatternValue: "Project Phoenix, Q3 Roadmap",
			Severity: "high", Enabled: true,
		},
	}}
	sc := newScanner(rules)
	text := "Mention of project phoenix appears here. Also q3 ROADMAP mentioned."
	matches := sc.scan(text, 200)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestScanner_DisabledRuleSkipped(t *testing.T) {
	rules := []policyRule{{
		PolicyID: 1, Action: "block",
		Rule: types.DLPRule{
			ID: 1, PatternType: "builtin", PatternValue: "credit_card",
			Severity: "high", Enabled: false, // disabled
		},
	}}
	sc := newScanner(rules)
	matches := sc.scan("4111111111111111 should NOT fire", 200)
	if len(matches) != 0 {
		t.Fatalf("disabled rule fired: %d", len(matches))
	}
}

func TestScanner_InvalidRegexSkipped(t *testing.T) {
	rules := []policyRule{
		{PolicyID: 1, Action: "block", Rule: types.DLPRule{
			ID: 1, PatternType: "regex",
			PatternValue: "[unclosed", // invalid
			Severity: "low", Enabled: true,
		}},
		{PolicyID: 1, Action: "block", Rule: types.DLPRule{
			ID: 2, PatternType: "builtin", PatternValue: "email",
			Severity: "low", Enabled: true,
		}},
	}
	sc := newScanner(rules)
	matches := sc.scan("alice@example.com", 200)
	if len(matches) != 1 {
		t.Fatalf("expected only the email match (invalid rule dropped silently), got %d", len(matches))
	}
}

func TestServiceScan_RecordsViolations(t *testing.T) {
	repo := newStubRepo()
	svc := NewDLPScanner(repo)
	// Create a policy with one rule.
	p, err := svc.CreatePolicy(context.Background(), 1, 100,
		"pii-credit-cards", "*", "high", "block", "Block all credit cards")
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := svc.AddRule(context.Background(), 1, p.ID,
		"builtin", "credit_card", "high", "All CC BIN ranges"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	// Activate.
	if err := svc.ActivatePolicy(context.Background(), 1, p.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	// Scan.
	res, err := svc.Scan(context.Background(), ScanInput{
		TenantID:   1,
		Resource:   "wiki_page",
		ResourceID: "page-1",
		ActorID:    100,
		Text:       "Card 4111111111111111 detected.",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(res.Matches))
	}
	if len(res.ViolationIDs) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(res.ViolationIDs))
	}
	if res.ActionCounts["block"] != 1 {
		t.Fatalf("expected 1 block action, got %+v", res.ActionCounts)
	}
}

func TestServiceCreatePolicy_VersionBump(t *testing.T) {
	repo := newStubRepo()
	svc := NewDLPScanner(repo)
	p1, err := svc.CreatePolicy(context.Background(), 1, 100,
		"my-policy", "*", "low", "log", "v1")
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	p2, err := svc.CreatePolicy(context.Background(), 1, 100,
		"my-policy", "*", "low", "log", "v2")
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if p2.Version <= p1.Version {
		t.Fatalf("expected version bump, got v1=%d v2=%d", p1.Version, p2.Version)
	}
}

func TestServiceAddRule_ValidatesBuiltinName(t *testing.T) {
	repo := newStubRepo()
	svc := NewDLPScanner(repo)
	p, _ := svc.CreatePolicy(context.Background(), 1, 100, "p", "*", "low", "log", "")
	if _, err := svc.AddRule(context.Background(), 1, p.ID, "builtin", "no_such_pattern", "low", ""); err == nil {
		t.Fatalf("expected error for unknown builtin")
	}
}

func TestServiceAddRule_ValidatesPatternType(t *testing.T) {
	repo := newStubRepo()
	svc := NewDLPScanner(repo)
	p, _ := svc.CreatePolicy(context.Background(), 1, 100, "p", "*", "low", "log", "")
	if _, err := svc.AddRule(context.Background(), 1, p.ID, "made_up", "x", "low", ""); err == nil {
		t.Fatalf("expected error for unknown pattern_type")
	}
}

func TestTrimLower(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Hello  ", "hello"},
		{"WORLD", "world"},
		{"\tMiXeD\n", "mixed"},
	}
	for _, tc := range cases {
		if got := TrimLower(tc.in); got != tc.want {
			t.Fatalf("TrimLower(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestContextWindow_BoundarySafe(t *testing.T) {
	text := "abcdefghij"
	cases := []struct{ start, end int; wantContains string }{
		{0, 1, "abcdefghij"},     // tiny match — full context returned
		{3, 5, "abcdefghij"},     // near start — no leading chars available
		{8, 10, "abcdefghij"},    // near end — no trailing chars available
		{5, 5, "abcdefghij"},     // zero-length match
	}
	for _, tc := range cases {
		got := contextWindow(text, tc.start, tc.end, 32, 512)
		if !strings.Contains(got, tc.wantContains) {
			t.Fatalf("contextWindow(%d,%d) = %q does not contain %q",
				tc.start, tc.end, got, tc.wantContains)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 3); got != "hel" {
		t.Fatalf("truncate: got %q want %q", got, "hel")
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Fatalf("truncate: got %q want %q", got, "hi")
	}
}

func TestDLPValidSeverity(t *testing.T) {
	for _, s := range []string{"low", "medium", "high", "critical"} {
		if !types.DLPValidSeverity(s) {
			t.Fatalf("expected severity %q to be valid", s)
		}
	}
	if types.DLPValidSeverity("extreme") {
		t.Fatalf("expected severity 'extreme' to be invalid")
	}
}

func TestDLPValidAction(t *testing.T) {
	for _, a := range []string{"log", "block", "redact", "notify_dpo"} {
		if !types.DLPValidAction(a) {
			t.Fatalf("expected action %q to be valid", a)
		}
	}
	if types.DLPValidAction("encrypt") {
		t.Fatalf("expected action 'encrypt' to be invalid")
	}
}

// Smoke: large document should not panic and respects the match cap.
func TestScanner_RespectsMatchCap(t *testing.T) {
	rules := []policyRule{{
		PolicyID: 1, Action: "log",
		Rule: types.DLPRule{
			ID: 1, PatternType: "builtin", PatternValue: "email",
			Severity: "low", Enabled: true,
		},
	}}
	sc := newScanner(rules)
	// 10000 emails in a row.
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString(fmt.Sprintf("user%d@example.com ", i))
	}
	matches := sc.scan(sb.String(), 50)
	if len(matches) != 50 {
		t.Fatalf("expected cap of 50, got %d", len(matches))
	}
}
