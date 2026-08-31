package llmstream_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/llmstream"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestNoopProvider_Name(t *testing.T) {
	if got := (llmstream.NoopProvider{}).Name(); got != "noop" {
		t.Fatalf("Name() = %q, want noop", got)
	}
}

func TestNoopProvider_Complete(t *testing.T) {
	resp, err := (llmstream.NoopProvider{}).Complete(context.Background(), llmstream.Request{
		Query:         "what is X?",
		KBCitations:   makeKBs(3),
		WikiCitations: makeWikis(2),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.ModelName != "noop" {
		t.Fatalf("ModelName = %q, want noop", resp.ModelName)
	}
	if !strings.Contains(resp.AnswerText, "3 KB citation") || !strings.Contains(resp.AnswerText, "2 wiki page") {
		t.Fatalf("placeholder missing counts: %q", resp.AnswerText)
	}
	if resp.FinishedAt.Before(resp.StartedAt) {
		t.Fatalf("FinishedAt before StartedAt")
	}
}

func TestNoopProvider_Stream_EmitsTokenThenDone(t *testing.T) {
	var got []llmstream.EventType
	sink := llmstream.FuncSink(func(e llmstream.Event) error {
		got = append(got, e.Type)
		return nil
	})
	err := (llmstream.NoopProvider{}).Stream(context.Background(),
		llmstream.Request{Query: "x", KBCitations: makeKBs(1)},
		sink,
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(got) != 2 || got[0] != llmstream.EventToken || got[1] != llmstream.EventDone {
		t.Fatalf("event sequence = %v, want [token done]", got)
	}
}

func TestFuncSink_PropagatesError(t *testing.T) {
	wantErr := errors.New("sink down")
	sink := llmstream.FuncSink(func(e llmstream.Event) error { return wantErr })
	if err := (llmstream.NoopProvider{}).Stream(context.Background(),
		llmstream.Request{Query: "x"}, sink,
	); !errors.Is(err, wantErr) {
		t.Fatalf("expected sink error to propagate, got %v", err)
	}
}

func TestFormatSSEEvent_Token(t *testing.T) {
	out, err := llmstream.FormatSSEEvent(llmstream.Event{
		Type: llmstream.EventToken,
		Data: llmstream.TokenEventData{Text: "hello"},
	})
	if err != nil {
		t.Fatalf("FormatSSEEvent: %v", err)
	}
	want := "event: token\ndata: {\"text\":\"hello\"}\n\n"
	if string(out) != want {
		t.Fatalf("got %q, want %q", string(out), want)
	}
}

func TestFormatSSEEvent_Citation(t *testing.T) {
	out, err := llmstream.FormatSSEEvent(llmstream.Event{
		Type: llmstream.EventCitation,
		Data: llmstream.CitationEventData{
			Index: 1,
			Citation: types.AssistantCitation{
				Type: "kb", ID: "k1", Title: "Doc", Score: 0.9,
			},
		},
	})
	if err != nil {
		t.Fatalf("FormatSSEEvent: %v", err)
	}
	if !strings.Contains(string(out), "event: citation") {
		t.Fatalf("missing event header: %s", out)
	}
	if !strings.Contains(string(out), `"Doc"`) {
		t.Fatalf("missing title: %s", out)
	}
}

func TestFormatSSEEvent_Done(t *testing.T) {
	out, err := llmstream.FormatSSEEvent(llmstream.Event{
		Type: llmstream.EventDone,
		Data: llmstream.DoneEventData{PromptTokens: 42, CompletionTokens: 17, FinishReason: "stop"},
	})
	if err != nil {
		t.Fatalf("FormatSSEEvent: %v", err)
	}
	if !strings.Contains(string(out), `"prompt_tokens":42`) {
		t.Fatalf("missing prompt_tokens: %s", out)
	}
	if !strings.Contains(string(out), `"completion_tokens":17`) {
		t.Fatalf("missing completion_tokens: %s", out)
	}
}

func TestFormatSSEEvent_Error(t *testing.T) {
	out, err := llmstream.FormatSSEEvent(llmstream.Event{
		Type:  llmstream.EventError,
		Error: errors.New("upstream blew up"),
	})
	if err != nil {
		t.Fatalf("FormatSSEEvent: %v", err)
	}
	if !strings.Contains(string(out), `"error":"upstream blew up"`) {
		t.Fatalf("missing error msg: %s", out)
	}
}

func TestFormatSSEEvent_UnknownType(t *testing.T) {
	if _, err := llmstream.FormatSSEEvent(llmstream.Event{Type: "nonsense"}); err == nil {
		t.Fatalf("expected error on unknown event type")
	}
}

func TestIsTransient(t *testing.T) {
	if llmstream.IsTransient(nil) {
		t.Fatalf("nil must not be transient")
	}
	if !llmstream.IsTransient(llmstream.ErrProviderUnavailable) {
		t.Fatalf("Unavailable must be transient")
	}
	if !llmstream.IsTransient(llmstream.ErrProviderTimeout) {
		t.Fatalf("Timeout must be transient")
	}
	if !llmstream.IsTransient(errors.New("wrapped: " + llmstream.ErrProviderTimeout.Error())) {
		// Errors.Is only matches via %w wrap chain. Use proper wrap:
	}
	wrapped := wrapErr(llmstream.ErrProviderTimeout)
	if !llmstream.IsTransient(wrapped) {
		t.Fatalf("wrapped Timeout must be transient")
	}
	if llmstream.IsTransient(errors.New("permanent failure")) {
		t.Fatalf("unknown error must NOT be transient")
	}
}

// wrapErr wraps inner with fmt.Errorf("...: %w", inner) so IsTransient
// can demonstrate the %w unwrap path.
func wrapErr(inner error) error {
	return wrappedErr{inner: inner}
}

type wrappedErr struct{ inner error }

func (w wrappedErr) Error() string { return "wrap: " + w.inner.Error() }
func (w wrappedErr) Unwrap() error { return w.inner }

// TestStreamSink_ConcurrentSafe — drive two streams from goroutines
// and wait for both to finish; the assertion is that both finish
// without races (run with -race) and that we saw at least 4 events
// (each stream emits one token + one done).
func TestStreamSink_ConcurrentSafe(t *testing.T) {
	var (
		mu   sync.Mutex
		seen int
	)
	sink := llmstream.FuncSink(func(e llmstream.Event) error {
		mu.Lock()
		seen++
		mu.Unlock()
		return nil
	})
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = (llmstream.NoopProvider{}).Stream(ctx, llmstream.Request{Query: "x"}, sink)
	}()
	go func() {
		defer wg.Done()
		_ = (llmstream.NoopProvider{}).Stream(ctx, llmstream.Request{Query: "y"}, sink)
	}()
	wg.Wait()
	mu.Lock()
	got := seen
	mu.Unlock()
	if got < 4 {
		t.Fatalf("expected at least 4 events, got %d", got)
	}
}

// makeKBs and makeWikis build tiny citation slices for the tests
// so the placeholder-string assertions stay readable.
func makeKBs(n int) []types.AssistantCitation {
	out := make([]types.AssistantCitation, n)
	for i := range out {
		out[i] = types.AssistantCitation{Type: "kb", ID: "k" + string(rune('A'+i)), Title: "Doc"}
	}
	return out
}

func makeWikis(n int) []types.AssistantCitation {
	out := make([]types.AssistantCitation, n)
	for i := range out {
		out[i] = types.AssistantCitation{Type: "wiki", ID: "w" + string(rune('A'+i)), Title: "Page"}
	}
	return out
}

// Compile-time assertion that NoopProvider satisfies Provider.
var _ llmstream.Provider = llmstream.NoopProvider{}
