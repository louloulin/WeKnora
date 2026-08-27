package chatpipeline

import (
	"context"
	"math"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
)

// Build #30 — chat pipeline reflection plugin.
//
// The fast chat pipeline (session_knowledge_qa.go) has no built-in way to
// recover from a poor first-pass retrieval: if rerank returns a top-1 score
// that is too low, or an empty result set, the chat model still gets the
// weak context and produces an ungrounded answer. PluginReflection is the
// fast-path heuristic that flags those cases and tells the caller
// (frontend, downstream stages) that a reflection was triggered.
//
// MVP scope (Build #30):
//   - Heuristic decision is fully implemented and unit-tested.
//   - ReflectionAttempted + ReflectionContext are populated on ChatManage.
//   - A streaming notification (event.ReflectionData) is emitted so the UI
//     can show "反思中..." (Build #30 A3).
//   - The actual re-search plumbing (re-run CHUNK_SEARCH with adjusted
//     params, re-merge, re-rerank, re-feed into INTO_CHAT_MESSAGE) is
//     intentionally deferred — that requires either a recursive
//     eventManager.Trigger pipeline or a queued re-entry, both out of
//     scope for a single-session delivery. Tracked as B30.1 / B31.
//
// Trigger conditions (D2/D3):
//   - SearchResult is empty AND we needed retrieval.
//   - Rerank top-1 score is below reflectionThreshold (default 0.5).
//   - At most one reflection per turn (ReflectionAttempted guards loops).

// Default heuristic constants. Threshold below 0.5 (low confidence) and
// top_k expanded by 50% / threshold loosened by 0.05 per Build #30 D3.
// Neither threshold dips below minReflectionThreshold (0) — there is no
// point setting a negative retrieval floor.
const (
	defaultReflectionThreshold     = 0.5
	reflectionTopKMultiplier      = 1.5
	reflectionThresholdDelta      = 0.05
	minReflectionThreshold        = 0.0
	maxReflectionsPerTurn         = 1
	reflectionReasonLowTopScore   = "low_top_score"
	reflectionReasonEmptyResults  = "empty_results"
	reflectionReasonDisabledMulti = "multi_turn_disabled"
)

// PluginReflection decides whether a turn needs a reflection re-retrieval.
//
// The plugin does not perform the re-retrieval itself — it inspects
// chatManage.SearchResult / RerankResult and, when the heuristic fires,
// stamps PipelineState with the adjusted params and emits a streaming
// notification. Re-search plumbing is a separate concern.
type PluginReflection struct {
	// threshold is the rerank top-1 score below which reflection fires.
	// Zero value uses defaultReflectionThreshold (0.5). Touched only by
	// tests; the wire-time construction always relies on the default.
	threshold float64
}

// NewPluginReflection registers the reflection plugin on the EventManager
// for the REFLECTION pipeline stage.
//
// The REFLECTION stage is fired explicitly by future pipeline orchestration
// (B30.1) after the rerank top-1 is known. Until then, no automatic
// trigger exists — the plugin sits dormant and tests drive OnEvent
// directly. This matches the spec: ActivationEvents() must return REFLECTION
// so that future re-search orchestration has a stable hook.
func NewPluginReflection(eventManager *EventManager) *PluginReflection {
	p := &PluginReflection{threshold: defaultReflectionThreshold}
	eventManager.Register(p)
	return p
}

// ActivationEvents declares which stages this plugin handles.
func (p *PluginReflection) ActivationEvents() []types.EventType {
	return []types.EventType{types.REFLECTION}
}

