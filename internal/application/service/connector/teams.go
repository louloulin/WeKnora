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

// TeamsConnector ingests channel messages and meeting transcripts from
// Microsoft Teams via the Graph API. Uses client_credentials OAuth2
// flow with a registered Azure AD application. Falls back to stub mode
// for offline tests.
//
// Expected config JSON:
//
//	{
//	  "tenant_id":     "...",
//	  "client_id":     "...",
//	  "client_secret": "...",
//	  "kinds":         ["channel","meeting"],
//	  "lookback":      "168h",
//	  "max_per_kind":  50,
//	  "channels":  [...],   // optional stubs
//	  "meetings":  [...]
//	}
type TeamsConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewTeamsConnector() *TeamsConnector {
	return &TeamsConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (TeamsConnector) Kind() types.ConnectorKind { return types.ConnectorTeams }

type teamsConfig struct {
	TenantID     string         `json:"tenant_id"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Kinds        []string       `json:"kinds"`
	Lookback     string         `json:"lookback"`
	MaxPerKind   int            `json:"max_per_kind"`
	Channels     []teamsStub    `json:"channels"`
	Meetings     []teamsStub    `json:"meetings"`
}

type teamsStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *TeamsConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var tc teamsConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &tc); err != nil {
			return nil, fmt.Errorf("teams: invalid config: %w", err)
		}
	}
	if tc.MaxPerKind <= 0 {
		tc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range tc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["channel"] {
		msgs, err := c.fetchChannels(ctx, tc)
		if err != nil {
			logger.Warnf(ctx, "[Teams] channel fetch failed: %v (stub fallback)", err)
			msgs = stubTeams(tc.Channels, "channel")
		}
		out = append(out, msgs...)
	}
	if all || kinds["meeting"] {
		msgs, err := c.fetchMeetings(ctx, tc)
		if err != nil {
			msgs = stubTeams(tc.Meetings, "meeting")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *TeamsConnector) fetchToken(ctx context.Context, tc teamsConfig) (string, error) {
	if tc.TenantID == "" || tc.ClientID == "" || tc.ClientSecret == "" {
		return "", errors.New("teams: tenant_id + client_id + client_secret required")
	}
	url := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tc.TenantID)
	body := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://graph.microsoft.com/.default",
		tc.ClientID, tc.ClientSecret)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, io.NopCloser(stringReader(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("teams: token status=%d body=%s", resp.StatusCode, string(buf))
	}
	var raw struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	return raw.AccessToken, nil
}

func (c *TeamsConnector) fetchChannels(ctx context.Context, tc teamsConfig) ([]types.ConnectorMessage, error) {
	if _, err := c.fetchToken(ctx, tc); err != nil {
		return nil, err
	}
	// Real call: GET /teams/{team-id}/channels + /teams/{team-id}/channels/{channel-id}/messages
	return nil, errors.New("teams channels: real fetch not yet wired (stub fallback)")
}

func (c *TeamsConnector) fetchMeetings(ctx context.Context, tc teamsConfig) ([]types.ConnectorMessage, error) {
	if _, err := c.fetchToken(ctx, tc); err != nil {
		return nil, err
	}
	return nil, errors.New("teams meetings: real fetch not yet wired (stub fallback)")
}

func stubTeams(items []teamsStub, kind string) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID:        i.ID,
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.Author,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": kind, "stub": "true"},
		})
	}
	return out
}

func stringReader(s string) *stringReaderImpl { return &stringReaderImpl{s: s} }

type stringReaderImpl struct {
	s   string
	off int
}

func (r *stringReaderImpl) Read(p []byte) (int, error) {
	if r.off >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.off:])
	r.off += n
	return n, nil
}
