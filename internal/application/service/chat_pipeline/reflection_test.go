package chatpipeline

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// reflectionCapturingBus captures emitted events so tests can assert that
// reflection fired (or did not fire) and what payload it carried. The
// production EventBus is sync by default; tests that care about emit
// outcomes can swap in this stub without pulling the bus apart.
type reflectionCapturingBus struct {
	types.EventBusInterface
	emitted []event.ReflectionData
}

func (b *reflectionCapturingBus) Emit(_ context.Context, evt types.Event) error {
	if data, ok := evt.Data.(event.ReflectionData); ok {
		b.emitted = append(b.emitted, data)
	}
	return nil
}

func (b *reflectionCapturingBus) On(types.EventType, types.EventHandler) {}

func newReflectionTestPlugin() *PluginReflection {
	return NewPluginReflection(NewEventManager())
}

func newReflectionChatManage(bus types.EventBusInterface) *types.ChatManage {
	cm := &types.ChatManage{}
	cm.SessionID = "s-1"
	cm.Query = "再展开说说 X"
	cm.MaxRounds = 3
	cm.EmbeddingTopK = 10
	cm.VectorThreshold = 0.6
	if bus != nil {
		cm.EventBus = bus
	}
	return cm
}

// TestReflectionFiresOnEmptySearchResult covers the empty-results branch
// of the heuristic: a turn that needed retrieval but produced zero
// chunks must trigger reflection so the answer pipeline can recover.
func TestReflectionFiresOnEmptySearchResult(t *testing.T) {
	bus := &reflectionCapturingBus{}
	cm := newReflectionChatManage(bus)
	cm.SearchResult = nil
	cm.RerankResult = nil
	// Intent defaults to "" which NeedsKBRetrieval() treats as needing
	// retrieval — exactly the empty-result trigger condition.

	nextCalled := false
	err := newReflectionTestPlugin().OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError {
		nextCalled = true
		return nil
	})
	require.Nil(t, err)
	require.True(t, nextCalled, "reflection must always continue the pipeline")

	require.Equal(t, 1, cm.ReflectionAttempted)
	require.NotNil(t, cm.ReflectionContext)
	require.Equal(t, reflectionReasonEmptyResults, cm.ReflectionContext.Reason)
	require.Equal(t, 10, cm.ReflectionContext.OriginalTopK)
	require.Equal(t, 0.6, cm.ReflectionContext.OriginalThresh)
	require.Equal(t, 15, cm.ReflectionContext.NewTopK, "10 * 1.5 = 15 (Build #30 D3)")
	require.InDelta(t, 0.55, cm.ReflectionContext.NewThresh, 1e-9, "0.6 - 0.05 = 0.55")

	require.Len(t, bus.emitted, 1, "the heuristic must emit exactly one reflection event")
	require.Equal(t, reflectionReasonEmptyResults, bus.emitted[0].Reason)
	require.True(t, bus.emitted[0].Done)
	require.Equal(t, 1, bus.emitted[0].Iteration)
	require.Equal(t, float64(0), bus.emitted[0].TopScore, "empty results report zero top score")
}

// TestReflectionFiresOnLowTopScore covers the score-threshold branch:
// rerank returned a top-1 score below the default threshold of 0.5.
func TestReflectionFiresOnLowTopScore(t *testing.T) {
	bus := &reflectionCapturingBus{}
	cm := newReflectionChatManage(bus)
	cm.SearchResult = []*types.SearchResult{{Score: 0.42}, {Score: 0.31}}
	cm.RerankResult = []*types.SearchResult{{Score: 0.42}, {Score: 0.31}}

	nextCalled := false
	err := newReflectionTestPlugin().OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError {
		nextCalled = true
		return nil
	})
	require.Nil(t, err)
	require.True(t, nextCalled)

	require.Equal(t, 1, cm.ReflectionAttempted)
	require.NotNil(t, cm.ReflectionContext)
	require.Equal(t, reflectionReasonLowTopScore, cm.ReflectionContext.Reason)
	require.Equal(t, 15, cm.ReflectionContext.NewTopK)
	require.InDelta(t, 0.55, cm.ReflectionContext.NewThresh, 1e-9)
	require.Len(t, bus.emitted, 1)
	require.Equal(t, reflectionReasonLowTopScore, bus.emitted[0].Reason)
	require.Equal(t, 10, bus.emitted[0].OriginalTopK, "OriginalTopK mirrors the EmbeddingTopK set on the request")
	require.Equal(t, 0.6, bus.emitted[0].OriginalThresh, "OriginalThresh mirrors the VectorThreshold set on the request")
}

