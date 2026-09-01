package doc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tdcore "github.com/Tencent/WeKnora/internal/datasource/connector/tencentdocs/core"
	"github.com/Tencent/WeKnora/internal/types"
)

// TestType verifies the connector advertises the canonical type identifier
// the container registry registers against.
func TestType(t *testing.T) {
	if got, want := NewConnector().Type(), tdcore.ConnectorTypeTencentDocs; got != want {
		t.Errorf("Type = %q, want %q", got, want)
	}
}

// TestValidate_RejectsMissingCredentials verifies Validate surfaces a clear
// error when neither client_id nor access_token is supplied.
func TestValidate_RejectsMissingCredentials(t *testing.T) {
	c := NewConnector()
	if err := c.Validate(context.Background(), &types.DataSourceConfig{
		Type:        tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{},
	}); err == nil || !strings.Contains(err.Error(), "client_id or access_token") {
		t.Fatalf("Validate error = %v, want credentials-required", err)
	}
}

// TestValidate_SuccessfulPing spins up a stub OAuth token endpoint and
// verifies the connector issues a token exchange with the expected body.
func TestValidate_SuccessfulPing(t *testing.T) {
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			if r.PostForm.Get("client_id") != "cid" || r.PostForm.Get("client_secret") != "csec" {
				t.Errorf("unexpected form: %+v", r.PostForm)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"stub-tok","expires_in":7200,"token_type":"Bearer"}`))
		case "/drive/v2/files":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errcode":0,"data":{"files":[],"has_more":false}}`))
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &types.DataSourceConfig{
		Type: tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{
			"client_id":     "cid",
			"client_secret": "csec",
		},
		Settings: map[string]interface{}{
			"base_url": srv.URL,
		},
	}
	c := &Connector{clientFactory: func(c *tdcore.Config) *tdcore.Client {
		return tdcore.NewClientWithHTTPClient(c, srv.Client())
	}}
	if err := c.Validate(context.Background(), cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if tokenCalls != 1 {
		t.Errorf("tokenCalls = %d, want 1", tokenCalls)
	}
}

// TestListResources_FlattensDriveListing verifies the picker adapter
// flattens a drive listing into Resource records with doc URL and metadata.
func TestListResources_FlattensDriveListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/v2/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":7200}`))
		case strings.HasPrefix(r.URL.Path, "/drive/v2/files"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"errcode":0,
				"data":{
					"files":[
						{"id":"Dabc","type":"doc","title":"Hello","updated_at":"2025-06-01T10:00:00Z","owner":"alice"},
						{"id":"Ddef","type":"doc","title":"World","updated_at":"2025-06-02T11:00:00Z","owner":"bob"}
					],
					"has_more":false
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &types.DataSourceConfig{
		Type: tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{
			"access_token": "tok",
		},
		Settings: map[string]interface{}{"base_url": srv.URL},
	}
	c := &Connector{clientFactory: func(c *tdcore.Config) *tdcore.Client {
		return tdcore.NewClientWithHTTPClient(c, srv.Client())
	}}
	resources, err := c.ListResources(context.Background(), cfg, "")
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("len(resources) = %d, want 2", len(resources))
	}
	if resources[0].ExternalID != "Dabc" || resources[0].Name != "Hello" {
		t.Errorf("resources[0] = %+v", resources[0])
	}
	if resources[0].URL != "https://docs.qq.com/doc/Dabc" {
		t.Errorf("resources[0].URL = %q", resources[0].URL)
	}
	if resources[0].Metadata["owner"] != "alice" {
		t.Errorf("resources[0].Metadata[owner] = %v", resources[0].Metadata["owner"])
	}
}

// TestListResources_NonEmptyParent verifies the lazy-load contract.
func TestListResources_NonEmptyParent(t *testing.T) {
	c := NewConnector()
	got, err := c.ListResources(context.Background(), &types.DataSourceConfig{
		Type:        tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{"access_token": "x"},
	}, "any-folder-id")
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (no children for v1)", len(got))
	}
}

// TestResolveResourceAncestors verifies the v1 flat-picker contract.
func TestResolveResourceAncestors(t *testing.T) {
	c := NewConnector()
	got, err := c.ResolveResourceAncestors(context.Background(), &types.DataSourceConfig{
		Type:        tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{"access_token": "x"},
	}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("ResolveResourceAncestors: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// TestFetchIncremental_EmptyResourceIDs verifies the defensive guard.
func TestFetchIncremental_EmptyResourceIDs(t *testing.T) {
	c := NewConnector()
	_, _, err := c.FetchIncremental(context.Background(), &types.DataSourceConfig{
		Type:        tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{"access_token": "x"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "no resource IDs") {
		t.Fatalf("error = %v, want no-resource-IDs", err)
	}
}

// TestFetchStream_EmptyResourceIDs mirrors the FetchIncremental guard.
func TestFetchStream_EmptyResourceIDs(t *testing.T) {
	c := NewConnector()
	_, err := c.FetchStream(context.Background(), &types.DataSourceConfig{
		Type:        tdcore.ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{"access_token": "x"},
	}, nil, &noopStreamHandler{})
	if err == nil || !strings.Contains(err.Error(), "no resource IDs") {
		t.Fatalf("error = %v, want no-resource-IDs", err)
	}
}

// TestDocOps_AdapterContract locks in the NodeOps field-by-field contract.
func TestDocOps_AdapterContract(t *testing.T) {
	d := tdcore.Document{
		ID:        "D123",
		Type:      "doc",
		Title:     "Plan",
		UpdatedAt: time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC),
	}
	ops := docOps{}
	if got, want := ops.Token(d), "D123"; got != want {
		t.Errorf("Token = %q, want %q", got, want)
	}
	if got, want := ops.Title(d), "Plan"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if got, want := ops.ObjType(d), "doc"; got != want {
		t.Errorf("ObjType = %q, want %q", got, want)
	}
	if got, want := ops.EditTime(d), "2025-06-01T10:00:00Z"; got != want {
		t.Errorf("EditTime = %q, want %q", got, want)
	}
	if got := ops.ResourceNoun(); got != "documents" {
		t.Errorf("ResourceNoun = %q", got)
	}
	if got := ops.LogTag(); got != "[TencentDocs]" {
		t.Errorf("LogTag = %q", got)
	}
	if got := ops.EmptyResourceIDsError(); !strings.Contains(got, "tencent_docs") {
		t.Errorf("EmptyResourceIDsError = %q, want 'tencent_docs' in text", got)
	}
}

// noopStreamHandler is a StreamHandler used by the empty-ResourceIDs guard
// tests. Emit / Checkpoint must never be reached on the empty path.
type noopStreamHandler struct{}

func (h *noopStreamHandler) Emit(_ context.Context, _ types.FetchedItem) error {
	return nil
}
func (h *noopStreamHandler) Checkpoint(_ context.Context, _ *types.SyncCursor) error {
	return nil
}