// OnEvent runs the heuristic and, when triggered, populates
// PipelineState.ReflectionAttempted/ReflectionContext and emits the
// streaming ReflectionData notification. Never returns a PluginError —
// reflection is best-effort and must never block the pipeline.
func (p *PluginReflection) OnEvent(
	ctx context.Context,
	eventType types.EventType,
	chatManage *types.ChatManage,
	next func() *PluginError,
) *PluginError {
	if err := next(); err != nil {
		return err
	}
	if chatManage == nil {
		return nil
	}

	// Build #30 D10: reflection is meaningful only when there is a
	// multi-turn history to reflect over. A single-turn request has no
	// prior retrieval quality to compare against, and forcing a
	// reflection only costs latency + tokens.
	if chatManage.MaxRounds <= 0 {
		pipelineInfo(ctx, "Reflection", "skip", map[string]interface{}{
			"reason":     reflectionReasonDisabledMulti,
			"session_id": chatManage.SessionID,
		})
		return nil
	}

	// Guard against infinite loops: at most one reflection per turn.
	if chatManage.ReflectionAttempted >= maxReflectionsPerTurn {
		pipelineInfo(ctx, "Reflection", "skip", map[string]interface{}{
			"reason":              "max_reflections_reached",
			"reflection_attempted": chatManage.ReflectionAttempted,
			"session_id":          chatManage.SessionID,
		})
		return nil
	}

	decision, reason, topScore := p.evaluate(chatManage)
	if !decision {
		pipelineInfo(ctx, "Reflection", "skip", map[string]interface{}{
			"reason":     "no_trigger",
			"top_score":  topScore,
			"session_id": chatManage.SessionID,
		})
		return nil
	}

	originalTopK := chatManage.EmbeddingTopK
	originalThresh := chatManage.VectorThreshold
	newTopK := expandTopK(originalTopK)
	newThresh := loosenThreshold(originalThresh)

	chatManage.ReflectionAttempted++
	chatManage.ReflectionContext = &types.ReflectionContext{
		Reason:         reason,
		OriginalTopK:   originalTopK,
		OriginalThresh: originalThresh,
		NewTopK:        newTopK,
		NewThresh:      newThresh,
	}

	pipelineInfo(ctx, "Reflection", "trigger", map[string]interface{}{
		"reason":            reason,
		"original_top_k":    originalTopK,
		"original_threshold": originalThresh,
		"new_top_k":         newTopK,
		"new_threshold":     newThresh,
		"top_score":         topScore,
		"session_id":        chatManage.SessionID,
	})

	p.emitReflectionEvent(ctx, chatManage, reason, newTopK, newThresh)
	return nil
}

// evaluate runs the heuristic and returns (shouldReflect, reason,
// observedTopScore). Pure function over ChatManage — no I/O, no globals,
// so it is trivially unit-testable.
func (p *PluginReflection) evaluate(chatManage *types.ChatManage) (bool, string, float64) {
	threshold := p.threshold
	if threshold <= minReflectionThreshold {
		threshold = defaultReflectionThreshold
	}

	// Heuristic 1: empty SearchResult means the retrieval pipeline returned
	// nothing. No chunk means no grounding — definitely reflect.
	if chatManage.NeedsRetrieval() && len(chatManage.SearchResult) == 0 {
		return true, reflectionReasonEmptyResults, 0
	}

	// Heuristic 2: rerank top-1 score below threshold. If RerankResult
	// is empty we fall back to SearchResult[0] (no rerank model in use),
	// and if both are empty we already handled it above.
	if len(chatManage.RerankResult) > 0 {
		top := chatManage.RerankResult[0].Score
		if top < threshold {
			return true, reflectionReasonLowTopScore, top
		}
		return false, "", top
	}
	if len(chatManage.SearchResult) > 0 {
		top := chatManage.SearchResult[0].Score
		if top < threshold {
			return true, reflectionReasonLowTopScore, top
		}
		return false, "", top
	}
	return false, "", 0
}

// expandTopK returns originalTopK * 1.5 rounded up to the next integer,
// with a floor of 1 to keep the value valid even when the request
// shipped with EmbeddingTopK=0.
func expandTopK(originalTopK int) int {
	if originalTopK <= 0 {
		return 1
	}
	expanded := int(math.Ceil(float64(originalTopK) * reflectionTopKMultiplier))
	if expanded < originalTopK {
		return originalTopK + 1
	}
	return expanded
}

// loosenThreshold subtracts 0.05 from the original threshold, floored at
// 0 (Build #30 D3: "下限 0.0,不能再低").
func loosenThreshold(originalThresh float64) float64 {
	next := originalThresh - reflectionThresholdDelta
	if next < minReflectionThreshold {
		return minReflectionThreshold
	}
	return next
}

// emitReflectionEvent sends the streaming notification so the UI can
// show "反思中..." (Build #30 A3). Fire-and-forget — a failure to emit
// must not fail the pipeline. The frontend times out after 5s and
// recovers to its normal answer-streaming state (spec Known limit #4).
func (p *PluginReflection) emitReflectionEvent(
	ctx context.Context,
	chatManage *types.ChatManage,
	reason string,
	newTopK int,
	newThresh float64,
) {
	if chatManage.EventBus == nil {
		return
	}
	data := event.ReflectionData{
		Reason:         reason,
		OriginalTopK:   chatManage.ReflectionContext.OriginalTopK,
		OriginalThresh: chatManage.ReflectionContext.OriginalThresh,
		Iteration:      chatManage.ReflectionAttempted,
		Done:           true,
		AdjustedParams: map[string]interface{}{
			"embedding_top_k":  newTopK,
			"vector_threshold": newThresh,
		},
	}
	if err := chatManage.EventBus.Emit(ctx, types.Event{
		Type:      types.EventType(event.EventAgentReflection),
		SessionID: chatManage.SessionID,
		Data:      data,
	}); err != nil {
		pipelineWarn(ctx, "Reflection", "emit_failed", map[string]interface{}{
			"reason": reason,
			"error":  err.Error(),
		})
	}
}