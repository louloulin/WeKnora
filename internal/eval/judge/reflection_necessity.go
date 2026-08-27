package judge

import (
	"encoding/json"
	"errors"
	"strings"
)

// Build #31 — reflection_necessity heuristic.
//
// Same composition as citation_fidelity: the full score is min(heuristic,
// llm). The heuristic half answers the question "did the chat pipeline
// run a reflection pass?". When the pipeline did NOT reflect AND the
// QA failed factuality, reflection would clearly have been useful — so
// the heuristic surfaces low.
//
// This is the only one of the three judge dimensions where the
// heuristic half can override the LLM. Reflection is a structural
// signal (event log presence), not a content signal — the LLM can
// only judge whether reflection *would have helped*; the heuristic
// confirms whether reflection *actually ran*.

var errReflectionUnparseable = errors.New("judge: reflection_events unparseable")

// ReflectionEvent is one entry in chat_manage's chat_reflection event
// log (Build #30 B1). We only need a couple of fields for the
// heuristic; the runner's EvalRunResult stores the full event as JSON.
type ReflectionEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
}

// HeuristicReflectionNecessity scores the QA on whether a reflection
// pass was actually attempted.
//
// Score bands:
//   5: reflection ran and reported outcome=resolved or outcome=improved
//   4: reflection ran and produced an error / partial
//   3: reflection ran but produced no events (degenerate case)
//   2: reflection did not run, but factuality passed
//   1: reflection did not run, and factuality failed
//
// The function is pure. Same inputs → same output. The harness pins
// this so the score distribution is stable across PRs.
func HeuristicReflectionNecessity(reflectionEventsJSON json.RawMessage, passed bool) int {
	events, err := parseReflectionEvents(reflectionEventsJSON)
	if err != nil {
		// Empty / unparseable events = reflection did not run. Use the
		// `passed` flag to disambiguate "needs reflection" from
		// "reflected already and got it right".
		if passed {
			return 4
		}
		return 1
	}
	if len(events) == 0 {
		if passed {
			return 4
		}
		return 2
	}
	improved := 0
	errored := 0
	for _, e := range events {
		switch strings.ToLower(strings.TrimSpace(e.Outcome)) {
		case "resolved", "improved":
			improved++
		case "error", "failed":
			errored++
		}
	}
	switch {
	case improved > 0 && errored == 0:
		return 5
	case improved > 0 && errored > 0:
		return 4
	case improved == 0 && errored > 0:
		return 3
	default:
		// Reflection ran but produced no terminal outcome events.
		// Treat as "inconclusive" rather than "succeeded".
		return 3
	}
}

func parseReflectionEvents(raw json.RawMessage) ([]ReflectionEvent, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errReflectionUnparseable
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "[]" {
		return []ReflectionEvent{}, nil
	}
	var events []ReflectionEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, errReflectionUnparseable
	}
	return events, nil
}

// CombinedReflectionNecessity returns min(heuristic, llmScore). When
// llmScore is 0 (judge was not run), we return the heuristic alone.
func CombinedReflectionNecessity(heuristic, llmScore int) int {
	if llmScore <= 0 {
		return clampScore(heuristic)
	}
	h := clampScore(heuristic)
	l := clampScore(llmScore)
	if h < l {
		return h
	}
	return l
}

// AutoBadcaseThreshold is the average score below which EvalRunner
// auto-flags the QA as a badcase (Build #31 D3). Picked at 3.0 so a
// single 2.0 dimension pulls the average below threshold, and a
// score of "3 across the board" sits on the boundary (borderline).
const AutoBadcaseThreshold = 3.0

// ShouldAutoFlag returns true when the per-QA averaged score is below
// the auto badcase threshold. The runner calls this once per QA after
// computing the three dimensions.
func ShouldAutoFlag(factuality, citationFidelity, reflectionNecessity int) bool {
	avg := (float64(factuality) + float64(citationFidelity) + float64(reflectionNecessity)) / 3.0
	return avg < AutoBadcaseThreshold
}