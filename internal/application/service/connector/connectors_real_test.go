package connector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- RSS ---

func TestRSSConnector_RejectsEmptyConfig(t *testing.T) {
	r := &RSSConnector{HTTPClient: http.DefaultClient, Now: time.Now}
	if _, err := r.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{}); err == nil {
		t.Fatal("expected error on empty config")
	}
}

func TestRSSConnector_FetchesFeed(t *testing.T) {
	const rss = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Test Feed</title>
<item>
<title>Hello world</title>
<link>https://example.com/a</link>
<guid>https://example.com/a</guid>
<pubDate>Mon, 30 Aug 2026 12:00:00 GMT</pubDate>
<description>First post body.</description>
</item>
<item>
<title>Second</title>
<link>https://example.com/b</link>
<guid>https://example.com/b</guid>
<pubDate>Tue, 31 Aug 2026 12:00:00 GMT</pubDate>
<description>Second body.</description>
</item>
</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, rss)
	}))
	defer srv.Close()

	r := NewRSSConnector()
	cfg := interfaces.ConnectorRuntimeConfig{
		Kind:       types.ConnectorRSS,
		ConfigJSON: `{"feed_url":"` + srv.URL + `","max_items":10}`,
	}
	got, err := r.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Title != "Hello world" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].URL != "https://example.com/a" {
		t.Errorf("url = %q", got[0].URL)
	}
	if !strings.Contains(got[0].Content, "First post body.") {
		t.Errorf("content missing body: %q", got[0].Content)
	}
	if got[0].Metadata["feed"] != "Test Feed" {
		t.Errorf("feed metadata = %q", got[0].Metadata["feed"])
	}
}

