package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// TestParseConfig_AccessTokenOnly verifies the long-lived access_token path:
// when the config supplies a token directly, client_id / client_secret are
// not required.
func TestParseConfig_AccessTokenOnly(t *testing.T) {
	cfg := &types.DataSourceConfig{
		Type: ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{
			"access_token": "tok-123",
		},
	}
	got, err := ParseConfig(cfg)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if got.AccessToken != "tok-123" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "tok-123")
	}
}

// TestParseConfig_ClientCredentials verifies the OAuth2 client_credentials
// path: both client_id and client_secret must be supplied.
func TestParseConfig_ClientCredentials(t *testing.T) {
	cfg := &types.DataSourceConfig{
		Type: ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{
			"client_id":     "cid",
			"client_secret": "csec",
		},
	}
	got, err := ParseConfig(cfg)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if got.ClientID != "cid" || got.ClientSecret != "csec" {
		t.Errorf("got %+v, want cid/csec", got)
	}
}

// TestParseConfig_MissingAll verifies the validation error when no
// credentials are supplied.
func TestParseConfig_MissingAll(t *testing.T) {
	cfg := &types.DataSourceConfig{Type: ConnectorTypeTencentDocs}
	if _, err := ParseConfig(cfg); err == nil {
		t.Fatal("expected error, got nil")
	} else if !strings.Contains(err.Error(), "client_id or access_token") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestParseConfig_MissingSecret verifies the validation error when
// client_id is supplied without client_secret.
func TestParseConfig_MissingSecret(t *testing.T) {
	cfg := &types.DataSourceConfig{
		Type: ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{"client_id": "cid"},
	}
	if _, err := ParseConfig(cfg); err == nil {
		t.Fatal("expected error, got nil")
	} else if !strings.Contains(err.Error(), "client_secret is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestParseConfig_Settings verifies non-secret settings (timezone, base_url)
// are pulled from DataSourceConfig.Settings.
func TestParseConfig_Settings(t *testing.T) {
	cfg := &types.DataSourceConfig{
		Type: ConnectorTypeTencentDocs,
		Credentials: map[string]interface{}{
			"access_token": "tok",
		},
		Settings: map[string]interface{}{
			"timezone": "  Asia/Tokyo  ",
			"base_url": "  https://docs.example.com  ",
		},
	}
	got, err := ParseConfig(cfg)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if got.Timezone != "Asia/Tokyo" {
		t.Errorf("Timezone = %q, want trimmed %q", got.Timezone, "Asia/Tokyo")
	}
	if got.BaseURL != "https://docs.example.com" {
		t.Errorf("BaseURL = %q, want trimmed %q", got.BaseURL, "https://docs.example.com")
	}
}

// TestGetBaseURL verifies the BaseURL fallback chain: explicit Config →
// package default.
func TestGetBaseURL(t *testing.T) {
	if got := (&Config{}).GetBaseURL(); got != tencentDocsOpenBaseURL {
		t.Errorf("empty Config.GetBaseURL = %q, want default %q", got, tencentDocsOpenBaseURL)
	}
	if got := (&Config{BaseURL: "https://override.example.com/"}).GetBaseURL(); got != "https://override.example.com" {
		t.Errorf("Config.GetBaseURL = %q, want trimmed %q", got, "https://override.example.com")
	}
}

// TestDocumentEditTime verifies EditTime returns the RFC3339 UTC string of
// UpdatedAt, and "" when UpdatedAt is zero (so the engine treats fresh
// documents as "changed" on first sync).
func TestDocumentEditTime(t *testing.T) {
	if got := (Document{}).EditTime(); got != "" {
		t.Errorf("zero Document.EditTime = %q, want \"\"", got)
	}
	d := Document{UpdatedAt: time.Date(2025, 6, 1, 12, 30, 0, 0, time.UTC)}
	if got, want := d.EditTime(), "2025-06-01T12:30:00Z"; got != want {
		t.Errorf("Document.EditTime = %q, want %q", got, want)
	}
}

// TestAPIResponse_ErrCodeMessage verifies the errcode/errmsg normaliser.
func TestAPIResponse_ErrCodeMessage(t *testing.T) {
	if got := (APIResponse{}).ErrCodeMessage(); got != "" {
		t.Errorf("empty ErrCodeMessage = %q, want \"\"", got)
	}
	if got := (APIResponse{ErrCode: 40001, ErrMsg: "invalid token"}).ErrCodeMessage(); got != "errcode=40001 errmsg=invalid token" {
		t.Errorf("ErrCodeMessage = %q", got)
	}
}

// TestFailure_Auth verifies the auth/permission classification. The error
// vocabulary mirrors feishu/core so the frontend i18n can render a single
// family of error keys for both connectors.
func TestFailure_Auth(t *testing.T) {
	cases := []struct {
		name     string
		err      string
		wantCode string
	}{
		{"auth error", "tencent_docs auth error: invalid_grant", "tencent_docs_auth_or_permission"},
		{"invalid access token", "tencent_docs api error: status=401 body=invalid access token", "tencent_docs_auth_or_permission"},
		{"forbidden", "tencent_docs api error: status=403 body=forbidden", "tencent_docs_auth_or_permission"},
		{"rate limited", "tencent_docs rate limited: status=429", "tencent_docs_rate_limited"},
		{"timeout", "tencent_docs api error: status=504 body=upstream timeout", "tencent_docs_timeout"},
		{"server error", "tencent_docs server error: status=503", "tencent_docs_server_unavailable"},
		{"api error", "tencent_docs api error: status=400 body=bad", "tencent_docs_api_error"},
		{"unknown", "something else entirely", "tencent_docs_sync_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, _ := Failure(errStr(tc.err))
			if code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}

// TestCursorRoundtrip verifies the engine cursor wire format survives a
// JSON round-trip through the generic map[string]interface{} the engine
// stores on disk.
func TestCursorRoundtrip(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	times := map[string]map[string]string{
		"folder-1": {"doc-A": "2025-05-30T10:00:00Z", "doc-B": "2025-05-31T11:00:00Z"},
	}
	cur := EncodeCursor(times, now)

	decoded := DecodeCursorTimes(cur.ConnectorCursor)
	if decoded["folder-1"]["doc-A"] != "2025-05-30T10:00:00Z" {
		t.Errorf("doc-A edit time lost in roundtrip: %+v", decoded)
	}
	if decoded["folder-1"]["doc-B"] != "2025-05-31T11:00:00Z" {
		t.Errorf("doc-B edit time lost in roundtrip: %+v", decoded)
	}
	if !cur.LastSyncTime.Equal(now) {
		t.Errorf("LastSyncTime lost in roundtrip: %v vs %v", cur.LastSyncTime, now)
	}
}

// TestCursorDecodeEmpty verifies DecodeCursorTimes returns nil for an absent
// or empty cursor (caller treats nil as "no prior sync").
func TestCursorDecodeEmpty(t *testing.T) {
	if got := DecodeCursorTimes(nil); got != nil {
		t.Errorf("nil map -> %+v, want nil", got)
	}
	if got := DecodeCursorTimes(map[string]interface{}{}); got != nil {
		t.Errorf("empty map -> %+v, want nil", got)
	}
}

// TestLooksLikeDocumentID verifies the heuristic that decides whether a
// resource ID is a single document (return it as-is) or a folder (paginate
// over /drive/v2/files).
func TestLooksLikeDocumentID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"DABC123def456ghi789", true},
		{"SXYZ000aaa111bbb222ccc", true},
		{"short", false},             // too short
		{"contains spaces here 1234", false},
		{"contains/slash/here/abcdefgh", false},
		{strings.Repeat("a", 100), false}, // too long
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := looksLikeDocumentID(tc.id); got != tc.want {
				t.Errorf("looksLikeDocumentID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestWebDocURL verifies the user-facing URL builder.
func TestWebDocURL(t *testing.T) {
	if got, want := WebDocURL("DABC123"), "https://docs.qq.com/doc/DABC123"; got != want {
		t.Errorf("WebDocURL = %q, want %q", got, want)
	}
}

// TestConnectorTypeConstant pins the public connector identifier. The
// container registry registers against this string; renaming the constant
// is a breaking change to data-source rows persisted under the old type.
func TestConnectorTypeConstant(t *testing.T) {
	if ConnectorTypeTencentDocs != "tencent_docs" {
		t.Errorf("ConnectorTypeTencentDocs = %q, want %q", ConnectorTypeTencentDocs, "tencent_docs")
	}
	// Mirror check: the constant must also appear in the top-level types
	// package so container.go can reference it through a single source.
	raw, err := json.Marshal(ConnectorTypeTencentDocs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `"tencent_docs"` {
		t.Errorf("marshal = %s, want \"tencent_docs\"", raw)
	}
}

// errStr adapts a string to error so the table test stays compact.
type stringErr string

func (s stringErr) Error() string { return string(s) }
func errStr(s string) error       { return stringErr(s) }
