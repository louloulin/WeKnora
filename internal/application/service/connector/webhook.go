package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WebhookConnector lets an external system push items into KB by
// POSTing to a URL that the v0.7.24 admin UI exposes. The stub
// version reads pre-staged items from the connector config — the
// real implementation would accept POST requests directly via a
// per-connector signed URL.
//
// Expected config JSON:
//   {
//     "token": "<shared-secret>",
//     "items": [
//       {"id":"evt-1","title":"Deploy v1.2","content":"...","author":"ci-bot"}
//     ]
//   }
type WebhookConnector struct{}

// Kind implements interfaces.Connector.
func (WebhookConnector) Kind() types.ConnectorKind { return types.ConnectorWebhook }

type webhookItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Author  string `json:"author"`
	URL     string `json:"url"`
}

type webhookConfig struct {
	Token string        `json:"token"`
	Items []webhookItem `json:"items"`
}

// Fetch implements interfaces.Connector.
func (WebhookConnector) Fetch(_ context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if strings.TrimSpace(cfg.ConfigJSON) == "" {
		return nil, errors.New("webhook: empty config")
	}
	var wc webhookConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &wc); err != nil {
		return nil, errors.New("webhook: parse config: " + err.Error())
	}
	out := make([]types.ConnectorMessage, 0, len(wc.Items))
	for _, it := range wc.Items {
		out = append(out, types.ConnectorMessage{
			ID:        it.ID,
			Title:     it.Title,
			Content:   it.Content,
			Author:    it.Author,
			URL:       it.URL,
			Timestamp: time.Now(),
			Metadata:  map[string]string{"source": "webhook"},
		})
	}
	return out, nil
}