func TestRSSConnector_RespectsMaxItems(t *testing.T) {
	const rss = `<?xml version="1.0"?>
<rss version="2.0"><channel><title>F</title>
<item><title>a</title><link>u/1</link><guid>g1</guid></item>
<item><title>b</title><link>u/2</link><guid>g2</guid></item>
<item><title>c</title><link>u/3</link><guid>g3</guid></item>
</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, rss)
	}))
	defer srv.Close()
	r := NewRSSConnector()
	cfg := interfaces.ConnectorRuntimeConfig{
		Kind:       types.ConnectorRSS,
		ConfigJSON: `{"feed_url":"` + srv.URL + `","max_items":2}`,
	}
	got, err := r.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d items, want 2", len(got))
	}
}

// --- Confluence ---

func TestConfluenceConnector_RejectsEmptyConfig(t *testing.T) {
	c := NewConfluenceConnector()
	if _, err := c.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{}); err == nil {
		t.Fatal("expected error on empty config")
	}
}

func TestConfluenceConnector_FetchesPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Verify auth header.
		if got := req.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Errorf("missing Basic auth header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"id":        "10001",
					"title":     "Onboarding",
					"type":      "page",
					"url":       "https://x.atlassian.net/wiki/spaces/ENG/pages/10001",
					"excerpt":   "Welcome to <b>the team</b>",
					"history":   map[string]interface{}{"createdBy": map[string]interface{}{"displayName": "Alice"}},
					"version":   map[string]interface{}{"number": 7, "when": "2026-08-15T10:00:00Z"},
					"resultParentContainer": map[string]interface{}{"key": "ENG", "name": "Engineering"},
				},
			},
			"start": 0, "limit": 25, "size": 1,
		})
	}))
	defer srv.Close()

	c := NewConfluenceConnector()
	cfg := interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorConfluence,
		ConfigJSON: `{"base_url":"` + srv.URL + `","auth":{"type":"basic","email":"a@b.com","api_token":"x"},"space_key":"ENG"}`,
	}
	got, err := c.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d pages, want 1", len(got))
	}
	if got[0].Title != "Onboarding" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].Author != "Alice" {
		t.Errorf("author = %q", got[0].Author)
	}
	if got[0].Metadata["space"] != "ENG" {
		t.Errorf("space metadata = %q", got[0].Metadata["space"])
	}
	if !strings.Contains(got[0].Content, "the team") {
		t.Errorf("excerpt should be stripped of HTML tags: %q", got[0].Content)
	}
}

// --- Slack (real, with HTTP) ---

func TestSlackConnector_FallsBackToStubWhenNoToken(t *testing.T) {
	cfg := `{"channel":"C01234567","messages":[{"ts":"1700000001.000100","text":"hi","user":"U123"}]}`
	got, err := (&SlackConnector{}).Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorSlack, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Author != "U123" {
		t.Errorf("author = %q", got[0].Author)
	}
}

func TestSlackConnector_CallsRealAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer xoxb-test" {
			t.Errorf("bad token: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":       true,
			"messages": []map[string]interface{}{{"type": "message", "ts": "1700000001.000100", "text": "hello", "user": "U123"}},
		})
	}))
	defer srv.Close()

	s := &SlackConnector{
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: rewriteTransport{base: srv.URL},
		},
		Now: time.Now,
	}
	cfg := `{"bot_token":"xoxb-test","channel":"C01234567"}`
	got, err := s.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorSlack, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Content != "hello" {
		t.Errorf("content = %q", got[0].Content)
	}
}

// rewriteTransport rewrites every outbound URL to point at the test
// server, so we can stub slack.com without DNS hacks.
type rewriteTransport struct{ base string }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(t.base, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

// --- Webhook (real, with HMAC) ---

func TestWebhookConnector_RejectsBadSignature(t *testing.T) {
	const secret = "topsecret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-WeKnora-Signature", "deadbeef")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{{"id": "x", "title": "t", "content": "c"}},
		})
	}))
	defer srv.Close()

	w := NewWebhookConnector()
	cfg := `{"secret":"` + secret + `","queue_url":"` + srv.URL + `"}`
	_, err := w.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorWebhook, ConfigJSON: cfg,
	})
	if err == nil {
		t.Fatal("expected signature mismatch error")
	}
}

func TestWebhookConnector_AcceptsValidSignature(t *testing.T) {
	const secret = "topsecret"
	body := []byte(`{"items":[{"id":"x","title":"deploy v1","content":"ok","author":"ci"}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-WeKnora-Signature", sig)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	w := NewWebhookConnector()
	cfg := `{"secret":"` + secret + `","queue_url":"` + srv.URL + `"}`
	got, err := w.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorWebhook, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Title != "deploy v1" {
		t.Errorf("title = %q", got[0].Title)
	}
}

func TestWebhookConnector_FallsBackToStub(t *testing.T) {
	cfg := `{"items":[{"id":"e1","title":"hi","content":"body","author":"alice"}]}`
	got, err := (&WebhookConnector{}).Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{
		Kind: types.ConnectorWebhook, ConfigJSON: cfg,
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 1 || got[0].Title != "hi" {
		t.Fatalf("got %+v", got)
	}
}

// --- Ingester ---

type fakeKnowledgeService struct {
	lastPayload *types.ManualKnowledgePayload
	lastKBID    string
}

func (f *fakeKnowledgeService) CreateKnowledgeFromManual(ctx context.Context, kbID string, p *types.ManualKnowledgePayload, channel string) (*types.Knowledge, error) {
	f.lastKBID = kbID
	f.lastPayload = p
	return &types.Knowledge{ID: "k1"}, nil
}

func TestKnowledgeIngester_BridgesToKnowledgeService(t *testing.T) {
	fks := &fakeKnowledgeService{}
	ing := NewKnowledgeIngester(fks)
	if err := ing.Ingest(context.Background(), "1", "kb-1", "title", "body", "alice", "https://x", time.Now()); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if fks.lastKBID != "kb-1" {
		t.Errorf("kb id = %q", fks.lastKBID)
	}
	if fks.lastPayload == nil {
		t.Fatal("payload missing")
	}
	if !strings.Contains(fks.lastPayload.Content, "body") {
		t.Errorf("content missing body: %q", fks.lastPayload.Content)
	}
	if fks.lastPayload.Channel != "connector" {
		t.Errorf("channel = %q", fks.lastPayload.Channel)
	}
}

func TestKnowledgeIngester_RejectsEmptyTenant(t *testing.T) {
	ing := NewKnowledgeIngester(&fakeKnowledgeService{})
	if err := ing.Ingest(context.Background(), "", "kb-1", "t", "c", "", "", time.Now()); err == nil {
		t.Fatal("expected error on empty tenant")
	}
}

func TestKnowledgeIngester_RejectsBadTenant(t *testing.T) {
	ing := NewKnowledgeIngester(&fakeKnowledgeService{})
	if err := ing.Ingest(context.Background(), "not-a-number", "kb-1", "t", "c", "", "", time.Now()); err == nil {
		t.Fatal("expected error on bad tenant")
	}
}

func TestKnowledgeIngester_RejectsNilKnowledgeService(t *testing.T) {
	ing := NewKnowledgeIngester(nil)
	err := ing.Ingest(context.Background(), "1", "kb-1", "t", "c", "", "", time.Now())
	if err != ErrNoIngester {
		t.Errorf("err = %v, want %v", err, ErrNoIngester)
	}
}

// --- M365 (Build #30 G10) ---

func TestM365Connector_KindAndErrorOnEmptyConfig(t *testing.T) {
	m := NewM365Connector()
	if m.Kind() != types.ConnectorM365 {
		t.Fatalf("kind = %v, want m365", m.Kind())
	}
	if _, err := m.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{}); err == nil {
		t.Fatal("expected error on empty config (no token)")
	}
}

func TestM365Connector_FetchesMail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"value":[
			{"id":"msg-1","subject":"Hello","bodyPreview":"Hi there","receivedDateTime":"2026-08-30T12:00:00Z","from":{"emailAddress":{"name":"Alice","address":"alice@contoso.com"}}},
			{"id":"msg-2","subject":"World","bodyPreview":"Bye","receivedDateTime":"2026-08-30T13:00:00Z","sender":{"emailAddress":{"address":"bob@contoso.com"}}}
		]}`)
	}))
	defer srv.Close()
	m := &M365Connector{
		HTTPClient: srv.Client(),
		GraphBase:  srv.URL,
	}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"access_token":"t-1","resource_kind":"m365_email","user_principal":"alice@contoso.com","top":10}`,
	}
	msgs, err := m.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Author != "Alice <alice@contoso.com>" {
		t.Errorf("msg0 author = %q", msgs[0].Author)
	}
	if msgs[1].Author != "bob@contoso.com" {
		t.Errorf("msg1 author = %q", msgs[1].Author)
	}
	if msgs[0].Metadata["source"] != "m365_email" {
		t.Errorf("msg0 source = %q", msgs[0].Metadata["source"])
	}
	if msgs[0].Timestamp.IsZero() {
		t.Errorf("msg0 timestamp not parsed")
	}
}

