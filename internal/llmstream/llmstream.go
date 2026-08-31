// Package llmstream is the v0.7.17 seam between the AI Assistant
// retrieval backend and the LLM provider that turns retrieved
// citations into a natural-language answer.
//
// Today the package only ships the wire shape (Request / Event /
// Provider interface) and a NoopProvider that emits the placeholder
// answer text used by v0.7.15. The real OpenAI / Anthropic /
// Doubao / Qwen / DeepSeek providers land in v0.7.17.x — adding a
// provider is a single Provide in internal/container/container.go.
//
// The package is intentionally tiny and dependency-free so it can
// be unit-tested in isolation and reused by other surfaces (chat
// pipeline, agent service) without dragging in the chat_pipeline
// package or any specific LLM client.
package llmstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Request is the wire shape the assistant service hands to the LLM
// provider. The provider decides how to format it (system prompt,
// RAG context, chat template, ...) — this package owns only the
// transport shape, not the prompt.
type Request struct {
	// Query is the user's question, exactly as the assistant received it.
	Query string
	// ConversationID threads a multi-turn conversation together. The
	// provider MAY use it to fetch prior turns from its own store;
	// the assistant service does NOT replay history here.
	ConversationID string
	// KBCitations are the KB hits the assistant's retrieval layer
	// returned, in relevance order. Providers that build a RAG prompt
	// SHOULD serialise these into the context window.
	KBCitations []types.AssistantCitation
	// WikiCitations are the Wiki hits, in the same shape.
	WikiCitations []types.AssistantCitation
	// ModelName lets the admin override the model per request. When
	// empty the provider's default model is used.
	ModelName string
	// MaxTokens caps the generated answer. Zero means "provider default".
	MaxTokens int
	// Temperature is in [0, 2]. Zero means "provider default".
	Temperature float64
	// SystemPrompt is an optional system-level instruction; providers
	// that don't support system prompts can ignore it.
	SystemPrompt string
}

// Response is what Complete returns: the full assembled answer.
type Response struct {
	AnswerText string
	// Usage statistics so the caller can persist them into the
	// assistant_conversations row.
	PromptTokens     int
	CompletionTokens int
	ModelName        string
	StartedAt        time.Time
	FinishedAt       time.Time
}

// Event is one chunk of a streaming response. The Type field
// discriminates; the data field carries the typed payload via the
// generic helpers.
type Event struct {
	Type EventType
	Data any
	// Error is populated when Type == EventError.
	Error error
}

// EventType is the small set of events the wire protocol knows about.
// Adding a new event type is a breaking change for clients, so do
// not add to this list lightly.
type EventType string

const (
	EventToken    EventType = "token"    // a chunk of generated text
	EventCitation EventType = "citation" // an additional citation discovered mid-stream
	EventDone     EventType = "done"     // the provider finished cleanly
	EventError    EventType = "error"    // the provider aborted
)

// TokenEventData carries one text chunk.
type TokenEventData struct {
	Text string
}

// CitationEventData carries one citation surfaced mid-stream (some
// providers discover extra citations during generation).
type CitationEventData struct {
	Index    int
	Citation types.AssistantCitation
}

// DoneEventData carries the final payload once generation is complete.
type DoneEventData struct {
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
}

// EventSink is the callback the provider calls for every event.
// The sink MUST be safe for concurrent use if the provider emits
// from multiple goroutines (current implementations emit serially).
type EventSink interface {
	OnEvent(Event) error
}

// FuncSink adapts a plain function into an EventSink, for tests.
type FuncSink func(Event) error

// OnEvent implements EventSink.
func (f FuncSink) OnEvent(e Event) error { return f(e) }

// Provider is the LLM-agnostic seam. The assistant service takes a
// Provider in its constructor; the container picks the concrete
// implementation at startup based on the model's provider name.
type Provider interface {
	// Complete is the synchronous path used by the v0.7.15 API
	// (POST /assistant/ask). It returns the full answer plus usage.
	Complete(ctx context.Context, req Request) (Response, error)

	// Stream is the v0.7.17+ path used by the SSE endpoint
	// (POST /assistant/ask?stream=1). It calls OnEvent on sink for
	// every EventToken / EventCitation / EventDone and returns nil
	// on clean completion, or the final error otherwise.
	Stream(ctx context.Context, req Request, sink EventSink) error

	// Name identifies the provider for logging / metrics / display.
	// Examples: "noop", "openai", "anthropic", "doubao", "qwen".
	Name() string
}