// TestReflectionSkipsWhenScoreAboveThreshold covers the negative path:
// retrieval produced strong results and reflection would only waste
// tokens, so the plugin must not modify PipelineState or emit any
// notification.
func TestReflectionSkipsWhenScoreAboveThreshold(t *testing.T) {
	bus := &reflectionCapturingBus{}
	cm := newReflectionChatManage(bus)
	cm.SearchResult = []*types.SearchResult{{Score: 0.91}, {Score: 0.83}}
	cm.RerankResult = []*types.SearchResult{{Score: 0.91}, {Score: 0.83}}

	err := newReflectionTestPlugin().OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError { return nil })
	require.Nil(t, err)
	require.Equal(t, 0, cm.ReflectionAttempted)
	require.Nil(t, cm.ReflectionContext, "no reflection context when heuristic says skip")
	require.Empty(t, bus.emitted, "no streaming notification on a healthy retrieval")
}

// TestReflectionSkipsWhenMultiTurnDisabled enforces Build #30 D10:
// without multi-turn history there is nothing to reflect over, so the
// heuristic must be inert.
func TestReflectionSkipsWhenMultiTurnDisabled(t *testing.T) {
	bus := &reflectionCapturingBus{}
	cm := newReflectionChatManage(bus)
	cm.MaxRounds = 0 // no history → multi-turn disabled
	cm.SearchResult = nil // would normally trigger empty-results

	err := newReflectionTestPlugin().OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError { return nil })
	require.Nil(t, err)
	require.Equal(t, 0, cm.ReflectionAttempted)
	require.Nil(t, cm.ReflectionContext)
	require.Empty(t, bus.emitted)
}

// TestReflectionCapsAtOnePerTurn enforces Build #30 D2: the heuristic
// must not loop. After a successful reflection, a second invocation
// during the same turn must skip rather than re-fire.
func TestReflectionCapsAtOnePerTurn(t *testing.T) {
	bus := &reflectionCapturingBus{}
	cm := newReflectionChatManage(bus)
	cm.SearchResult = nil

	plugin := newReflectionTestPlugin()

	// First invocation — fires and stamps the state.
	require.Nil(t, plugin.OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError { return nil }))
	require.Equal(t, 1, cm.ReflectionAttempted)
	require.NotNil(t, cm.ReflectionContext)
	require.Len(t, bus.emitted, 1)

	// Second invocation — must skip regardless of retrieval state.
	cm.SearchResult = []*types.SearchResult{{Score: 0.1}} // would normally trigger again
	require.Nil(t, plugin.OnEvent(t.Context(), types.REFLECTION, cm, func() *PluginError { return nil }))
	require.Equal(t, 1, cm.ReflectionAttempted, "reflection must not increment past 1")
	require.Len(t, bus.emitted, 1, "the second invocation must not emit a second event")
}

// TestExpandTopKAndLoosenThreshold are the pure-function sanity tests
// for the two arithmetic helpers — they are the only knobs the
// heuristic can move, so any future regression here would silently
// change the operator-visible behaviour.
func TestExpandTopKAndLoosenThreshold(t *testing.T) {
	require.Equal(t, 15, expandTopK(10), "10 * 1.5 = 15 (round-up)")
	require.Equal(t, 17, expandTopK(11), "11 * 1.5 = 16.5 → ceil = 17")
	require.Equal(t, 1, expandTopK(0), "zero top_k must not divide by zero — return 1")
	require.Equal(t, 1, expandTopK(-3), "negative top_k must not produce garbage — return 1")
	require.Equal(t, 21, expandTopK(14))

	require.InDelta(t, 0.55, loosenThreshold(0.6), 1e-9)
	require.InDelta(t, 0.0, loosenThreshold(0.02), 1e-9, "floor at 0.0 (Build #30 D3)")
	require.InDelta(t, 0.0, loosenThreshold(-0.5), 1e-9, "floor at 0.0 even for negative inputs")
}