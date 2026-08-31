package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// M365Connector is the v0.7.25 Build #30 (G10) Microsoft 365 /
// Graph connector. It speaks to Microsoft Graph
// (graph.microsoft.com) using an OAuth2 access token carried in the
// runtime config and pulls mail, calendar events, or Teams chat
// messages depending on the `resource_kind` config field.
//
// Expected config JSON:
//
//	{
//	  "access_token":   "<OAuth2 bearer>",
//	  "resource_kind":  "m365_email" | "m365_meeting" | "teams_chat",
//	  "user_principal": "alice@contoso.com",
//	  "top":            50,
//	  "filter":         "receivedDateTime ge 2026-08-01T00:00:00Z"
//	}
//
// The connector is read-only. We pull the access token from inside
// the config JSON rather than threading a separate field through the
// ConnectorRuntimeConfig so legacy v0.7.24 callers can keep using
// the connector with a fresh credential set per run.
type M365Connector struct {
	HTTPClient *http.Client
	Now        func() time.Time
	GraphBase  string
}

// ErrM365AuthExpired is returned when Graph responds 401 / 403.
var ErrM365AuthExpired = errors.New("m365: access token expired or missing")

// NewM365Connector returns an M365Connector with sensible defaults.
func NewM365Connector() *M365Connector {
	return &M365Connector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
		GraphBase:  "https://graph.microsoft.com/v1.0",
	}
}

// Kind implements interfaces.Connector.
func (m *M365Connector) Kind() types.ConnectorKind { return types.ConnectorM365 }

// Fetch implements interfaces.Connector.
func (m *M365Connector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var cfgBody struct {
		AccessToken   string `json:"access_token"`
		ResourceKind  string `json:"resource_kind"`
		UserPrincipal string `json:"user_principal"`
		Top           int    `json:"top"`
		Filter        string `json:"filter"`
		TenantID      string `json:"tenant_id"`
	}
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &cfgBody); err != nil {
		return nil, fmt.Errorf("m365: invalid config: %w", err)
	}
	if cfgBody.AccessToken == "" {
		// Fall back to the env var so operators can inject a token
		// for cron-style ingestion without touching the config table.
		cfgBody.AccessToken = os.Getenv("WEKNORA_M365_TOKEN")
	}
	if cfgBody.AccessToken == "" {
		return nil, ErrM365AuthExpired
	}
	switch cfgBody.ResourceKind {
	case "m365_email":
		return m.fetchMail(cfgBody, ctx)
	case "m365_meeting":
		return m.fetchMeetings(cfgBody, ctx)
	case "teams_chat":
		return m.fetchTeams(cfgBody, ctx)
	default:
		return nil, fmt.Errorf("m365: unsupported resource_kind %q", cfgBody.ResourceKind)
	}
}

func (m *M365Connector) fetchMail(body struct {
	AccessToken   string `json:"access_token"`
	ResourceKind  string `json:"resource_kind"`
	UserPrincipal string `json:"user_principal"`
	Top           int    `json:"top"`
	Filter        string `json:"filter"`
	TenantID      string `json:"tenant_id"`
}, ctx context.Context) ([]types.ConnectorMessage, error) {
	if body.UserPrincipal == "" {
		return nil, errors.New("m365: user_principal required for m365_email")
	}
	top := body.Top
	if top <= 0 || top > 200 {
		top = 50
	}
	endpoint := fmt.Sprintf("%s/users/%s/messages?$top=%d",
		m.GraphBase, url.PathEscape(body.UserPrincipal), top)
	if body.Filter != "" {
		endpoint += "&$filter=" + url.QueryEscape(body.Filter)
	}
	payload := map[string]any{}
	if err := m.graphGet(ctx, body.AccessToken, endpoint, &payload); err != nil {
		return nil, err
	}
	values, _ := payload["value"].([]any)
	msgs := make([]types.ConnectorMessage, 0, len(values))
	for _, v := range values {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		subject, _ := row["subject"].(string)
		preview, _ := row["bodyPreview"].(string)
		author := extractM365Sender(row)
		dateStr, _ := row["receivedDateTime"].(string)
		msgs = append(msgs, types.ConnectorMessage{
			ID:        "m365:email:" + id,
			Title:     subject,
			Content:   preview,
			Author:    author,
			Timestamp: parseM365Date(dateStr),
			Metadata: map[string]string{
				"source":    "m365_email",
				"graph_id":  id,
				"tenant_id": body.TenantID,
			},
		})
	}
	return msgs, nil
}