func TestM365Connector_AuthExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_token"}`)
	}))
	defer srv.Close()
	m := &M365Connector{HTTPClient: srv.Client(), GraphBase: srv.URL}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"access_token":"stale","resource_kind":"m365_email","user_principal":"alice@contoso.com"}`,
	}
	_, err := m.Fetch(context.Background(), cfg)
	if err != ErrM365AuthExpired {
		t.Fatalf("err = %v, want ErrM365AuthExpired", err)
	}
}

func TestM365Connector_UnsupportedResourceKind(t *testing.T) {
	m := NewM365Connector()
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"access_token":"t-1","resource_kind":"m365_unknown"}`,
	}
	if _, err := m.Fetch(context.Background(), cfg); err == nil {
		t.Fatal("expected error on unsupported kind")
	}
}

// --- Google Workspace (Build #30 G10) ---

func TestGoogleWorkspaceConnector_KindAndErrorOnEmptyConfig(t *testing.T) {
	g := NewGoogleWorkspaceConnector()
	if g.Kind() != types.ConnectorGoogle {
		t.Fatalf("kind = %v, want google_workspace", g.Kind())
	}
	if _, err := g.Fetch(context.Background(), interfaces.ConnectorRuntimeConfig{}); err == nil {
		t.Fatal("expected error on empty config (no token)")
	}
}

func TestGoogleWorkspaceConnector_FetchesGmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer token")
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "/messages"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"messages":[{"id":"m-1"},{"id":"m-2"}]}`)
		case strings.Contains(req.URL.Path, "/messages/m-1"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"id":"m-1",
				"snippet":"Hello world snippet",
				"internalDate":"1700000000000",
				"payload":{"headers":[
					{"name":"Subject","value":"Subject 1"},
					{"name":"From","value":"alice@example.com"},
					{"name":"Date","value":"Mon, 30 Aug 2026 12:00:00 GMT"}
				]}
			}`)
		case strings.Contains(req.URL.Path, "/messages/m-2"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"m-2","snippet":"snip 2","payload":{"headers":[]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	g := &GoogleWorkspaceConnector{
		HTTPClient: srv.Client(),
		GmailBase:  srv.URL,
	}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_email","user_id":"alice@example.com","max_results":10}`,
	}
	msgs, err := g.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Title != "Subject 1" {
		t.Errorf("msg0 title = %q", msgs[0].Title)
	}
	if msgs[0].Author != "alice@example.com" {
		t.Errorf("msg0 author = %q", msgs[0].Author)
	}
	if msgs[0].Metadata["source"] != "google_email" {
		t.Errorf("msg0 source = %q", msgs[0].Metadata["source"])
	}
}

