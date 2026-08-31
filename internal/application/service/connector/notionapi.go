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

// NotionAPIConnector ingests Pages and Databases from Notion via the
// 2025-09-03 REST API. Uses an internal integration token. Falls
// back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "integration_token": "secret_...",
//	  "kinds":             ["page","database"],
//	  "lookback":          "168h",
//	  "max_per_kind":      50,
//	  "pages":     [...],   // optional stubs
//	  "databases": [...]
//	}
type NotionAPIConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewNotionAPIConnector() *NotionAPIConnector {
	return &NotionAPIConnector{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (NotionAPIConnector) Kind() types.ConnectorKind { return types.ConnectorNotionAPI }

type notionAPIConfig struct {
	IntegrationToken string         `json:"integration_token"`
	Kinds            []string       `json:"kinds"`
	Lookback         string         `json:"lookback"`
	MaxPerKind       int            `json:"max_per_kind"`
	Pages            []notionAPIStub `json:"pages"`
	Databases        []notionAPIStub `json:"databases"`
}

type notionAPIStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *NotionAPIConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var nc notionAPIConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &nc); err != nil {
			return nil, fmt.Errorf("notion_api: invalid config: %w", err)
		}
	}
	if nc.IntegrationToken == "" {
		return nil, errors.New("notion_api: integration_token required")
	}
	if nc.MaxPerKind <= 0 {
		nc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range nc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["page"] {
		msgs, err := c.fetchPages(ctx, nc)
		if err != nil {
			logger.Warnf(ctx, "[NotionAPI] pages fetch failed: %v (stub fallback)", err)
			msgs = stubNotionAPI(nc.Pages, "page")
		}
		out = append(out, msgs...)
	}
	if all || kinds["database"] {
		msgs, err := c.fetchDatabases(ctx, nc)
		if err != nil {
			msgs = stubNotionAPI(nc.Databases, "database")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *NotionAPIConnector) fetchPages(ctx context.Context, nc notionAPIConfig) ([]types.ConnectorMessage, error) {
	url := "https://api.notion.com/v1/search?filter={\"property\":\"object\",\"value\":\"page\"}&page_size=" + fmt.Sprintf("%d", nc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	req.Header.Set("Authorization", "Bearer "+nc.IntegrationToken)
	req.Header.Set("Notion-Version", "2025-09-03")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("notion_api: pages status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Results []struct {
			ID        string    `json:"id"`
			URL       string    `json:"url"`
			LastEdited time.Time `json:"last_edited_time"`
			Properties map[string]struct {
				Title []struct {
					PlainText string `json:"plain_text"`
				} `json:"title"`
			} `json:"properties"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Results {
		title := ""
		for _, ps := range r.Properties {
			for _, t := range ps.Title {
				title += t.PlainText
			}
			break
		}
		out = append(out, types.ConnectorMessage{
			ID: r.ID, Title: title, Content: "",
			URL: r.URL, Timestamp: r.LastEdited,
			Metadata: map[string]string{"kind": "page"},
		})
	}
	return out, nil
}

func (c *NotionAPIConnector) fetchDatabases(ctx context.Context, nc notionAPIConfig) ([]types.ConnectorMessage, error) {
	url := "https://api.notion.com/v1/search?filter={\"property\":\"object\",\"value\":\"database\"}&page_size=" + fmt.Sprintf("%d", nc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	req.Header.Set("Authorization", "Bearer "+nc.IntegrationToken)
	req.Header.Set("Notion-Version", "2025-09-03")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion_api: databases status=%d", resp.StatusCode)
	}
	var raw struct {
		Results []struct {
			ID         string    `json:"id"`
			URL        string    `json:"url"`
			LastEdited time.Time `json:"last_edited_time"`
			Title      []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw.Results {
		title := ""
		for _, t := range r.Title {
			title += t.PlainText
		}
		out = append(out, types.ConnectorMessage{
			ID: r.ID, Title: title, Content: "",
			URL: r.URL, Timestamp: r.LastEdited,
			Metadata: map[string]string{"kind": "database"},
		})
	}
	return out, nil
}

func stubNotionAPI(items []notionAPIStub, kind string) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(items))
	for _, i := range items {
		out = append(out, types.ConnectorMessage{
			ID: i.ID, Title: i.Title, Content: i.Body,
			Author: i.Author, URL: i.URL, Timestamp: i.UpdatedAt,
			Metadata: map[string]string{"kind": kind, "stub": "true"},
		})
	}
	return out
}
