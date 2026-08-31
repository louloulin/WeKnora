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

// GoogleWorkspaceConnector is the v0.7.25 Build #30 (G10) Google
// Workspace connector. It speaks to the public Google APIs
// (gmail.googleapis.com, calendar.googleapis.com, drive.googleapis.com)
// using an OAuth2 access token carried on the runtime config. Pulls
// Gmail messages, Calendar events, or Drive files depending on the
// `resource_kind` config field.
//
// Expected config JSON:
//
//	{
//	  "resource_kind":  "google_email" | "google_meeting" | "google_drive",
//	  "user_id":        "alice@example.com" or "primary" (calendar),
//	  "query":          "label:important",
//	  "max_results":    50,
//	  "modified_after": "2026-08-01T00:00:00Z"
//	}
//
// The bearer token is read from the ConfigJSON blob itself when
// possible, with an env-var fallback so cron-style ingestion can
// rotate credentials without touching the config table.
//
// The connector is read-only; it never sends mail or modifies
// events. OAuth2 token refresh is the caller's job — the connector
// returns ErrGoogleAuthExpired on 401/403 so the caller can refresh.
type GoogleWorkspaceConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
	GmailBase  string
	CalBase    string
	DriveBase  string
}

// ErrGoogleAuthExpired is returned when Google APIs respond 401 / 403.
var ErrGoogleAuthExpired = errors.New("google: access token expired or missing")

// NewGoogleWorkspaceConnector returns a connector with sensible defaults.
func NewGoogleWorkspaceConnector() *GoogleWorkspaceConnector {
	return &GoogleWorkspaceConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
		GmailBase:  "https://gmail.googleapis.com/gmail/v1",
		CalBase:    "https://www.googleapis.com/calendar/v3",
		DriveBase:  "https://www.googleapis.com/drive/v3",
	}
}

// Kind implements interfaces.Connector.
func (g *GoogleWorkspaceConnector) Kind() types.ConnectorKind { return types.ConnectorGoogle }

// googleConfig mirrors the inner struct shape used everywhere
// throughout this file so we can take it by value without repeating
// the anonymous struct literal three times.
type googleConfig struct {
	ResourceKind  string `json:"resource_kind"`
	UserID        string `json:"user_id"`
	Query         string `json:"query"`
	MaxResults    int    `json:"max_results"`
	ModifiedAfter string `json:"modified_after"`
}

// Fetch implements interfaces.Connector.
func (g *GoogleWorkspaceConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var cfgBody googleConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &cfgBody); err != nil {
		return nil, fmt.Errorf("google: invalid config: %w", err)
	}
	token := cfg.ConfigJSON
	if token == "" {
		// Fall back to the env var so operators can inject a token
		// for cron-style ingestion without touching the config table.
		token = os.Getenv("WEKNORA_GOOGLE_TOKEN")
	}
	if token == "" {
		return nil, ErrGoogleAuthExpired
	}
	switch cfgBody.ResourceKind {
	case "google_email":
		return g.fetchGmail(ctx, token, cfgBody)
	case "google_meeting":
		return g.fetchCalendar(ctx, token, cfgBody)
	case "google_drive":
		return g.fetchDrive(ctx, token, cfgBody)
	default:
		return nil, fmt.Errorf("google: unsupported resource_kind %q", cfgBody.ResourceKind)
	}
}

func (g *GoogleWorkspaceConnector) fetchGmail(ctx context.Context, token string, body googleConfig) ([]types.ConnectorMessage, error) {
	if body.UserID == "" {
		return nil, errors.New("google: user_id required for google_email")
	}
	maxResults := body.MaxResults
	if maxResults <= 0 || maxResults > 500 {
		maxResults = 50
	}
	q := body.Query
	if body.ModifiedAfter != "" {
		if q != "" {
			q += " "
		}
		q += "after:" + body.ModifiedAfter
	}
	endpoint := fmt.Sprintf("%s/users/%s/messages?maxResults=%d",
		g.GmailBase, url.PathEscape(body.UserID), maxResults)
	if q != "" {
		endpoint += "&q=" + url.QueryEscape(q)
	}
	payload := map[string]any{}
	if err := g.apiGet(ctx, token, endpoint, &payload); err != nil {
		return nil, err
	}
	messages, _ := payload["messages"].([]any)
	out := make([]types.ConnectorMessage, 0, len(messages))
	for _, v := range messages {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		if id == "" {
			continue
		}
		// Fetch one message to get the headers.
		msgPayload := map[string]any{}
		if err := g.apiGet(ctx, token,
			fmt.Sprintf("%s/users/%s/messages/%s?format=metadata&metadataHeaders=Subject&metadataHeaders=From&metadataHeaders=Date",
				g.GmailBase, url.PathEscape(body.UserID), url.PathEscape(id)),
			&msgPayload); err != nil {
			logger.Warnf(ctx, "google: gmail msg %s: %v", id, err)
			continue
		}
		subject := extractGmailHeader(msgPayload, "Subject")
		from := extractGmailHeader(msgPayload, "From")
		dateStr := extractGmailHeader(msgPayload, "Date")
		internalDate, _ := msgPayload["internalDate"].(string)
		ts := parseGoogleDate(internalDate)
		if ts.IsZero() {
			ts = parseGoogleDate(dateStr)
		}
		snippet, _ := msgPayload["snippet"].(string)
		out = append(out, types.ConnectorMessage{
			ID:        "google:email:" + id,
			Title:     subject,
			Content:   snippet,
			Author:    from,
			URL:       "https://mail.google.com/mail/u/" + url.PathEscape(body.UserID) + "/#inbox/" + id,
			Timestamp: ts,
			Metadata: map[string]string{
				"source":   "google_email",
				"gmail_id": id,
				"user_id":  body.UserID,
			},
		})
	}
	return out, nil
}

