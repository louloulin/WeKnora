// Package core holds the shared Tencent Docs (腾讯文档) Open API client,
// region/config parsing, error classification and the streaming fetch
// helpers reused by the per-type connectors (doc, sheet, slide, form).
//
// Design follows the Feishu connector (internal/datasource/connector/feishu/core):
//   - One Client per region with a cached OAuth access token.
//   - Per-type connectors (doc/...) implement NodeOps[N] and delegate the
//     actual sync loop to feishu/core's FetchStreamEngine / FetchAllEngine /
//     FetchIncrementalEngine. We intentionally REUSE those engines rather than
//     fork them: the streaming + checkpoint + resume semantics are identical and
//     the Feishu implementation is already covered by golden/stream/convergence
//     tests. Keeping the engine single-sourced means new connectors only ship
//     the NodeOps adapter and any type-specific list/fetch helpers.
//   - Region is a small struct (host/tenant) so international / mainland
//     clouds and the Drive-style mode register as separate ConnectorType
//     values that share the same client code.
package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Tencent Docs Open API origins. The Open Platform hosts a single API surface
// for docs.qq.com (mainland) and docs.qq.com/intl (international); credentials
// are scoped to one cloud and never valid on the other.
const (
	tencentDocsOpenBaseURL = "https://docs.qq.com/openapi"
)

// Web origins used to build user-facing links to documents and drives.
const (
	tencentDocsWebBaseURL = "https://docs.qq.com"
)

// ConnectorTypeTencentDocs is the public connector identifier registered in
// internal/container/container.go initConnectorRegistry. Lives here (not in
// internal/types/datasource.go) so the Tencent Docs package owns its own
// constant - mirrors how feishu/core owns Region* while types/ keeps the
// canonical table.
const (
	ConnectorTypeTencentDocs = "tencent_docs"
)

// Config is the validated Tencent Docs connector configuration. It mirrors the
// Feishu Config shape so the existing datasource_service can render forms and
// store credentials without per-connector branching.
type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// AccessToken is the OAuth2 access token. May be supplied directly (for
	// service-account / long-lived integrations) when client_credentials is not
	// available. When set, refresh tokens are skipped.
	AccessToken string `json:"access_token,omitempty"`
	// BaseURL overrides tencentDocsOpenBaseURL (mainly for tests / private
	// deployments).
	BaseURL string `json:"base_url,omitempty"`
	// Timezone is the IANA name (e.g. "Asia/Shanghai") used when rendering
	// timestamps in fetched content; defaults to Asia/Shanghai.
	Timezone string `json:"timezone,omitempty"`
	// ResourceIDs are the document / drive IDs the user picked in the picker.
	ResourceIDs []string `json:"resource_ids,omitempty"`
}

// GetBaseURL returns the configured BaseURL or the package default.
func (c *Config) GetBaseURL() string {
	if c != nil && c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return tencentDocsOpenBaseURL
}

// TokenResponse is the OAuth2 token endpoint reply. Mirrors the public Tencent
// Docs Open Platform contract.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope,omitempty"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
}

// APIResponse is the standard envelope used by Tencent Docs list endpoints.
type APIResponse struct {
	Data    json.RawMessage `json:"data"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
	HasMore bool            `json:"has_more,omitempty"`
	Next    string          `json:"next,omitempty"`
}

// Document describes one Tencent Docs document surfaced in ListResources and
// consumed by the doc/ NodeOps adapter.
type Document struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // doc / sheet / slide / form
	Title      string    `json:"title"`
	URL        string    `json:"url,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedAt  time.Time `json:"created_at"`
	Owner      string    `json:"owner,omitempty"`
	ParentID   string    `json:"parent_id,omitempty"`
	SizeBytes  int64     `json:"size,omitempty"`
	Permission string    `json:"permission,omitempty"`
}

// EditTime returns the change-detection timestamp string used by the cursor.
// Empty when UpdatedAt is the zero value so the engine treats a fresh node as
// "changed" on first sync.
func (d Document) EditTime() string {
	if d.UpdatedAt.IsZero() {
		return ""
	}
	return d.UpdatedAt.UTC().Format(time.RFC3339)
}

// ErrCodeMessage returns a stable "<code>: <msg>" string for logs and error
// metadata. Many Tencent Docs endpoints return errcode/errmsg with no numeric
// zero sentinel, so we normalise the presentation here.
func (r APIResponse) ErrCodeMessage() string {
	if r.ErrCode == 0 && r.ErrMsg == "" {
		return ""
	}
	return fmt.Sprintf("errcode=%d errmsg=%s", r.ErrCode, r.ErrMsg)
}

// ParseConfig extracts and validates the Tencent Docs configuration from a
// generic DataSourceConfig. Kept as a package-level function so per-type
// connectors (doc/...) can call it without depending on each other.
//
// Mirrors feishu/core.ParseFeishuConfig: credentials come from
// DataSourceConfig.Credentials (the secret-bearing subresource), non-secret
// settings such as timezone come from DataSourceConfig.Settings.
func ParseConfig(config *types.DataSourceConfig) (*Config, error) {
	if config == nil {
		return nil, fmt.Errorf("tencent_docs: missing config")
	}

	credBytes, err := json.Marshal(config.Credentials)
	if err != nil {
		return nil, fmt.Errorf("tencent_docs: marshal credentials: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(credBytes, &cfg); err != nil {
		return nil, fmt.Errorf("tencent_docs: parse credentials: %w", err)
	}

	if cfg.ClientID == "" && cfg.AccessToken == "" {
		return nil, fmt.Errorf("tencent_docs: client_id or access_token is required")
	}
	if cfg.AccessToken == "" && cfg.ClientSecret == "" {
		return nil, fmt.Errorf("tencent_docs: client_secret is required when client_id is set")
	}

	if config.Settings != nil {
		if tz, ok := config.Settings["timezone"].(string); ok {
			cfg.Timezone = strings.TrimSpace(tz)
		}
		if baseURL, ok := config.Settings["base_url"].(string); ok {
			if cfg.BaseURL == "" {
				cfg.BaseURL = strings.TrimSpace(baseURL)
			}
		}
	}

	return &cfg, nil
}
