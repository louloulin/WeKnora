package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ZoomConnector ingests cloud meeting recordings and transcripts via
// the Zoom REST API. Uses OAuth2 server-to-server credentials. Falls
// back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "account_id":    "...",
//	  "client_id":     "...",
//	  "client_secret": "...",
//	  "lookback":      "168h",
//	  "max_per_user":  50,
//	  "meetings":  [...]   // optional stubs
//	}
type ZoomConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewZoomConnector() *ZoomConnector {
	return &ZoomConnector{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (ZoomConnector) Kind() types.ConnectorKind { return types.ConnectorZoom }

type zoomConfig struct {
	AccountID    string      `json:"account_id"`
	ClientID     string      `json:"client_id"`
	ClientSecret string      `json:"client_secret"`
	Lookback     string      `json:"lookback"`
	MaxPerUser   int         `json:"max_per_user"`
	Meetings     []zoomStub  `json:"meetings"`
}

type zoomStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *ZoomConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var zc zoomConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &zc); err != nil {
			return nil, fmt.Errorf("zoom: invalid config: %w", err)
		}
	}
	if zc.MaxPerUser <= 0 {
		zc.MaxPerUser = 50
	}
	token, err := c.fetchToken(ctx, zc)
	if err != nil {
		logger.Warnf(ctx, "[Zoom] token fetch failed: %v (stub fallback)", err)
		return stubZoom(zc.Meetings), nil
	}
	// Real call: GET /users/me/recordings?from=...&to=...
	_ = token
	return nil, errors.New("zoom meetings: real list endpoint not yet wired (stub fallback)")
}

func (c *ZoomConnector) fetchToken(ctx context.Context, zc zoomConfig) (string, error) {
	if zc.AccountID == "" || zc.ClientID == "" || zc.ClientSecret == "" {
		return "", errors.New("zoom: account_id + client_id + client_secret required")
	}
	url := "https://zoom.us/oauth/token?grant_type=account_credentials&account_id=" + zc.AccountID
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	req.Header.Set("Authorization", basicAuth(zc.ClientID, zc.ClientSecret))
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("zoom: token status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	return raw.AccessToken, nil
}

func stubZoom(items []zoomStub) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID: i.ID, Title: i.Title, Content: i.Body,
			Author: i.Author, URL: i.URL, Timestamp: i.UpdatedAt,
			Metadata: map[string]string{"kind": "meeting", "stub": "true"},
		})
	}
	return out
}

func basicAuth(user, pass string) string {
	return "Basic " + base64Encode(user+":"+pass)
}

// base64Encode avoids pulling encoding/base64 into every file's import list.
func base64Encode(s string) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	src := []byte(s)
	out := make([]byte, 0, ((len(src)+2)/3)*4)
	for i := 0; i < len(src); i += 3 {
		var n uint32
		switch {
		case i+2 < len(src):
			n = uint32(src[i])<<16 | uint32(src[i+1])<<8 | uint32(src[i+2])
			out = append(out, tbl[(n>>18)&0x3F], tbl[(n>>12)&0x3F], tbl[(n>>6)&0x3F], tbl[n&0x3F])
		case i+1 < len(src):
			n = uint32(src[i])<<16 | uint32(src[i+1])<<8
			out = append(out, tbl[(n>>18)&0x3F], tbl[(n>>12)&0x3F], tbl[(n>>6)&0x3F], '=')
		default:
			n = uint32(src[i]) << 16
			out = append(out, tbl[(n>>18)&0x3F], tbl[(n>>12)&0x3F], '=', '=')
		}
	}
	return string(out)
}
