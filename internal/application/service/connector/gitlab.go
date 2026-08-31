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

// GitLabConnector ingests Issues, Merge Requests and Snippets from a
// GitLab project. Uses the REST API v4 with a personal / project /
// group access token. Falls back to a stub mode for offline tests.
//
// Expected config JSON:
//
//	{
//	  "token":      "glpat-...",
//	  "project_id": "weknora/weknora",   // numeric or path-encoded
//	  "base_url":   "https://gitlab.com", // optional, default gitlab.com
//	  "kinds":      ["issue","mr","snippet"],
//	  "state":      "opened",
//	  "lookback":   "168h",
//	  "max_per_kind": 50,
//	  "issues":     [...],   // optional stubs
//	  "mrs":        [...],
//	  "snippets":   [...]
//	}
type GitLabConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewGitLabConnector() *GitLabConnector {
	return &GitLabConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (GitLabConnector) Kind() types.ConnectorKind { return types.ConnectorGitLab }

type gitlabConfig struct {
	Token      string             `json:"token"`
	ProjectID  string             `json:"project_id"`
	BaseURL    string             `json:"base_url"`
	Kinds      []string           `json:"kinds"`
	State      string             `json:"state"`
	Lookback   string             `json:"lookback"`
	MaxPerKind int                `json:"max_per_kind"`
	Issues     []gitlabIssueStub  `json:"issues"`
	MRs        []gitlabMRStub     `json:"mrs"`
	Snippets   []gitlabSnippetStub `json:"snippets"`
}

type gitlabIssueStub struct {
	IID       int       `json:"iid"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

type gitlabMRStub struct {
	IID       int       `json:"iid"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

type gitlabSnippetStub struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

func (c *GitLabConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var gc gitlabConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &gc); err != nil {
			return nil, fmt.Errorf("gitlab: invalid config: %w", err)
		}
	}
	if gc.ProjectID == "" {
		return nil, errors.New("gitlab: project_id is required")
	}
	if gc.BaseURL == "" {
		gc.BaseURL = "https://gitlab.com"
	}
	if gc.State == "" {
		gc.State = "all"
	}
	if gc.MaxPerKind <= 0 {
		gc.MaxPerKind = 50
	}
	out := []types.ConnectorMessage{}
	kinds := map[string]bool{}
	for _, k := range gc.Kinds {
		kinds[k] = true
	}
	all := len(kinds) == 0

	if all || kinds["issue"] {
		msgs, err := c.fetchIssues(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitLab] issues fetch failed: %v (falling back to stub)", err)
			msgs = c.stubIssues(gc)
		}
		out = append(out, msgs...)
	}
	if all || kinds["mr"] {
		msgs, err := c.fetchMRs(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitLab] MRs fetch failed: %v (falling back to stub)", err)
			msgs = c.stubMRs(gc)
		}
		out = append(out, msgs...)
	}
	if all || kinds["snippet"] {
		msgs, err := c.fetchSnippets(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitLab] snippets fetch failed: %v (falling back to stub)", err)
			msgs = c.stubSnippets(gc)
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *GitLabConnector) fetchIssues(ctx context.Context, gc gitlabConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("gitlab: token missing")
	}
	pid := strings.ReplaceAll(gc.ProjectID, "/", "%2F")
	url := fmt.Sprintf("%s/api/v4/projects/%s/issues?state=%s&per_page=%d",
		gc.BaseURL, pid, gc.State, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("PRIVATE-TOKEN", gc.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab: status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		IID       int       `json:"iid"`
		Title     string    `json:"title"`
		Body      string    `json:"description"`
		State     string    `json:"state"`
		Author    struct{ Username string `json:"username"` } `json:"author"`
		WebURL    string    `json:"web_url"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("issue-%d", r.IID),
			Title:     r.Title,
			Content:   r.Body,
			Author:    r.Author.Username,
			URL:       r.WebURL,
			Timestamp: r.UpdatedAt,
			Metadata:  map[string]string{"kind": "issue", "state": r.State, "iid": fmt.Sprintf("%d", r.IID)},
		})
	}
	return out, nil
}

func (c *GitLabConnector) fetchMRs(ctx context.Context, gc gitlabConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("gitlab: token missing")
	}
	pid := strings.ReplaceAll(gc.ProjectID, "/", "%2F")
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=%s&per_page=%d",
		gc.BaseURL, pid, gc.State, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("PRIVATE-TOKEN", gc.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab: mrs status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		IID       int       `json:"iid"`
		Title     string    `json:"title"`
		Body      string    `json:"description"`
		State     string    `json:"state"`
		Author    struct{ Username string `json:"username"` } `json:"author"`
		WebURL    string    `json:"web_url"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("mr-%d", r.IID),
			Title:     r.Title,
			Content:   r.Body,
			Author:    r.Author.Username,
			URL:       r.WebURL,
			Timestamp: r.UpdatedAt,
			Metadata:  map[string]string{"kind": "merge_request", "state": r.State, "iid": fmt.Sprintf("%d", r.IID)},
		})
	}
	return out, nil
}

func (c *GitLabConnector) fetchSnippets(ctx context.Context, gc gitlabConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("gitlab: token missing")
	}
	pid := strings.ReplaceAll(gc.ProjectID, "/", "%2F")
	url := fmt.Sprintf("%s/api/v4/projects/%s/snippets?per_page=%d",
		gc.BaseURL, pid, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("PRIVATE-TOKEN", gc.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab: snippets status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		ID        int       `json:"id"`
		Title     string    `json:"title"`
		Body      string    `json:"description"`
		Author    struct{ Username string `json:"username"` } `json:"author"`
		WebURL    string    `json:"web_url"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("snippet-%d", r.ID),
			Title:     r.Title,
			Content:   r.Body,
			Author:    r.Author.Username,
			URL:       r.WebURL,
			Timestamp: r.UpdatedAt,
			Metadata:  map[string]string{"kind": "snippet", "id": fmt.Sprintf("%d", r.ID)},
		})
	}
	return out, nil
}

func (c *GitLabConnector) stubIssues(gc gitlabConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.Issues))
	for _, i := range gc.Issues {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("issue-%d", i.IID),
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.Author,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": "issue", "state": i.State, "stub": "true"},
		})
	}
	return out
}

func (c *GitLabConnector) stubMRs(gc gitlabConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.MRs))
	for _, m := range gc.MRs {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("mr-%d", m.IID),
			Title:     m.Title,
			Content:   m.Body,
			Author:    m.Author,
			URL:       m.URL,
			Timestamp: m.UpdatedAt,
			Metadata:  map[string]string{"kind": "merge_request", "state": m.State, "stub": "true"},
		})
	}
	return out
}

func (c *GitLabConnector) stubSnippets(gc gitlabConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.Snippets))
	for _, s := range gc.Snippets {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("snippet-%d", s.ID),
			Title:     s.Title,
			Content:   s.Body,
			Author:    s.Author,
			URL:       s.URL,
			Timestamp: s.UpdatedAt,
			Metadata:  map[string]string{"kind": "snippet", "stub": "true"},
		})
	}
	return out
}
