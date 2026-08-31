package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SlackConnector is the v0.7.25 Build #25 (G05) real implementation
// of the Slack ingestion connector. It calls the Slack Web API
// `conversations.history` to list recent messages in a channel
// (or all channels) using an OAuth bot token or xoxp/xoxb token.
// It also accepts the v0.7.24 stub config ({"channel":...,"messages":[...]})
// for offline testing — when no token is present it falls back to
// reading messages from the config so unit tests and dev sandboxes
// don't need network access.
//
// Expected config JSON:
//
//	{
//	  "bot_token": "xoxb-...",          // preferred: OAuth bot token
//	  "channel":   "C01234567",          // channel ID (preferred) or "#name"
//	  "channels":  ["C01234567"],        // optional: multi-channel mode
//	  "lookback":  "168h",               // optional: time window (Go duration)
//	  "max_per_channel": 50,             // optional: cap per channel (default 50)
//	  "messages": [...]                  // optional: stub fallback (v0.7.24)
//	}
type SlackConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewSlackConnector() *SlackConnector {
	return &SlackConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (SlackConnector) Kind() types.ConnectorKind { return types.ConnectorSlack }

type slackMessage struct {
	TS        string `json:"ts"`
	Text      string `json:"text"`
	User      string `json:"user"`
	Permalink string `json:"permalink"`
}

type slackConfig struct {
	BotToken       string         `json:"bot_token"`
	Channel        string         `json:"channel"`
	Channels       []string       `json:"channels"`
	Lookback       string         `json:"lookback"`
	MaxPerChannel  int            `json:"max_per_channel"`
	Messages       []slackMessage `json:"messages"` // v0.7.24 stub fallback
}

type slackAPIResponse struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
	Messages []struct {
		Type string `json:"type"`
		TS   string `json:"ts"`
		Text string `json:"text"`
		User string `json:"user"`
	} `json:"messages"`
	HasMore  bool   `json:"has_more"`
	Response struct {
		OK       bool   `json:"ok"`
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		Error string `json:"error"`
	} `json:"-"`
}

// Fetch implements interfaces.Connector. Falls back to stub mode when
// no bot_token is configured, so existing v0.7.24 manual configs keep
// working in tests.
func (s *SlackConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if s.HTTPClient == nil {
		s.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	if strings.TrimSpace(cfg.ConfigJSON) == "" {
		return nil, errors.New("slack: empty config")
	}
	var sc slackConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &sc); err != nil {
		return nil, fmt.Errorf("slack: parse config: %w", err)
	}

	// Stub mode — preserved for v0.7.24 tests and offline dev.
	if sc.BotToken == "" {
		return s.fetchStub(sc)
	}

	channels := sc.Channels
	if len(channels) == 0 && sc.Channel != "" {
		channels = []string{sc.Channel}
	}
	if len(channels) == 0 {
		return nil, errors.New("slack: bot_token set but channel(s) missing")
	}

	lookback := 7 * 24 * time.Hour
	if sc.Lookback != "" {
		if d, err := time.ParseDuration(sc.Lookback); err == nil && d > 0 {
			lookback = d
		}
	}
	oldest := float64(s.Now().Add(-lookback).Unix())

	maxPer := sc.MaxPerChannel
	if maxPer <= 0 {
		maxPer = 50
	}
	if maxPer > 200 {
		maxPer = 200
	}

	out := make([]types.ConnectorMessage, 0, len(channels)*maxPer)
	for _, ch := range channels {
		msgs, err := s.fetchChannel(ctx, sc.BotToken, ch, oldest, maxPer)
		if err != nil {
			return nil, err
		}
		out = append(out, msgs...)
	}

	logger.Infof(ctx, "slack: fetched %d messages across %d channel(s)", len(out), len(channels))
	return out, nil
}

func (s *SlackConnector) fetchChannel(ctx context.Context, token, channel string, oldest float64, limit int) ([]types.ConnectorMessage, error) {
	apiURL := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&oldest=%s&limit=%d",
		url.QueryEscape(channel), url.QueryEscape(strconvFloat(oldest)), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WeKnora/0.7.25")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("slack: API status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var ar slackAPIResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("slack: parse response: %w", err)
	}
	if !ar.OK {
		return nil, fmt.Errorf("slack: API error: %s", ar.Error)
	}

	out := make([]types.ConnectorMessage, 0, len(ar.Messages))
	for _, m := range ar.Messages {
		if m.Type != "" && m.Type != "message" {
			continue
		}
		ts, _ := parseSlackTS(m.TS)
		if ts.IsZero() {
			ts = s.Now()
		}
		out = append(out, types.ConnectorMessage{
			ID:        m.TS,
			Title:     fmt.Sprintf("Slack #%s — %s", channel, truncate(m.Text, 60)),
			Content:   m.Text,
			Author:    m.User,
			URL:       buildSlackPermalink(token, channel, m.TS),
			Timestamp: ts,
			Metadata: map[string]string{
				"channel": channel,
				"source":  "slack",
			},
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// fetchStub preserves v0.7.24 behavior for offline configs.
func (s *SlackConnector) fetchStub(sc slackConfig) ([]types.ConnectorMessage, error) {
	if sc.Channel == "" && len(sc.Messages) == 0 {
		return nil, errors.New("slack: channel or messages required")
	}
	out := make([]types.ConnectorMessage, 0, len(sc.Messages))
	for _, m := range sc.Messages {
		ts, err := parseSlackTS(m.TS)
		if err != nil {
			ts = time.Now()
		}
		out = append(out, types.ConnectorMessage{
			ID:        m.TS,
			Title:     fmt.Sprintf("Slack #%s — %s", sc.Channel, truncate(m.Text, 60)),
			Content:   m.Text,
			Author:    m.User,
			URL:       m.Permalink,
			Timestamp: ts,
			Metadata: map[string]string{
				"channel": sc.Channel,
				"source":  "slack",
			},
		})
	}
	return out, nil
}

// buildSlackPermalink constructs the canonical app link shape for a
// message. Slack's per-message permalink API requires a separate call
// (chat.getPermalink); this client-side shape is the user-friendly
// fallback that still resolves in the Slack web app.
func buildSlackPermalink(token, channel, ts string) string {
	_ = token
	return fmt.Sprintf("slack://channel?team=&id=%s&message=%s", url.QueryEscape(channel), url.QueryEscape(ts))
}

// strconvFloat formats a float64 as a string with no exponent, the
// shape Slack's `oldest` parameter expects.
func strconvFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(
		fmt.Sprintf("%.6f", f), "0"), ".")
}

// parseSlackTS converts "1700000001.000100" → time.Time.
func parseSlackTS(ts string) (time.Time, error) {
	parts := strings.SplitN(ts, ".", 2)
	if len(parts) == 0 {
		return time.Time{}, errors.New("invalid ts")
	}
	var sec int64
	if _, err := fmt.Sscanf(parts[0], "%d", &sec); err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}