func TestGoogleWorkspaceConnector_FetchesCalendar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[
			{"id":"e-1","summary":"Standup","description":"Daily sync","htmlLink":"https://cal/e-1","start":{"dateTime":"2026-08-30T09:00:00Z"},"organizer":{"email":"alice@example.com"}}
		]}`)
	}))
	defer srv.Close()
	g := &GoogleWorkspaceConnector{
		HTTPClient: srv.Client(),
		CalBase:    srv.URL,
	}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_meeting","user_id":"primary","max_results":5}`,
	}
	msgs, err := g.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d events, want 1", len(msgs))
	}
	if msgs[0].Title != "Standup" {
		t.Errorf("event title = %q", msgs[0].Title)
	}
	if msgs[0].URL != "https://cal/e-1" {
		t.Errorf("event url = %q", msgs[0].URL)
	}
	if msgs[0].Metadata["source"] != "google_meeting" {
		t.Errorf("event source = %q", msgs[0].Metadata["source"])
	}
}

func TestGoogleWorkspaceConnector_FetchesDrive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"files":[
			{"id":"f-1","name":"report.pdf","mimeType":"application/pdf","modifiedTime":"2026-08-30T10:00:00Z","webViewLink":"https://drive/f-1"}
		]}`)
	}))
	defer srv.Close()
	g := &GoogleWorkspaceConnector{
		HTTPClient: srv.Client(),
		DriveBase:  srv.URL,
	}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_drive","max_results":5}`,
	}
	msgs, err := g.Fetch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d files, want 1", len(msgs))
	}
	if msgs[0].Title != "report.pdf" {
		t.Errorf("file name = %q", msgs[0].Title)
	}
	if msgs[0].URL != "https://drive/f-1" {
		t.Errorf("file url = %q", msgs[0].URL)
	}
	if msgs[0].Metadata["source"] != "google_drive" {
		t.Errorf("file source = %q", msgs[0].Metadata["source"])
	}
}

func TestGoogleWorkspaceConnector_AuthExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"forbidden"}}`)
	}))
	defer srv.Close()
	g := &GoogleWorkspaceConnector{
		HTTPClient: srv.Client(),
		GmailBase:  srv.URL,
	}
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_email","user_id":"alice@example.com"}`,
	}
	_, err := g.Fetch(context.Background(), cfg)
	if err != ErrGoogleAuthExpired {
		t.Fatalf("err = %v, want ErrGoogleAuthExpired", err)
	}
}

func TestGoogleWorkspaceConnector_RequiresUserIDForGmail(t *testing.T) {
	g := NewGoogleWorkspaceConnector()
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_email"}`,
	}
	if _, err := g.Fetch(context.Background(), cfg); err == nil {
		t.Fatal("expected error when user_id missing")
	}
}

func TestGoogleWorkspaceConnector_UnsupportedResourceKind(t *testing.T) {
	g := NewGoogleWorkspaceConnector()
	cfg := interfaces.ConnectorRuntimeConfig{
		ConfigJSON: `{"resource_kind":"google_unknown"}`,
	}
	if _, err := g.Fetch(context.Background(), cfg); err == nil {
		t.Fatal("expected error on unsupported kind")
	}
}

func TestParseGoogleDate(t *testing.T) {
	cases := map[string]bool{
		"2026-08-30T12:00:00Z":       true,
		"2026-08-30T12:00:00.123Z":   true,
		"Mon, 30 Aug 2026 12:00:00 GMT": true,
		"not a date":                 false,
		"":                           false,
	}
	for s, expectValid := range cases {
		tm := parseGoogleDate(s)
		if expectValid && tm.IsZero() {
			t.Errorf("parseGoogleDate(%q) returned zero time, want non-zero", s)
		}
		if !expectValid && !tm.IsZero() {
			t.Errorf("parseGoogleDate(%q) = %v, want zero time", s, tm)
		}
	}
}