func (m *M365Connector) fetchMeetings(body struct {
	AccessToken   string `json:"access_token"`
	ResourceKind  string `json:"resource_kind"`
	UserPrincipal string `json:"user_principal"`
	Top           int    `json:"top"`
	Filter        string `json:"filter"`
	TenantID      string `json:"tenant_id"`
}, ctx context.Context) ([]types.ConnectorMessage, error) {
	if body.UserPrincipal == "" {
		return nil, errors.New("m365: user_principal required for m365_meeting")
	}
	top := body.Top
	if top <= 0 || top > 200 {
		top = 50
	}
	endpoint := fmt.Sprintf("%s/users/%s/events?$top=%d",
		m.GraphBase, url.PathEscape(body.UserPrincipal), top)
	if body.Filter != "" {
		endpoint += "&$filter=" + url.QueryEscape(body.Filter)
	}
	payload := map[string]any{}
	if err := m.graphGet(ctx, body.AccessToken, endpoint, &payload); err != nil {
		return nil, err
	}
	values, _ := payload["value"].([]any)
	msgs := make([]types.ConnectorMessage, 0, len(values))
	for _, v := range values {
		ev, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := ev["id"].(string)
		if id == "" {
			continue
		}
		subject, _ := ev["subject"].(string)
		preview, _ := ev["bodyPreview"].(string)
		author := extractM365Sender(ev)
		var dateStr string
		if start, ok := ev["start"].(map[string]any); ok {
			dateStr, _ = start["dateTime"].(string)
		}
		msgs = append(msgs, types.ConnectorMessage{
			ID:        "m365:meeting:" + id,
			Title:     subject,
			Content:   preview,
			Author:    author,
			Timestamp: parseM365Date(dateStr),
			Metadata: map[string]string{
				"source":    "m365_meeting",
				"graph_id":  id,
				"tenant_id": body.TenantID,
			},
		})
	}
	return msgs, nil
}

func (m *M365Connector) fetchTeams(body struct {
	AccessToken   string `json:"access_token"`
	ResourceKind  string `json:"resource_kind"`
	UserPrincipal string `json:"user_principal"`
	Top           int    `json:"top"`
	Filter        string `json:"filter"`
	TenantID      string `json:"tenant_id"`
}, ctx context.Context) ([]types.ConnectorMessage, error) {
	if body.UserPrincipal == "" {
		return nil, errors.New("m365: user_principal required for teams_chat")
	}
	top := body.Top
	if top <= 0 || top > 200 {
		top = 50
	}
	chatsPayload := map[string]any{}
	if err := m.graphGet(ctx, body.AccessToken, fmt.Sprintf("%s/users/%s/chats?$top=10",
		m.GraphBase, url.PathEscape(body.UserPrincipal)), &chatsPayload); err != nil {
		return nil, err
	}
	chats, _ := chatsPayload["value"].([]any)
	msgs := make([]types.ConnectorMessage, 0)
	for _, c := range chats {
		chat, ok := c.(map[string]any)
		if !ok {
			continue
		}
		chatID, _ := chat["id"].(string)
		if chatID == "" {
			continue
		}
		endpoint := fmt.Sprintf("%s/chats/%s/messages?$top=%d", m.GraphBase, url.PathEscape(chatID), top)
		messagesPayload := map[string]any{}
		if err := m.graphGet(ctx, body.AccessToken, endpoint, &messagesPayload); err != nil {
			logger.Warnf(ctx, "m365: failed to read chat %s: %v", chatID, err)
			continue
		}
		messages, _ := messagesPayload["value"].([]any)
		for _, v := range messages {
			msg, ok := v.(map[string]any)
			if !ok {
				continue
			}
			id, _ := msg["id"].(string)
			if id == "" {
				continue
			}
			bodyMap, _ := msg["body"].(map[string]any)
			content, _ := bodyMap["content"].(string)
			author := extractM365Sender(msg)
			dateStr, _ := msg["createdDateTime"].(string)
			msgs = append(msgs, types.ConnectorMessage{
				ID:        "m365:teams:" + id,
				Title:     "Teams chat " + chatID,
				Content:   content,
				Author:    author,
				Timestamp: parseM365Date(dateStr),
				Metadata: map[string]string{
					"source":  "teams_chat",
					"chat_id": chatID,
					"graph_id": id,
				},
			})
		}
	}
	return msgs, nil
}

func (m *M365Connector) graphGet(ctx context.Context, token, endpoint string, out any) error {
	if m.HTTPClient == nil {
		m.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("m365: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrM365AuthExpired
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("m365: graph returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("m365: parse %s: %w", endpoint, err)
		}
	}
	return nil
}

func extractM365Sender(m map[string]any) string {
	if from, ok := m["from"].(map[string]any); ok {
		if email, ok := from["emailAddress"].(map[string]any); ok {
			name, _ := email["name"].(string)
			addr, _ := email["address"].(string)
			if name != "" && addr != "" {
				return name + " <" + addr + ">"
			}
			return addr
		}
	}
	if sender, ok := m["sender"].(map[string]any); ok {
		if email, ok := sender["emailAddress"].(map[string]any); ok {
			addr, _ := email["address"].(string)
			return addr
		}
	}
	return ""
}

func parseM365Date(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
