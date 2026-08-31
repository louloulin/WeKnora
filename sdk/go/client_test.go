package weknora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripFunc lets a test inject any response it wants.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestClient(t *testing.T, rt roundTripFunc) *Client {
	t.Helper()
	c, err := NewClient(context.Background(),
		WithBaseURL("https://example.test/v1"),
		WithBearerToken("test-token"),
		WithHTTPClient(&http.Client{Transport: rt}),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func jsonResp(status int, body any) *http.Response {
	buf, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(buf)),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}
}

func TestNewClient_RequiresBaseURL(t *testing.T) {
	_, err := NewClient(context.Background(), WithBearerToken("x"))
	if err == nil || !strings.Contains(err.Error(), "BaseURL") {
		t.Fatalf("expected BaseURL error, got %v", err)
	}
}

func TestNewClient_RequiresAuth(t *testing.T) {
	_, err := NewClient(context.Background(), WithBaseURL("https://x.test"))
	if err == nil || !strings.Contains(err.Error(), "authenticator") {
		t.Fatalf("expected authenticator error, got %v", err)
	}
}

func TestKnowledgeBase_Create(t *testing.T) {
	var calls int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}
		if req.URL.Path != "/v1/knowledge-bases" {
			t.Errorf("path = %s, want /knowledge-bases", req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer auth")
		}
		return jsonResp(http.StatusCreated, KnowledgeBase{
			ID:   "kb-1",
			Name: "Engineering",
			Type: "rag",
		}), nil
	})
	c := newTestClient(t, rt)
	kb, err := c.KnowledgeBase.Create(context.Background(), KnowledgeBaseInput{Name: "Engineering", Type: "rag"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if kb.ID != "kb-1" || kb.Name != "Engineering" {
		t.Fatalf("unexpected kb: %+v", kb)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestKnowledgeBase_NotFound(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusNotFound, map[string]string{"error": "kb not found"}), nil
	})
	c := newTestClient(t, rt)
	_, err := c.KnowledgeBase.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestKnowledgeBase_Iterate(t *testing.T) {
	pages := []Page[KnowledgeBase]{
		{Items: []KnowledgeBase{{ID: "a"}, {ID: "b"}}, NextPageToken: "next-1"},
		{Items: []KnowledgeBase{{ID: "c"}}, NextPageToken: ""},
	}
	var idx int
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		page := pages[idx]
		idx++
		return jsonResp(http.StatusOK, KnowledgeBasePage{
			Items: page.Items, NextPageToken: page.NextPageToken,
		}), nil
	})
	c := newTestClient(t, rt)
	it := c.KnowledgeBase.Iterate(context.Background(), 2)
	var ids []string
	for it.Next(context.Background()) {
		ids = append(ids, it.Item().ID)
	}
	if err := it.Err(); err != nil {
		t.Fatalf("iterator error: %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids[%d] = %s, want %s", i, ids[i], want[i])
		}
	}
}

func TestAutomation_Create(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/kb-1/databases/db-1/automations") {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return jsonResp(http.StatusCreated, Automation{
			ID:         "auto-1",
			Name:       "send-slack",
			DatabaseID: "db-1",
			TriggerType: TriggerRowChanged,
		}), nil
	})
	c := newTestClient(t, rt)
	auto, err := c.Automation.Create(context.Background(), "kb-1", AutomationInput{
		DatabaseID:  "db-1",
		Name:        "send-slack",
		TriggerType: TriggerRowChanged,
		Steps: []AutomationStep{{
			ID: "s1", ActionType: ActionNotify,
		}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if auto.ID != "auto-1" || auto.TriggerType != TriggerRowChanged {
		t.Fatalf("unexpected auto: %+v", auto)
	}
}

func TestFormula_Eval(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/kb-1/formula/eval") {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return jsonResp(http.StatusOK, FormulaEvalResponse{
			Value: 110.0, Type: "number",
		}), nil
	})
	c := newTestClient(t, rt)
	resp, err := c.Formula.Eval(context.Background(), "kb-1", FormulaEvalRequest{
		Expression: "price * (1 + tax_rate)",
		Context:    map[string]any{"price": 100.0, "tax_rate": 0.1},
	})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v, ok := resp.Value.(float64); !ok || v != 110.0 {
		t.Fatalf("unexpected value: %+v", resp.Value)
	}
}

func TestCollabDoc_UploadBytes(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/collaborative-docs/doc-1/upload") {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		if !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart, got %s", req.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "test bytes") {
			t.Errorf("missing payload in body: %s", body)
		}
		return jsonResp(http.StatusOK, CollabDocFile{
			DocID: "doc-1", Version: 7, SHA256: "abc", SizeBytes: 10,
		}), nil
	})
	c := newTestClient(t, rt)
	file, err := c.CollabDoc.UploadBytes(context.Background(), "doc-1", "application/octet-stream", []byte("test bytes"))
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if file.Version != 7 || file.SHA256 != "abc" {
		t.Fatalf("unexpected file: %+v", file)
	}
}

func TestAPIError_Is(t *testing.T) {
	cases := []struct {
		status int
		target error
		match  bool
	}{
		{http.StatusUnauthorized, ErrUnauthorized, true},
		{http.StatusForbidden, ErrForbidden, true},
		{http.StatusNotFound, ErrNotFound, true},
		{http.StatusTooManyRequests, ErrRateLimited, true},
		{http.StatusInternalServerError, ErrServer, true},
		{http.StatusOK, ErrNotFound, false},
	}
	for _, c := range cases {
		api := &APIError{StatusCode: c.status}
		got := errors.Is(api, c.target)
		if got != c.match {
			t.Errorf("Is(%d, %v) = %v, want %v", c.status, c.target, got, c.match)
		}
	}
}

func TestRetryPolicy_Backoff(t *testing.T) {
	p := RetryPolicy{InitialBackoff: 100 * time.Millisecond, MaxBackoff: 1 * time.Second}
	if got := p.Backoff(0); got != 100*time.Millisecond {
		t.Errorf("Backoff(0) = %v", got)
	}
	if got := p.Backoff(1); got != 200*time.Millisecond {
		t.Errorf("Backoff(1) = %v", got)
	}
	if got := p.Backoff(5); got > p.MaxBackoff {
		t.Errorf("Backoff(5) = %v exceeds max", got)
	}
}

func TestOptions_RejectEmptyTokens(t *testing.T) {
	if _, err := NewClient(context.Background(), WithBaseURL("https://x"), WithBearerToken("")); err == nil {
		t.Fatal("expected empty bearer token error")
	}
	if _, err := NewClient(context.Background(), WithBaseURL("https://x"), WithAPIKey("")); err == nil {
		t.Fatal("expected empty api key error")
	}
}

// Compile-time check: ensure Iterator exposes the expected methods.
var _ = func() bool {
	var it *Iterator[KnowledgeBase]
	_ = it.Next
	_ = it.Item
	_ = it.Err
	_ = url.Values{}
	return true
}()
