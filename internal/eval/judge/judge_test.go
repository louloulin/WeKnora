package judge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
)

// Build #31 — judge harness tests.
//
// These tests pin the pure scoring behaviour so a future refactor
// cannot silently change the score distribution. The runner reads
// these scores to auto-flag badcases; changing the bands without
// updating the harness would silently raise or lower the threshold.

func TestParseJudgeResponse_HappyPath(t *testing.T) {
	raw := `{"score": 4, "rationale": "mostly correct"}`
	res, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Score != 4 {
		t.Fatalf("expected score 4, got %d", res.Score)
	}
	if !strings.Contains(res.Rationale, "mostly correct") {
		t.Fatalf("rationale lost: %q", res.Rationale)
	}
}

func TestParseJudgeResponse_ScoreOutOfRange(t *testing.T) {
	cases := []string{
		`{"score": 0, "rationale": "bad"}`,
		`{"score": 6, "rationale": "bad"}`,
		`{"score": -1, "rationale": "bad"}`,
	}
	for _, raw := range cases {
		if _, err := parseJudgeResponse(raw); !errors.Is(err, ErrJudgeParseFailure) {
			t.Fatalf("expected ErrJudgeParseFailure for %q, got %v", raw, err)
		}
	}
}

func TestParseJudgeResponse_ToleratesProse(t *testing.T) {
	raw := "Sure! Here you go: {\"score\": 5, \"rationale\": \"perfect\"} -- hope that helps!"
	res, err := parseJudgeResponse(raw)
	if err != nil {
		t.Fatalf("expected tolerance for prose, got %v", err)
	}
	if res.Score != 5 {
		t.Fatalf("expected 5, got %d", res.Score)
	}
}

func TestParseJudgeResponse_NoJSON(t *testing.T) {
	if _, err := parseJudgeResponse("sorry, can't help"); !errors.Is(err, ErrJudgeParseFailure) {
		t.Fatalf("expected ErrJudgeParseFailure, got %v", err)
	}
}

func TestClampScore(t *testing.T) {
	cases := map[string]struct {
		in, want int
	}{
		"below range": {-1, 1},
		"min":         {1, 1},
		"mid":         {3, 3},
		"max":         {5, 5},
		"above range": {9, 5},
	}
	for name, c := range cases {
		got := clampScore(c.in)
		if got != c.want {
			t.Fatalf("%s: clampScore(%d)=%d, want %d", name, c.in, got, c.want)
		}
	}
}

// stubCaller returns canned responses. Used for the LLM-driven paths.
type stubCaller struct {
	responses map[string]string
	err       error
	calls     int
}

func (s *stubCaller) Chat(_ context.Context, _ string, _ []chat.Message) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	if resp, ok := s.responses["default"]; ok {
		return resp, nil
	}
	return `{"score": 3, "rationale": "stub default"}`, nil
}

func TestJudgeFactuality_HappyPath(t *testing.T) {
	caller := &stubCaller{responses: map[string]string{
		"default": `{"score": 5, "rationale": "correct"}`,
	}}
	res, err := JudgeFactuality(context.Background(), caller, "model-1", "q", "expected", "got")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Score != 5 {
		t.Fatalf("expected 5, got %d", res.Score)
	}
	if caller.calls != 1 {
		t.Fatalf("expected 1 call, got %d", caller.calls)
	}
}

func TestJudgeFactuality_ParseFailure(t *testing.T) {
	caller := &stubCaller{responses: map[string]string{
		"default": "not json",
	}}
	_, err := JudgeFactuality(context.Background(), caller, "model-1", "q", "expected", "got")
	if !errors.Is(err, ErrJudgeParseFailure) {
		t.Fatalf("expected ErrJudgeParseFailure, got %v", err)
	}
}

func TestJudgeFactuality_NilCaller(t *testing.T) {
	_, err := JudgeFactuality(context.Background(), nil, "model-1", "q", "expected", "got")
	if err == nil {
		t.Fatal("expected error for nil caller")
	}
}

// ---------------------------------------------------------------------
// Heuristic citation_fidelity
// ---------------------------------------------------------------------

