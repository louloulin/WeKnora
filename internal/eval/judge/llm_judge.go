package judge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// Build #31 — LLM-as-judge caller.
//
// The judge sits behind three pure scoring entry points (JudgeFactuality,
// JudgeCitationFidelity, JudgeReflectionNecessity); each composes a
// prompt from the templates in prompts.go, calls the configured judge
// model, and parses the JSON response into a 1-5 score.
//
// Design notes:
//
//   - LLM-as-judge is a *separate* model from the chat model so a
//     reflection loop / self-evaluation bias cannot inflate the score.
//     Build #31 D2 picks the first ModelTypeKnowledgeQA model by
//     default; admins can override per-run via EvalRunStartRequest.
//   - The judge never blocks the runner pipeline: each call has its
//     own bounded timeout via the model service, and a judge failure
//     degrades the run to "score missing" instead of failing the QA.
//     Run-level failure is reserved for "the dataset is corrupt" — a
//     per-QA judge error must not abort a 1000-QA run.
//   - The LLMCallers interface is satisfied by the existing
//     application LLM service (chat_manage). Tests can pass a stub.

const (
	// minScore / maxScore are the closed bounds the LLM is asked to
	// produce. Anything outside the range triggers parse failure.
	minScore = 1
	maxScore = 5
)

// LLMCaller is the slice of the chat LLM service the judge needs.
// Returning raw string content keeps the judge package decoupled from
// the chat streaming protocol — we never want the judge to block on
// chunk boundaries.
type LLMCaller interface {
	Chat(ctx context.Context, modelID string, messages []chat.Message) (string, error)
}

// JudgeResult is the parsed shape from any judge call.
type JudgeResult struct {
	Score     int    `json:"score"`
	Rationale string `json:"rationale"`
}

// ErrJudgeParseFailure is returned when the LLM output is not the
// expected JSON shape. The runner converts this into a per-QA
// "score_missing" without aborting the run.
var ErrJudgeParseFailure = errors.New("judge: parse failure")

// parseJudgeResponse pulls the first JSON object out of the LLM
// output. We accept leading/trailing prose because some chat models
// refuse pure JSON-only outputs even when explicitly told to. The
// rationale field is optional and trimmed.
func parseJudgeResponse(raw string) (JudgeResult, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return JudgeResult{}, fmt.Errorf("%w: no JSON object in response: %q", ErrJudgeParseFailure, raw)
	}
	candidate := raw[start : end+1]
	var out JudgeResult
	err := json.Unmarshal([]byte(candidate), &out)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("%w: %v", ErrJudgeParseFailure, err)
	}
	if out.Score < minScore || out.Score > maxScore {
		return JudgeResult{}, fmt.Errorf("%w: score %d out of [%d,%d]",
			ErrJudgeParseFailure, out.Score, minScore, maxScore)
	}
	return JudgeResult{Score: out.Score, Rationale: strings.TrimSpace(out.Rationale)}, nil
}

// clampScore coerces a 1-5 score into the [1, 5] closed range. Used
// by the heuristic functions in citation_fidelity.go and
// reflection_necessity.go so the summary averaging never sees an
// out-of-range outlier from a malformed event log.
func clampScore(v int) int {
	if v < minScore {
		return minScore
	}
	if v > maxScore {
		return maxScore
	}
	return v
}

// formatScore renders a score as a stable, parseable string the LLM
// can see in prompts that quote past scores. We never round-trip
// through float64 to avoid "5.0 vs 5" quirks in prompt text.
func formatScore(s int) string { return strconv.Itoa(s) }

// JudgeFactuality runs the factuality judge for one QA row.
//
// Returns the JudgeResult on success, ErrJudgeParseFailure when the
// LLM output is unparseable, and any wrapped model error otherwise.
// The runner treats the parse case as "score missing"; other failures
// are surfaced as a run-level error so an operator can retry.
func JudgeFactuality(ctx context.Context, caller LLMCaller, modelID, question, expectedAnswer, modelAnswer string) (JudgeResult, error) {
	if caller == nil {
		return JudgeResult{}, errors.New("judge: nil LLM caller")
	}
	prompt := fmt.Sprintf(factualityPromptTemplate, question, expectedAnswer, modelAnswer)
	messages := []chat.Message{{Role: "user", Content: prompt}}
	raw, err := caller.Chat(ctx, modelID, messages)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("judge: factuality chat: %w", err)
	}
	result, err := parseJudgeResponse(raw)
	if err != nil {
		logger.Warnf(ctx, "[judge] factuality parse failure model_id=%s raw=%q", modelID, truncate(raw, 200))
		return JudgeResult{}, err
	}
	return result, nil
}

// JudgeCitationFidelity runs the citation-fidelity judge. The
// citationIndexJSON argument is the verbatim `citation_index` JSON
// payload from chat_manage (B30 B3), already in the {1: chunk_id, ...
// 1-based int → chunk_id} shape. The heuristic companion
// (citation_fidelity.go:HeuristicCitationFidelity) verifies the
// indices are well-formed; this call verifies the content match.
func JudgeCitationFidelity(ctx context.Context, caller LLMCaller, modelID, question, modelAnswer string, citationIndexJSON json.RawMessage) (JudgeResult, error) {
	if caller == nil {
		return JudgeResult{}, errors.New("judge: nil LLM caller")
	}
	citationDump := string(citationIndexJSON)
	if strings.TrimSpace(citationDump) == "" || citationDump == "null" {
		citationDump = "(no citations)"
	}
	prompt := fmt.Sprintf(citationFidelityPromptTemplate, question, citationDump, modelAnswer)
	messages := []chat.Message{{Role: "user", Content: prompt}}
	raw, err := caller.Chat(ctx, modelID, messages)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("judge: citation fidelity chat: %w", err)
	}
	result, err := parseJudgeResponse(raw)
	if err != nil {
		logger.Warnf(ctx, "[judge] citation_fidelity parse failure model_id=%s raw=%q", modelID, truncate(raw, 200))
		return JudgeResult{}, err
	}
	return result, nil
}

// JudgeReflectionNecessity runs the reflection-necessity judge. The
// reflectionEventsJSON argument is the verbatim `reflection_events`
// JSON from chat_manage; an empty value (or `[]`) means the pipeline
// did not reflect, which the judge treats as a high-necessity signal.
func JudgeReflectionNecessity(ctx context.Context, caller LLMCaller, modelID, question, modelAnswer string, reflectionEventsJSON json.RawMessage) (JudgeResult, error) {
	if caller == nil {
		return JudgeResult{}, errors.New("judge: nil LLM caller")
	}
	eventsDump := string(reflectionEventsJSON)
	if strings.TrimSpace(eventsDump) == "" || eventsDump == "null" {
		eventsDump = "[]"
	}
	prompt := fmt.Sprintf(reflectionNecessityPromptTemplate, question, modelAnswer, eventsDump)
	messages := []chat.Message{{Role: "user", Content: prompt}}
	raw, err := caller.Chat(ctx, modelID, messages)
	if err != nil {
		return JudgeResult{}, fmt.Errorf("judge: reflection necessity chat: %w", err)
	}
	result, err := parseJudgeResponse(raw)
	if err != nil {
		logger.Warnf(ctx, "[judge] reflection_necessity parse failure model_id=%s raw=%q", modelID, truncate(raw, 200))
		return JudgeResult{}, err
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}