func (g *GoogleWorkspaceConnector) fetchCalendar(ctx context.Context, token string, body googleConfig) ([]types.ConnectorMessage, error) {
	maxResults := body.MaxResults
	if maxResults <= 0 || maxResults > 500 {
		maxResults = 50
	}
	endpoint := fmt.Sprintf("%s/calendars/%s/events?maxResults=%d&singleEvents=true&orderBy=startTime",
		g.CalBase, url.PathEscape(body.UserID), maxResults)
	if body.ModifiedAfter != "" {
		endpoint += "&timeMin=" + url.QueryEscape(body.ModifiedAfter)
	}
	if body.Query != "" {
		endpoint += "&q=" + url.QueryEscape(body.Query)
	}
	payload := map[string]any{}
	if err := g.apiGet(ctx, token, endpoint, &payload); err != nil {
		return nil, err
	}
	items, _ := payload["items"].([]any)
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, v := range items {
		ev, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := ev["id"].(string)
		if id == "" {
			continue
		}
		summary, _ := ev["summary"].(string)
		description, _ := ev["description"].(string)
		startMap, _ := ev["start"].(map[string]any)
		startStr, _ := startMap["dateTime"].(string)
		organizer := extractGoogleOrganizer(ev)
		htmlLink, _ := ev["htmlLink"].(string)
		out = append(out, types.ConnectorMessage{
			ID:        "google:meeting:" + id,
			Title:     summary,
			Content:   description,
			Author:    organizer,
			URL:       htmlLink,
			Timestamp: parseGoogleDate(startStr),
			Metadata: map[string]string{
				"source":      "google_meeting",
				"event_id":    id,
				"calendar_id": body.UserID,
			},
		})
	}
	return out, nil
}

func (g *GoogleWorkspaceConnector) fetchDrive(ctx context.Context, token string, body googleConfig) ([]types.ConnectorMessage, error) {
	maxResults := body.MaxResults
	if maxResults <= 0 || maxResults > 500 {
		maxResults = 50
	}
	q := body.Query
	if q == "" {
		q = "trashed = false"
	}
	if body.ModifiedAfter != "" {
		q += " and modifiedTime > '" + body.ModifiedAfter + "'"
	}
	endpoint := fmt.Sprintf("%s/files?pageSize=%d&q=%s",
		g.DriveBase, maxResults, url.QueryEscape(q))
	payload := map[string]any{}
	if err := g.apiGet(ctx, token, endpoint, &payload); err != nil {
		return nil, err
	}
	files, _ := payload["files"].([]any)
	out := make([]types.ConnectorMessage, 0, len(files))
	for _, v := range files {
		f, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := f["id"].(string)
		if id == "" {
			continue
		}
		name, _ := f["name"].(string)
		modifiedStr, _ := f["modifiedTime"].(string)
		mime, _ := f["mimeType"].(string)
		webViewLink, _ := f["webViewLink"].(string)
		out = append(out, types.ConnectorMessage{
			ID:        "google:drive:" + id,
			Title:     name,
			Content:   "Drive file (mime: " + mime + ")",
			URL:       webViewLink,
			Timestamp: parseGoogleDate(modifiedStr),
			Metadata: map[string]string{
				"source":    "google_drive",
				"file_id":   id,
				"mime_type": mime,
			},
		})
	}
	return out, nil
}

func (g *GoogleWorkspaceConnector) apiGet(ctx context.Context, token, endpoint string, out any) error {
	if g.HTTPClient == nil {
		g.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("google: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrGoogleAuthExpired
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("google: api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("google: parse %s: %w", endpoint, err)
		}
	}
	return nil
}

func extractGmailHeader(m map[string]any, name string) string {
	payload, _ := m["payload"].(map[string]any)
	if payload == nil {
		return ""
	}
	headers, _ := payload["headers"].([]any)
	for _, h := range headers {
		entry, ok := h.(map[string]any)
		if !ok {
			continue
		}
		n, _ := entry["name"].(string)
		if strings.EqualFold(n, name) {
			v, _ := entry["value"].(string)
			return v
		}
	}
	return ""
}

func extractGoogleOrganizer(ev map[string]any) string {
	org, ok := ev["organizer"].(map[string]any)
	if !ok {
		return ""
	}
	if email, ok := org["email"].(string); ok {
		return email
	}
	if d, ok := org["displayName"].(string); ok {
		return d
	}
	return ""
}

func parseGoogleDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Gmail Date header is RFC1123 (alpha zone such as "GMT").
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t
	}
	return time.Time{}
}