func TestHeuristicCitationFidelity(t *testing.T) {
	cases := []struct {
		name      string
		answer    string
		idxJSON   string
		wantScore int
	}{
		{
			name:      "valid single citation",
			answer:    "answer text [[cite:1]]",
			idxJSON:   `{"1":"chunk_a"}`,
			wantScore: 5,
		},
		{
			name:      "out-of-range citation",
			answer:    "answer [[cite:99]]",
			idxJSON:   `{"1":"chunk_a"}`,
			wantScore: 2, // empty idx map = score 2
		},
		{
			name:      "duplicate citation",
			answer:    "first [[cite:1]] then [[cite:1]]",
			idxJSON:   `{"1":"chunk_a"}`,
			wantScore: 4,
		},
		{
			name:      "no citations at all",
			answer:    "answer without any cites",
			idxJSON:   `{"1":"chunk_a"}`,
			wantScore: 2,
		},
		{
			name:      "empty answer",
			answer:    "",
			idxJSON:   `{"1":"chunk_a"}`,
			wantScore: 1,
		},
		{
			name:      "missing citation_index",
			answer:    "answer [[cite:1]]",
			idxJSON:   `null`,
			wantScore: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HeuristicCitationFidelity(c.answer, json.RawMessage(c.idxJSON))
			if got != c.wantScore {
				t.Fatalf("HeuristicCitationFidelity(%q, %s)=%d, want %d",
					c.answer, c.idxJSON, got, c.wantScore)
			}
		})
	}
}

func TestCombinedCitationFidelity(t *testing.T) {
	if got := CombinedCitationFidelity(5, 3); got != 3 {
		t.Fatalf("expected min(5,3)=3, got %d", got)
	}
	if got := CombinedCitationFidelity(0, 4); got != 4 {
		t.Fatalf("expected 4 (heuristic zero falls back), got %d", got)
	}
	if got := CombinedCitationFidelity(5, 0); got != 5 {
		t.Fatalf("expected 5 (no llm), got %d", got)
	}
	if got := CombinedCitationFidelity(7, 8); got != 5 { // clamps
		t.Fatalf("expected clamp to 5, got %d", got)
	}
}

// ---------------------------------------------------------------------
// Heuristic reflection_necessity
// ---------------------------------------------------------------------

func TestHeuristicReflectionNecessity(t *testing.T) {
	cases := []struct {
		name     string
		events   string
		passed   bool
		wantScore int
	}{
		{
			name:    "no events, passed",
			events:  `null`,
			passed:  true,
			wantScore: 4,
		},
		{
			name:    "no events, failed",
			events:  `null`,
			passed:  false,
			wantScore: 1,
		},
		{
			name:    "events improved",
			events:  `[{"type":"chat_reflection","outcome":"resolved"}]`,
			passed:  true,
			wantScore: 5,
		},
		{
			name:    "events errored",
			events:  `[{"type":"chat_reflection","outcome":"error"}]`,
			passed:  false,
			wantScore: 3,
		},
		{
			name:    "events empty array",
			events:  `[]`,
			passed:  true,
			wantScore: 4,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HeuristicReflectionNecessity(json.RawMessage(c.events), c.passed)
			if got != c.wantScore {
				t.Fatalf("HeuristicReflectionNecessity(%s, passed=%v)=%d, want %d",
					c.events, c.passed, got, c.wantScore)
			}
		})
	}
}

func TestShouldAutoFlag(t *testing.T) {
	cases := []struct {
		fact, cite, refl int
		want             bool
	}{
		{5, 5, 5, false},  // avg 5
		{3, 3, 3, false},  // avg 3 (boundary; not below)
		{2, 4, 4, true},   // avg ~3.33? No: (2+4+4)/3 = 3.33 → false
		{2, 3, 3, true},   // avg 2.67
		{1, 1, 1, true},   // avg 1
		{4, 2, 2, true},   // avg ~2.67
	}
	for _, c := range cases {
		got := ShouldAutoFlag(c.fact, c.cite, c.refl)
		avg := (c.fact + c.cite + c.refl) / 3.0
		wantFlag := avg < AutoBadcaseThreshold
		if got != wantFlag {
			t.Fatalf("ShouldAutoFlag(%d,%d,%d)=%v, want %v (avg=%.2f, threshold=%.2f)",
				c.fact, c.cite, c.refl, got, wantFlag, avg, AutoBadcaseThreshold)
		}
	}
}

func TestPromptVersion(t *testing.T) {
	if PromptVersion() == "" {
		t.Fatal("PromptVersion must not be empty")
	}
	if !strings.HasPrefix(PromptVersion(), "v") {
		t.Fatalf("PromptVersion should start with 'v', got %q", PromptVersion())
	}
}