// ErrProviderUnavailable is the sentinel the assistant service
// maps to 503. Providers should return this when their backend is
// down or rate-limited; other errors surface as 500.
var ErrProviderUnavailable = errors.New("llmstream: provider unavailable")

// ErrProviderTimeout is the sentinel the assistant service maps to
// 504. Returned when ctx.Done() fires before the provider completes.
var ErrProviderTimeout = errors.New("llmstream: provider timeout")

// IsTransient returns true when err signals a transient failure
// (timeout / unavailable / rate-limit). The assistant service uses
// this to decide whether to retry, fall back to a placeholder, or
// surface the error to the user.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrProviderTimeout) {
		return true
	}
	// Provider-specific implementations are expected to wrap a
	// transient sentinel via fmt.Errorf("...: %w", ErrProviderTimeout).
	return errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrProviderTimeout)
}

// FormatSSEEvent renders an Event as one SSE frame (event: + data: +
// blank line), suitable for writing to an http.ResponseWriter with
// a Flusher. The data field is JSON-encoded; EventError's err.Error()
// is what lands in the "error" JSON key.
//
// This helper is exported so the handler and the smoke tests stay
// in sync — both call the same formatter.
func FormatSSEEvent(e Event) ([]byte, error) {
	switch e.Type {
	case EventToken:
		text, _ := e.Data.(TokenEventData)
		return []byte("event: token\ndata: " + jsonEncode(map[string]any{"text": text.Text}) + "\n\n"), nil
	case EventCitation:
		cit, _ := e.Data.(CitationEventData)
		return []byte("event: citation\ndata: " + jsonEncode(map[string]any{
			"index":    cit.Index,
			"citation": cit.Citation,
		}) + "\n\n"), nil
	case EventDone:
		done, _ := e.Data.(DoneEventData)
		return []byte("event: done\ndata: " + jsonEncode(map[string]any{
			"prompt_tokens":     done.PromptTokens,
			"completion_tokens": done.CompletionTokens,
			"finish_reason":     done.FinishReason,
		}) + "\n\n"), nil
	case EventError:
		msg := ""
		if e.Error != nil {
			msg = e.Error.Error()
		}
		return []byte("event: error\ndata: " + jsonEncode(map[string]any{"error": msg}) + "\n\n"), nil
	default:
		return nil, fmt.Errorf("llmstream: unknown event type %q", e.Type)
	}
}

// NoopProvider is the default provider used when no real LLM is
// configured. It emits the same placeholder text the v0.7.15
// AssistantService.composeAnswerPlaceholder produced, so existing
// frontends see no regression.
type NoopProvider struct{}

// Name implements Provider.
func (NoopProvider) Name() string { return "noop" }

// Complete implements Provider by emitting one big token event
// equivalent to the placeholder text.
func (p NoopProvider) Complete(ctx context.Context, req Request) (Response, error) {
	return Response{
		AnswerText: p.composePlaceholder(req.Query, len(req.KBCitations), len(req.WikiCitations)),
		ModelName:  "noop",
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}, nil
}

// Stream implements Provider by emitting a single token event then
// a done event. Useful for end-to-end smoke testing the SSE plumbing
// without an LLM.
func (p NoopProvider) Stream(ctx context.Context, req Request, sink EventSink) error {
	text := p.composePlaceholder(req.Query, len(req.KBCitations), len(req.WikiCitations))
	if err := sink.OnEvent(Event{Type: EventToken, Data: TokenEventData{Text: text}}); err != nil {
		return err
	}
	return sink.OnEvent(Event{Type: EventDone, Data: DoneEventData{FinishReason: "stop"}})
}

// composePlaceholder mirrors the v0.7.15
// AssistantService.composeAnswerPlaceholder. Keeping it duplicated
// here avoids an import cycle (assistant service will be the only
// caller in v0.7.17).
func (NoopProvider) composePlaceholder(query string, kbCount, wikiCount int) string {
	return fmt.Sprintf("Found %d KB citation(s) and %d wiki page(s) relevant to your question.",
		kbCount, wikiCount)
}
