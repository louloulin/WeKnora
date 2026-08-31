package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LinearConnector ingests Issues and Projects from Linear via the
// GraphQL API (https://api.linear.app/graphql). Uses a personal API
// key or OAuth2 token. Falls back to stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "api_key":      "lin_api_...",
//	  "team_id":      "uuid-of-team",
//	  "kinds":        ["issue","project"],
//	  "lookback":     "168h",
//	  "max_per_kind": 50,
//	  "issues":   [...],   // optional stubs
//	  "projects": [...]
//	}
type LinearConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewLinearConnector() *LinearConnector {
	return &LinearConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (LinearConnector) Kind() types.ConnectorKind { return types.ConnectorLinear }

type linearConfig struct {
	APIKey     string         `json:"api_key"`
	TeamID     string         `json:"team_id"`
	Kinds      []string       `json:"kinds"`
	Lookback   string         `json:"lookback"`
	MaxPerKind int            `json:"max_per_kind"`
	Issues     []linearStub   `json:"issues"`
	Projects   []linearStub   `json:"projects"`
}

type linearStub struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *LinearConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var lc linearConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &lc); err != nil {
			return nil, fmt.Errorf("linear: invalid config: %w", err)
		}
	}
	if lc.MaxPerKind <= 0 {
		lc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range lc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0
	if all || kinds["issue"] {
		msgs, err := c.fetchIssues(ctx, lc)
		if err != nil {
			logger.Warnf(ctx, "[Linear] issues fetch failed: %v (stub fallback)", err)
			msgs = stubLinear(lc.Issues, "issue")
		}
		out = append(out, msgs...)
	}
	if all || kinds["project"] {
		msgs, err := c.fetchProjects(ctx, lc)
		if err != nil {
			msgs = stubLinear(lc.Projects, "project")
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *LinearConnector) fetchIssues(ctx context.Context, lc linearConfig) ([]types.ConnectorMessage, error) {
	if lc.APIKey == "" {
		return nil, errors.New("linear: api_key required")
	}
	query := fmt.Sprintf(`{"query":"{issues(filter:{team:{id:{eq:\"%s\"}}},first:%d){nodes{id title description url updatedAt creator{name}}}}"}`,
		lc.TeamID, lc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/graphql", strings.NewReader(query))
	req.Header.Set("Authorization", lc.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("linear: issues status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Data struct {
			Issues struct {
				Nodes []struct {
					ID          string    `json:"id"`
					Title       string    `json:"title"`
					Description string    `json:"description"`
					URL         string    `json:"url"`
					UpdatedAt   time.Time `json:"updatedAt"`
					Creator     struct{ Name string `json:"name"` } `json:"creator"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, n := range raw.Data.Issues.Nodes {
		out = append(out, types.ConnectorMessage{
			ID: n.ID, Title: n.Title, Content: n.Description,
			Author: n.Creator.Name, URL: n.URL, Timestamp: n.UpdatedAt,
			Metadata: map[string]string{"kind": "issue"},
		})
	}
	return out, nil
}

func (c *LinearConnector) fetchProjects(ctx context.Context, lc linearConfig) ([]types.ConnectorMessage, error) {
	if lc.APIKey == "" {
		return nil, errors.New("linear: api_key required")
	}
	query := fmt.Sprintf(`{"query":"{projects(first:%d){nodes{id name description url updatedAt lead{name}}}}"}`, lc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/graphql", strings.NewReader(query))
	req.Header.Set("Authorization", lc.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("linear: projects status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw struct {
		Data struct {
			Projects struct {
				Nodes []struct {
					ID          string    `json:"id"`
					Name        string    `json:"name"`
					Description string    `json:"description"`
					URL         string    `json:"url"`
					UpdatedAt   time.Time `json:"updatedAt"`
					Lead        struct{ Name string `json:"name"` } `json:"lead"`
				} `json:"nodes"`
			} `json:"projects"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, n := range raw.Data.Projects.Nodes {
		out = append(out, types.ConnectorMessage{
			ID: n.ID, Title: n.Name, Content: n.Description,
			Author: n.Lead.Name, URL: n.URL, Timestamp: n.UpdatedAt,
			Metadata: map[string]string{"kind": "project"},
		})
	}
	return out, nil
}

func stubLinear(items []linearStub, kind string) []types.ConnectorMessage {
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
