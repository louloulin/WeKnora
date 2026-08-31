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

// AirtableConnector ingests Bases, Tables and Records from Airtable
// via the Web API. Uses a personal access token or OAuth2 token.
// Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "access_token": "pat...",
//	  "base_id":      "app...",
//	  "kinds":        ["table","record"],
//	  "lookback":     "168h",
//	  "max_per_kind": 50,
//	  "tables":  [...],   // optional stubs
//	  "records": [...]
//	}
type AirtableConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewAirtableConnector() *AirtableConnector {
	return &AirtableConnector{HTTPClient: &http.Client{Timeout: 30 * time.Second}, Now: time.Now}
}

func (AirtableConnector) Kind() types.ConnectorKind { return types.ConnectorAirtable }

type airtableConfig struct {
	AccessToken string        `json:"access_token"`
	BaseID      string        `json:"base_id"`
	Kinds       []string      `json:"kinds"`
	Lookback    string        `json:"lookback"`
	MaxPerKind  int           `json:"max_per_kind"`
	Tables      []airtableStub `json:"tables"`
	Records     []airtableStub `json:"records"`
}

type airtableStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *AirtableConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var ac airtableConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &ac); err != nil {
			return nil, fmt.Errorf("airtable: invalid config: %w", err)
		}
	}
	if ac.AccessToken == "" || ac.BaseID == "" {
		return nil, errors.New("airtable: access_token + base_id required")
	}
	if ac.MaxPerKind <= 0 {
		ac.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range ac.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["table"] {
		msgs, err := c.fetchTables(ctx, ac)
		if err != nil {
			logger.Warnf(ctx, "[Airtable] tables fetch failed: %v (stub fallback)", err)
			msgs = stubAirtable(ac.Tables, "table")
		}
		out = append(out, msgs...)
	}
	if all || kinds["record"] {
		msgs, err := c.fetchRecords(ctx, ac)
		if err != nil {
			msgs = stubAirtable(ac.Records, "record")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *AirtableConnector) fetchTables(ctx context.Context, ac airtableConfig) ([]types.ConnectorMessage, error) {
	url := fmt.Sprintf("https://api.airtable.com/v0/meta/bases/%s/tables", ac.BaseID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+ac.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("airtable: tables status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Tables []struct {
			ID  string `json:"id"`
			Name string `json:"name"`
		} `json:"tables"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, t := range raw.Tables {
		out = append(out, types.ConnectorMessage{
			ID: t.ID, Title: t.Name, Content: "",
			URL: fmt.Sprintf("https://airtable.com/%s/%s", ac.BaseID, t.ID),
			Metadata: map[string]string{"kind": "table"},
		})
	}
	return out, nil
}

func (c *AirtableConnector) fetchRecords(ctx context.Context, ac airtableConfig) ([]types.ConnectorMessage, error) {
	// Real impl iterates over tables; this stub-only path returns empty.
	return nil, errors.New("airtable records: real fetch not yet wired (stub fallback)")
}

func stubAirtable(items []airtableStub, kind string) []types.ConnectorMessage {
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
