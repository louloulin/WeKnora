package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SlackConnector is the v0.7.24 stub for the Slack connector. It
// returns messages parsed from the connector's config JSON. In
// production this would call conversations.history against the
// Slack Web API; here we keep the surface identical so swapping in
// the real client is a single function change.
//
// Expected config JSON:
//   {
//     "channel": "C01234567",
//     "messages": [
//       {"ts":"1700000001.000100","text":"hello","user":"U123","permalink":"https://..."}
//     ]
//   }
type SlackConnector struct{}

// Kind implements interfaces.Connector.
func (SlackConnector) Kind() types.ConnectorKind { return types.ConnectorSlack }

type slackMessage struct {
	TS        string `json:"ts"`
	Text      string `json:"text"`
	User      string `json:"user"`
	Permalink string `json:"permalink"`
}

type slackConfig struct {
	Channel  string         `json:"channel"`
	Messages []slackMessage `json:"messages"`
}

// Fetch implements interfaces.Connector.
func (SlackConnector) Fetch(_ context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if strings.TrimSpace(cfg.ConfigJSON) == "" {
		return nil, errors.New("slack: empty config")
	}
	var sc slackConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &sc); err != nil {
		return nil, fmt.Errorf("slack: parse config: %w", err)
	}
	if sc.Channel == "" {
		return nil, errors.New("slack: channel required")
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
