package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// capturingAuditService is the in-memory fake used by every test in
// this file. It records each Log call and optionally returns a synthetic
// error so we can drive the fire-and-forget path without a real DB.
// Embedding the interface keeps any future method-dispatch regression
// from silently no-op-ing — a method we forgot to fake becomes a nil
// deref panic at the call site.
type capturingAuditService struct {
	interfaces.AuditLogService
	mu      sync.Mutex
	entries []*types.AuditLog
	logErr  error
}

func (c *capturingAuditService) Log(_ context.Context, entry *types.AuditLog) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
	return c.logErr
}

func (c *capturingAuditService) snapshot() []*types.AuditLog {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*types.AuditLog, len(c.entries))
	copy(out, c.entries)
	return out
}

// newCitationLogRouter builds a minimal gin engine with the route
// registered, the audit dependency pre-wired, and the auth context
// fields (tenant + user) stamped via a synthetic middleware. We do
// NOT exercise the full RBAC chain here — that lives in router_wiki_test
// style files — because the handler only needs the context values.
func newCitationLogRouter(audit interfaces.AuditLogService, tenantID uint64, userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		if tenantID != 0 {
			ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)
		}
		if userID != "" {
			ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewCitationLogHandler(audit)
	r.POST("/citation-log", h.LogCitationAccess)
	return r
}

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestCitationLog_NilAuditSvcIsNoop pins the documented degraded mode:
// the handler must return 200 even when auditSvc is nil so unit tests
// of the broader chat pipeline stay lightweight, and so a misconfigured
// container does not silently crash the chat UX.
func TestCitationLog_NilAuditSvcIsNoop(t *testing.T) {
	r := newCitationLogRouter(nil, 42, "u-1")
	w := postJSON(r, "/citation-log", CitationLogRequest{
		KnowledgeBaseID: "kb-1", ChunkID: "c-1",
		SourceMessageID: "m-1", CitationIndex: 1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("nil audit svc must still 200; got %d body=%s", w.Code, w.Body.String())
	}
}

// TestCitationLog_HappyPathWritesAudit pins the canonical row shape:
// tenant from context, action = chat.citation_accessed, scope_type /
// scope_id = knowledge_base / kb_id, target_type = citation,
// target_id = chunk_id, details JSON carrying all four wire fields
// (chunk_id / source_message_id / citation_index / kb_id) plus the
// optional title.
func TestCitationLog_HappyPathWritesAudit(t *testing.T) {
	audit := &capturingAuditService{}
	r := newCitationLogRouter(audit, 42, "u-1")
	w := postJSON(r, "/citation-log", CitationLogRequest{
		KnowledgeBaseID: "kb-7", ChunkID: "chunk-99",
		SourceMessageID: "msg-3", CitationIndex: 2, Title: "Sales playbook",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d body=%s", w.Code, w.Body.String())
	}

	// The write is fire-and-forget; allow the goroutine a moment to land.
	waitForEntries(t, audit, 1)
	entries := audit.snapshot()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(entries))
	}
	e := entries[0]
	if e.TenantID != 42 {
		t.Errorf("TenantID = %d, want 42", e.TenantID)
	}
	if e.Action != types.AuditActionChatCitationAccessed {
		t.Errorf("Action = %q, want %q", e.Action, types.AuditActionChatCitationAccessed)
	}
	if e.ScopeType != "knowledge_base" {
		t.Errorf("ScopeType = %q, want knowledge_base", e.ScopeType)
	}
	if e.ScopeID != "kb-7" {
		t.Errorf("ScopeID = %q, want kb-7", e.ScopeID)
	}
	if e.TargetType != "citation" {
		t.Errorf("TargetType = %q, want citation", e.TargetType)
	}
	if e.TargetID != "chunk-99" {
		t.Errorf("TargetID = %q, want chunk-99", e.TargetID)
	}
	if e.ActorUserID != "u-1" {
		t.Errorf("ActorUserID = %q, want u-1", e.ActorUserID)
	}
	if e.Outcome != types.AuditOutcomeSuccess {
		t.Errorf("Outcome = %q, want success", e.Outcome)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(e.Details), &details); err != nil {
		t.Fatalf("Details not valid JSON: %v", err)
	}
	for k, want := range map[string]any{
		"chunk_id":          "chunk-99",
		"source_message_id": "msg-3",
		"citation_index":    float64(2),
		"kb_id":             "kb-7",
		"title":             "Sales playbook",
	} {
		if got := details[k]; got != want {
			t.Errorf("Details[%q] = %v, want %v", k, got, want)
		}
	}
}

