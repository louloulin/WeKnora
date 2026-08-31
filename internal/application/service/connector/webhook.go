package connector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WebhookConnector is the v0.7.25 Build #25 (G05) webhook ingestion
// connector. Two operating modes are supported:
//
//  1. Stub mode (v0.7.24): reads items from the connector config
//     JSON {"items":[...]}. Useful for offline testing and seeding.
//
//  2. Real mode (v0.7.25): the connector points at an HTTP queue
//     that buffers signed payloads. The HTTP server returns a JSON
//     array of {id, title, content, author, url, timestamp} rows
//     whose body was HMAC-SHA256-signed with the shared secret at
//     ingest time. The same secret verifies the response.
//
// Expected config JSON:
//
//	{
//	  "secret": "<shared-secret>",
//	  "queue_url": "https://hook-relay.internal/queue/<id>", // optional
//	  "queue_token": "<bearer>",                            // optional
//	  "max_items": 100,                                     // optional
//	  "items": [...]                                        // stub fallback
//	}
type WebhookConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewWebhookConnector() *WebhookConnector {
	return &WebhookConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (WebhookConnector) Kind() types.ConnectorKind { return types.ConnectorWebhook }

type webhookItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
}

type webhookConfig struct {
	Secret    string        `json:"secret"`
	QueueURL  string        `json:"queue_url"`
	QueueToken string       `json:"queue_token"`
	MaxItems  int           `json:"max_items"`
	Items     []webhookItem `json:"items"`
}

// webhookQueueResponse is the body shape returned by the upstream
// queue server. The HMAC signature is delivered via the
// X-WeKnora-Signature header (hex digest, optional sha256= prefix)
// so it can be verified against the body bytes without including
// the signature inside the bytes themselves.
type webhookQueueResponse struct {
	Items []webhookItem `json:"items"`
}

// Fetch implements interfaces.Connector. Falls back to stub mode if
// no queue_url is configured (preserves v0.7.24 behavior).
func (w *WebhookConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if w.HTTPClient == nil {
		w.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if w.Now == nil {
		w.Now = time.Now
	}

	if strings.TrimSpace(cfg.ConfigJSON) == "" {
		return nil, errors.New("webhook: empty config")
	}
	var wc webhookConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &wc); err != nil {
		return nil, fmt.Errorf("webhook: parse config: %w", err)
	}

	maxItems := wc.MaxItems
	if maxItems <= 0 {
		maxItems = 100
	}
	if maxItems > 1000 {
		maxItems = 1000
	}

	// Stub mode
	if wc.QueueURL == "" {
		return w.fetchStub(wc.Items, maxItems), nil
	}

	// Real mode: fetch signed queue.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wc.QueueURL, nil)
	if err != nil {
		return nil, fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WeKnora/0.7.25")
	if wc.QueueToken != "" {
		req.Header.Set("Authorization", "Bearer "+wc.QueueToken)
	}

	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("webhook: queue returned status %d", resp.StatusCode)
	}

	// Read the raw body so we can both verify the HMAC and parse the
	// payload. The signature is computed over the exact response bytes
	// the server signed — re-encoding the items array client-side
	// would risk byte-order drift.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("webhook: read body: %w", err)
	}

	var qr webhookQueueResponse
	if err := json.Unmarshal(rawBody, &qr); err != nil {
		return nil, fmt.Errorf("webhook: parse response: %w", err)
	}

	// Verify HMAC-SHA256 signature over the response body if the
	// secret is configured. Signature comes via the
	// X-WeKnora-Signature header (hex digest, optional sha256= prefix).
	if wc.Secret != "" {
		sig := resp.Header.Get("X-WeKnora-Signature")
		if sig == "" {
			return nil, errors.New("webhook: missing X-WeKnora-Signature header")
		}
		sig = strings.TrimPrefix(sig, "sha256=")
		mac := hmac.New(sha256.New, []byte(wc.Secret))
		mac.Write(rawBody)
		want := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(want))) {
			return nil, errors.New("webhook: signature mismatch")
		}
	}

	return w.itemsToMessages(qr.Items, maxItems), nil
}

func (w *WebhookConnector) fetchStub(items []webhookItem, max int) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, it := range items {
		out = append(out, types.ConnectorMessage{
			ID:        it.ID,
			Title:     it.Title,
			Content:   it.Content,
			Author:    it.Author,
			URL:       it.URL,
			Timestamp: it.Timestamp,
			Metadata:  map[string]string{"source": "webhook"},
		})
		if len(out) >= max {
			break
		}
	}
	return out
}

func (w *WebhookConnector) itemsToMessages(items []webhookItem, max int) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, it := range items {
		ts := it.Timestamp
		if ts.IsZero() {
			ts = w.Now()
		}
		out = append(out, types.ConnectorMessage{
			ID:        it.ID,
			Title:     it.Title,
			Content:   it.Content,
			Author:    it.Author,
			URL:       it.URL,
			Timestamp: ts,
			Metadata:  map[string]string{"source": "webhook"},
		})
		if len(out) >= max {
			break
		}
	}
	return out
}
