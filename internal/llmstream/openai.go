package llmstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// OpenAIProvider is a Provider implementation backed by the OpenAI
// Chat Completions API (and OpenAI-compatible endpoints such as
// Azure OpenAI, vLLM, Ollama's OpenAI-compat shim, and Doubao's
// OpenAI-compat mode).
//
// The provider is intentionally small: it formats the RAG prompt,
// ships a Chat Completion Stream request, and forwards every SSE
// delta into the EventSink as an EventToken. When the underlying
// HTTP transport returns 4xx / 5xx / timeout the provider maps the
// cause onto ErrProviderUnavailable or ErrProviderTimeout so the
// assistant handler can return 503 / 504 without inspecting the
// error message.
//
// The provider is safe for concurrent use; one Provider instance
// can be shared across all assistant requests in a process.
type OpenAIProvider struct {
	client       *openai.Client
	defaultModel string
	now          func() time.Time
}

// OpenAIProviderOptions carries the knobs the constructor exposes.
// Zero values fall back to sensible defaults: gpt-4o-mini, the
// default openai endpoint, 30-second timeout.
type OpenAIProviderOptions struct {
	// APIKey is the OpenAI bearer token. Required.
	APIKey string
	// BaseURL lets you point the provider at a compatible endpoint
	// (Azure / vLLM / Doubao / Ollama OpenAI shim). When empty the
	// official OpenAI endpoint is used.
	BaseURL string
	// OrgID is the OpenAI organization ID. Optional.
	OrgID string
	// DefaultModel is the model used when Request.ModelName is empty.
	// Defaults to "gpt-4o-mini".
	DefaultModel string
	// HTTPClient overrides the default 30-second client. Optional.
	HTTPClient *http.Client
	// Now is the clock hook for tests.
	Now func() time.Time
}

// NewOpenAIProvider is the DI constructor. Returns an error when
// APIKey is empty so a misconfigured deployment fails loudly at
// startup instead of silently emitting the placeholder forever.
func NewOpenAIProvider(opts OpenAIProviderOptions) (*OpenAIProvider, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("llmstream: openai: APIKey required")
	}
	cfg := openai.DefaultConfig(opts.APIKey)
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	if opts.OrgID != "" {
		cfg.OrgID = opts.OrgID
	}
	if opts.HTTPClient != nil {
		cfg.HTTPClient = opts.HTTPClient
	}
	defaultModel := opts.DefaultModel
	if defaultModel == "" {
		defaultModel = "gpt-4o-mini"
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &OpenAIProvider{
		client:       openai.NewClientWithConfig(cfg),
		defaultModel: defaultModel,
		now:          now,
	}, nil
}

// Name implements Provider.
func (p *OpenAIProvider) Name() string { return "openai" }

// Complete implements Provider by sending a non-streaming Chat
// Completion request and returning the assembled answer.
func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (Response, error) {
	messages := p.formatMessages(req)
	model := p.modelFor(req)

	started := p.now()
	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
	})
	if err != nil {
		return Response{}, p.mapError(err)
	}
	finished := p.now()
	if len(resp.Choices) == 0 {
		return Response{ModelName: model, StartedAt: started.UTC(), FinishedAt: finished.UTC()}, nil
	}
	return Response{
		AnswerText:       resp.Choices[0].Message.Content,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		ModelName:        resp.Model,
		StartedAt:        started.UTC(),
		FinishedAt:       finished.UTC(),
	}, nil
}

// Stream implements Provider by opening a Chat Completion Stream and
// forwarding every SSE delta as an EventToken. The provider emits
// one final EventDone when the upstream stream closes cleanly, or
// one EventError followed by the mapped error on transport failure.
func (p *OpenAIProvider) Stream(ctx context.Context, req Request, sink EventSink) error {
	messages := p.formatMessages(req)
	model := p.modelFor(req)

	started := p.now()
	stream, err := p.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Stream:      true,
	})
	if err != nil {
		return p.mapError(err)
	}
	defer stream.Close()

	var (
		promptTokens     int
		completionTokens int
	)
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = sink.OnEvent(Event{Type: EventError, Error: p.mapError(err)})
			return p.mapError(err)
		}
		if len(resp.Choices) > 0 {
			delta := resp.Choices[0].Delta.Content
			if delta != "" {
				if err := sink.OnEvent(Event{Type: EventToken, Data: TokenEventData{Text: delta}}); err != nil {
					return err
				}
			}
		}
		// The library doesn't surface usage in stream responses
		// (OpenAI's `stream_options.include_usage` is a 2024-add);
		// we leave the token counters zero here and rely on the
		// caller to back-fill them via a non-streaming request if
		// needed. Future revision: pass StreamOptions.
		_ = promptTokens
		_ = completionTokens
	}
	finished := p.now()
	_ = started
	_ = finished
	return sink.OnEvent(Event{
		Type: EventDone,
		Data: DoneEventData{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			FinishReason:     "stop",
		},
	})
}

// modelFor returns the explicit Request.ModelName when supplied,
// otherwise the provider's default.
func (p *OpenAIProvider) modelFor(req Request) string {
	if req.ModelName != "" {
		return req.ModelName
	}
	return p.defaultModel
}

// formatMessages turns the assistant Request into the OpenAI
// messages shape. The system prompt is rendered from a template
// that prepends the KB + Wiki citations as a labelled context
// block. When the caller supplies their own SystemPrompt, it wins
// over the default — useful for power users who want a constrained
// behaviour.
func (p *OpenAIProvider) formatMessages(req Request) []openai.ChatCompletionMessage {
	system := req.SystemPrompt
	if system == "" {
		system = p.systemPrompt(req)
	}
	return []openai.ChatCompletionMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: req.Query},
	}
}

func (p *OpenAIProvider) systemPrompt(req Request) string {
	var b strings.Builder
	b.WriteString("You are WeKnora's assistant. Answer the user's question using ONLY the citations below.\n")
	b.WriteString("If the citations don't cover the question, say so explicitly.\n")
	b.WriteString("Cite the relevant sources inline as [KB: <title>] or [Wiki: <title>].\n\n")
	if len(req.KBCitations) > 0 {
		b.WriteString("KB citations:\n")
		for i, c := range req.KBCitations {
			fmt.Fprintf(&b, "  [%d] [KB] %s\n      %s\n", i+1, c.Title, c.Snippet)
		}
	}
	if len(req.WikiCitations) > 0 {
		b.WriteString("Wiki citations:\n")
		for i, c := range req.WikiCitations {
			fmt.Fprintf(&b, "  [%d] [Wiki] %s\n      %s\n", i+1, c.Title, c.Snippet)
		}
	}
	return b.String()
}

// mapError translates the OpenAI / HTTP error space into the
// assistant service's transient-vs-permanent contract.
func (p *OpenAIProvider) mapError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate limit"):
		return fmt.Errorf("openai rate-limited: %w", ErrProviderUnavailable)
	case strings.Contains(msg, "timeout"), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("openai timeout: %w", ErrProviderTimeout)
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"),
		strings.Contains(msg, "404"), strings.Contains(msg, "400"):
		// Permanent: bad API key / bad model / bad request shape.
		return fmt.Errorf("openai permanent: %w", err)
	default:
		// Default to transient so callers retry; the assistant
		// service degrades to the placeholder when this fires.
		return fmt.Errorf("openai transient: %w", ErrProviderUnavailable)
	}
}

// Compile-time assertion that OpenAIProvider satisfies Provider.
var _ Provider = (*OpenAIProvider)(nil)