// TestCitationLog_RejectsInvalidBodies covers the four required-field
// validation gates the handler enforces up front. The whole point of
// validating inside the handler (rather than at the repo boundary) is
// to avoid spinning up the audit goroutine for a request that was
// never going to land.
func TestCitationLog_RejectsInvalidBodies(t *testing.T) {
	audit := &capturingAuditService{}
	r := newCitationLogRouter(audit, 42, "u-1")

	cases := []struct{
		name string
		req CitationLogRequest
	}{
		{"missing kb_id", CitationLogRequest{ChunkID: "c", SourceMessageID: "m", CitationIndex: 1}},
		{"missing chunk_id", CitationLogRequest{KnowledgeBaseID: "kb", SourceMessageID: "m", CitationIndex: 1}},
		{"missing source_message_id", CitationLogRequest{KnowledgeBaseID: "kb", ChunkID: "c", CitationIndex: 1}},
		{"zero citation_index", CitationLogRequest{KnowledgeBaseID: "kb", ChunkID: "c", SourceMessageID: "m"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(r, "/citation-log", tc.req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400; got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if entries := audit.snapshot(); len(entries) != 0 {
		t.Fatalf("validation failures must not write audit rows; got %d", len(entries))
	}
}

// TestCitationLog_RejectsMissingTenant verifies the tenant gate. The
// auth middleware is responsible for stamping the tenant id, so a 401
// here only happens when a request slips through without a tenant
// (e.g. unit-test wiring or a misconfigured middleware chain). We
// fail closed so a stray request never lands an audit row into the
// default tenant=0 bucket.
func TestCitationLog_RejectsMissingTenant(t *testing.T) {
	audit := &capturingAuditService{}
	r := newCitationLogRouter(audit, 0, "u-1")
	w := postJSON(r, "/citation-log", CitationLogRequest{
		KnowledgeBaseID: "kb-1", ChunkID: "c-1",
		SourceMessageID: "m-1", CitationIndex: 1,
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401; got %d body=%s", w.Code, w.Body.String())
	}
	if entries := audit.snapshot(); len(entries) != 0 {
		t.Fatalf("missing-tenant request must not write audit rows; got %d", len(entries))
	}
}

// TestCitationLog_FireAndForgetOnAuditError verifies the async semantics
// (D9): even when auditSvc.Log returns an error, the handler still
// returns 200 to the caller. The audit goroutine logs the failure;
// the chat UX never observes an audit outage.
func TestCitationLog_FireAndForgetOnAuditError(t *testing.T) {
	audit := &capturingAuditService{logErr: errAuditSinkDown}
	r := newCitationLogRouter(audit, 42, "u-1")
	w := postJSON(r, "/citation-log", CitationLogRequest{
		KnowledgeBaseID: "kb-1", ChunkID: "c-1",
		SourceMessageID: "m-1", CitationIndex: 1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("audit failure must not 5xx the client; got %d body=%s", w.Code, w.Body.String())
	}
	waitForEntries(t, audit, 1)
}

// TestCitationLog_StampsCorrelationID verifies the Build #25 contract:
// a request carrying X-Request-ID lands in audit_logs with the same
// value on the correlation_id column. We inject the value via the
// middleware-equivalent helper because middleware.RequestID is not in
// the unit-test gin chain.
func TestCitationLog_StampsCorrelationID(t *testing.T) {
	audit := &capturingAuditService{}
	r := newCitationLogRouter(audit, 42, "u-1")

	buf, _ := json.Marshal(CitationLogRequest{
		KnowledgeBaseID: "kb-1", ChunkID: "c-1",
		SourceMessageID: "m-1", CitationIndex: 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/citation-log", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-correlation-abc123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d body=%s", w.Code, w.Body.String())
	}
	waitForEntries(t, audit, 1)
	e := audit.snapshot()[0]
	if e.CorrelationID != "test-correlation-abc123" {
		t.Errorf("CorrelationID = %q, want test-correlation-abc123", e.CorrelationID)
	}
}

// waitForEntries polls the capturing audit service until the goroutine
// has landed or a deadline elapses. Keeps test latency short while
// staying robust against slow CI schedules. Failure to land surfaces
// as a clear "expected N entries, got M" rather than a flappy
// out-of-order panic.
func waitForEntries(t *testing.T, audit *capturingAuditService, want int) {
	t.Helper()
	const deadline = 2 * time.Second
	start := time.Now()
	for time.Since(start) < deadline {
		if got := len(audit.snapshot()); got >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected %d audit rows, got %d", want, len(audit.snapshot()))
}

// errAuditSinkDown is a sentinel error returned by the fake audit
// service when the test wants to verify fire-and-forget semantics.
var errAuditSinkDown = &auditSinkError{}

type auditSinkError struct{}

func (e *auditSinkError) Error() string { return "audit sink down" }