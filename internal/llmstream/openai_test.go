package llmstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/Tencent/WeKnora/internal/llmstream"
)

// fakeOpenAIServer is a tiny stub of the OpenAI Chat Completions
// endpoint. It serves both the streaming (text/event-stream) and
// the non-streaming (application/json) response shapes so the
// provider tests can hit it directly.
type fakeOpenAIServer struct {
	server *httptest.Server
	// mode: "stream" | "complete" | "rate-limit" | "timeout" | "bad-key"
	mode       string
	requestHit int32
}

func newFakeOpenAIServer(mode string) *fakeOpenAIServer {
	f := &fakeOpenAIServer{mode: mode}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeOpenAIServer) handle(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&f.requestHit, 1)
	switch f.mode {
	case "stream":
		f.streamOK(w, r)
	case "complete":
		f.completeOK(w, r)
	case "rate-limit":
		http.Error(w, `{"error":{"message":"rate limit exceeded"}}`, http.StatusTooManyRequests)
	case "timeout":
		// Stall until the client gives up.
		time.Sleep(2 * time.Second)
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	case "bad-key":
		http.Error(w, `{"error":{"message":"incorrect api key"}}`, http.StatusUnauthorized)
	default:
		http.Error(w, "unknown mode", http.StatusInternalServerError)
	}
}

func (f *fakeOpenAIServer) streamOK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	deliver := func(payload string) {
		fmt.Fprintf(w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	}
	deliver(`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant","content":""},"index":0}]}`)
	deliver(`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"},"index":0}]}`)
	deliver(`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"delta":{"content":" world"},"index":0}]}`)
	deliver(`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop","index":0}]}`)
	deliver(`[DONE]`)
}

func (f *fakeOpenAIServer) completeOK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":"chatcmpl-2","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`)
}

func (f *fakeOpenAIServer) close() {
	f.server.Close()
}

func newTestProvider(t *testing.T, mode string) (*llmstream.OpenAIProvider, *fakeOpenAIServer) {
	t.Helper()
	srv := newFakeOpenAIServer(mode)
	t.Cleanup(srv.close)
	p, err := llmstream.NewOpenAIProvider(llmstream.OpenAIProviderOptions{
		APIKey:       "sk-test",
		BaseURL:      srv.server.URL + "/v1",
		DefaultModel: "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	return p, srv
}

func TestOpenAIProvider_Name(t *testing.T) {
	p, _ := newTestProvider(t, "complete")
	if got := p.Name(); got != "openai" {
		t.Fatalf("Name = %q, want openai", got)
	}
}

func TestNewOpenAIProvider_RequiresAPIKey(t *testing.T) {
	if _, err := llmstream.NewOpenAIProvider(llmstream.OpenAIProviderOptions{}); err == nil {
		t.Fatalf("expected error when APIKey is empty")
	}
}

func TestOpenAIProvider_Complete_AssemblesAnswer(t *testing.T) {
	p, srv := newTestProvider(t, "complete")
	resp, err := p.Complete(context.Background(), llmstream.Request{
		Query: "hi",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.AnswerText != "hi" {
		t.Fatalf("AnswerText = %q, want %q", resp.AnswerText, "hi")
	}
	if resp.PromptTokens != 7 || resp.CompletionTokens != 2 {
		t.Fatalf("usage = (%d,%d), want (7,2)", resp.PromptTokens, resp.CompletionTokens)
	}
	if atomic.LoadInt32(&srv.requestHit) != 1 {
		t.Fatalf("server should have been hit exactly once")
	}
}

func TestOpenAIProvider_Stream_ForwardsTokensAndDone(t *testing.T) {
	p, _ := newTestProvider(t, "stream")
	var got []llmstream.EventType
	var collected strings.Builder
	sink := llmstream.FuncSink(func(e llmstream.Event) error {
		got = append(got, e.Type)
		if e.Type == llmstream.EventToken {
			if d, ok := e.Data.(llmstream.TokenEventData); ok {
				collected.WriteString(d.Text)
			}
		}
		return nil
	})
	err := p.Stream(context.Background(), llmstream.Request{Query: "say hi"}, sink)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	wantText := "Hello world"
	if collected.String() != wantText {
		t.Fatalf("collected text = %q, want %q", collected.String(), wantText)
	}
	// Stream emits one token per non-empty delta (3 here: "", "Hello", " world") +
	// the final Done. The empty first delta is filtered out by the
	// provider's `if delta != ""` guard.
	tokenCount, doneCount := 0, 0
	for _, t := range got {
		switch t {
		case llmstream.EventToken:
			tokenCount++
		case llmstream.EventDone:
			doneCount++
		}
	}
	if tokenCount != 2 || doneCount != 1 {
		t.Fatalf("event counts = (token=%d, done=%d), want (2, 1); sequence = %v", tokenCount, doneCount, got)
	}
}

func TestOpenAIProvider_Complete_RateLimitMappedToUnavailable(t *testing.T) {
	p, _ := newTestProvider(t, "rate-limit")
	_, err := p.Complete(context.Background(), llmstream.Request{Query: "x"})
	if !errors.Is(err, llmstream.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestOpenAIProvider_Complete_BadKeyIsPermanent(t *testing.T) {
	p, _ := newTestProvider(t, "bad-key")
	_, err := p.Complete(context.Background(), llmstream.Request{Query: "x"})
	if errors.Is(err, llmstream.ErrProviderUnavailable) || errors.Is(err, llmstream.ErrProviderTimeout) {
		t.Fatalf("bad-key should be permanent, got transient: %v", err)
	}
	if err == nil {
		t.Fatalf("expected error from bad-key response")
	}
}

func TestOpenAIProvider_Stream_RespectsContextCancellation(t *testing.T) {
	p, _ := newTestProvider(t, "timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := p.Stream(ctx, llmstream.Request{Query: "x"}, llmstream.FuncSink(func(llmstream.Event) error { return nil }))
	if err == nil {
		t.Fatalf("expected error on timeout, got nil")
	}
	if !errors.Is(err, llmstream.ErrProviderTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		// Some HTTP client errors are returned as plain errors; mapError
		// converts the recognised ones. If the underlying transport
		// returned context.DeadlineExceeded we accept that too.
		t.Fatalf("expected timeout-class error, got %v", err)
	}
}

func TestOpenAIProvider_SystemPromptCarriesCitations(t *testing.T) {
	// Indirect: pass a non-empty SystemPrompt and verify the
	// provider accepts it (no shape constraint to test in this
	// unit; full coverage would need a request-body recorder).
	p, _ := newTestProvider(t, "complete")
	resp, err := p.Complete(context.Background(), llmstream.Request{
		Query:        "x",
		SystemPrompt: "Answer in haiku.",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.AnswerText == "" {
		t.Fatalf("expected non-empty answer")
	}
}

func TestOpenAIProvider_OpenAIImportCompatible(t *testing.T) {
	// Sanity: the sashabaranov/openai package exposes the same
	// ClientConfig shape we use in NewOpenAIProvider; this test
	// exists so a future bump that breaks the shape gets caught
	// before it hits main.
	var cfg openai.ClientConfig = openai.DefaultConfig("sk-x")
	if cfg.APIType != openai.APITypeOpenAI {
		t.Fatalf("openai DefaultConfig APIType changed")
	}
}
