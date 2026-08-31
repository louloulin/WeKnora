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

// GitHubConnector ingests Issues, Pull Requests and Discussions from a
// GitHub repository. Uses the REST API with a personal access token
// (classic or fine-grained) or installation token. Falls back to a
// stub mode when no token is configured so unit tests + offline dev
// remain viable.
//
// Expected config JSON:
//
//	{
//	  "token":      "ghp_...",          // personal access token
//	  "owner":      "Tencent",
//	  "repo":       "WeKnora",
//	  "kinds":      ["issue","pr","discussion"],  // optional, default all
//	  "state":      "open",             // optional: open|closed|all
//	  "lookback":   "168h",             // optional: time window (Go duration)
//	  "max_per_kind": 50,               // optional: cap per kind (default 50)
//	  "issues":     [...],              // optional: stub fallback
//	  "pulls":      [...],              // optional: stub fallback
//	  "discussions":[...]               // optional: stub fallback
//	}
type GitHubConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewGitHubConnector() *GitHubConnector {
	return &GitHubConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (GitHubConnector) Kind() types.ConnectorKind { return types.ConnectorGitHub }

type githubConfig struct {
	Token       string             `json:"token"`
	Owner       string             `json:"owner"`
	Repo        string             `json:"repo"`
	Kinds       []string           `json:"kinds"`
	State       string             `json:"state"`
	Lookback    string             `json:"lookback"`
	MaxPerKind  int                `json:"max_per_kind"`
	Issues      []githubIssueStub  `json:"issues"`
	Pulls       []githubPullStub   `json:"pulls"`
	Discussions []githubDiscussStub `json:"discussions"`
}

type githubIssueStub struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	User      string    `json:"user"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

type githubPullStub struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	User      string    `json:"user"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

type githubDiscussStub struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	User      string    `json:"user"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
}

// Fetch implements interfaces.Connector.
func (c *GitHubConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	var gc githubConfig
	if cfg.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(cfg.ConfigJSON), &gc); err != nil {
			return nil, fmt.Errorf("github: invalid config: %w", err)
		}
	}
	if gc.Owner == "" || gc.Repo == "" {
		return nil, errors.New("github: owner and repo are required")
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

	// Issue fetch
	if all || kinds["issue"] {
		msgs, err := c.fetchIssues(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitHub] issues fetch failed: %v (falling back to stub)", err)
			msgs = c.stubIssues(gc)
		}
		out = append(out, msgs...)
	}
	// PR fetch
	if all || kinds["pr"] {
		msgs, err := c.fetchPulls(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitHub] pulls fetch failed: %v (falling back to stub)", err)
			msgs = c.stubPulls(gc)
		}
		out = append(out, msgs...)
	}
	// Discussion fetch
	if all || kinds["discussion"] {
		msgs, err := c.fetchDiscussions(ctx, gc)
		if err != nil {
			logger.Warnf(ctx, "[GitHub] discussions fetch failed: %v (falling back to stub)", err)
			msgs = c.stubDiscussions(gc)
		}
		out = append(out, msgs...)
	}
	return out, nil
}

func (c *GitHubConnector) fetchIssues(ctx context.Context, gc githubConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("github: token missing")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=%s&per_page=%d",
		gc.Owner, gc.Repo, gc.State, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "token "+gc.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		User      struct{ Login string `json:"login"` } `json:"user"`
		HTMLURL   string    `json:"html_url"`
		UpdatedAt time.Time `json:"updated_at"`
		PullRequest *struct{} `json:"pull_request"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw {
		if r.PullRequest != nil {
			continue // skip PRs in issue endpoint
		}
		body := strings.TrimSpace(r.Body)
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("issue-%d", r.Number),
			Title:     r.Title,
			Content:   body,
			Author:    r.User.Login,
			URL:       r.HTMLURL,
			Timestamp: r.UpdatedAt,
			Metadata:  map[string]string{"kind": "issue", "state": r.State, "number": fmt.Sprintf("%d", r.Number)},
		})
	}
	return out, nil
}

func (c *GitHubConnector) fetchPulls(ctx context.Context, gc githubConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("github: token missing")
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=%s&per_page=%d",
		gc.Owner, gc.Repo, gc.State, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "token "+gc.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: pulls status=%d body=%s", resp.StatusCode, string(body))
	}
	var raw []struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		State     string    `json:"state"`
		User      struct{ Login string `json:"login"` } `json:"user"`
		HTMLURL   string    `json:"html_url"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, r := range raw {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("pr-%d", r.Number),
			Title:     r.Title,
			Content:   r.Body,
			Author:    r.User.Login,
			URL:       r.HTMLURL,
			Timestamp: r.UpdatedAt,
			Metadata:  map[string]string{"kind": "pull_request", "state": r.State, "number": fmt.Sprintf("%d", r.Number)},
		})
	}
	return out, nil
}

func (c *GitHubConnector) fetchDiscussions(ctx context.Context, gc githubConfig) ([]types.ConnectorMessage, error) {
	if gc.Token == "" {
		return nil, errors.New("github: token missing")
	}
	// GitHub Discussions require GraphQL. The REST endpoint is the
	// `/discussions` route on the repo. For simplicity we POST a tiny
	// GraphQL query when running against a real token; offline stubs
	// still work.
	query := fmt.Sprintf(`{"query":"{repository(owner:\"%s\",name:\"%s\"){discussions(first:%d){nodes{number,title body,url,updatedAt,author{login}}}}}"}`,
		gc.Owner, gc.Repo, gc.MaxPerKind)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", strings.NewReader(query))
	req.Header.Set("Authorization", "bearer "+gc.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: discussions status=%d body=%s", resp.StatusCode, string(body))
	}
	var envelope struct {
		Data struct {
			Repository struct {
				Discussions struct {
					Nodes []struct {
						Number    int       `json:"number"`
						Title     string    `json:"title"`
						Body      string    `json:"body"`
						URL       string    `json:"url"`
						UpdatedAt time.Time `json:"updatedAt"`
						Author    struct{ Login string `json:"login"` } `json:"author"`
					} `json:"nodes"`
				} `json:"discussions"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	out := []types.ConnectorMessage{}
	for _, d := range envelope.Data.Repository.Discussions.Nodes {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("discussion-%d", d.Number),
			Title:     d.Title,
			Content:   d.Body,
			Author:    d.Author.Login,
			URL:       d.URL,
			Timestamp: d.UpdatedAt,
			Metadata:  map[string]string{"kind": "discussion", "number": fmt.Sprintf("%d", d.Number)},
		})
	}
	return out, nil
}

func (c *GitHubConnector) stubIssues(gc githubConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.Issues))
	for _, i := range gc.Issues {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("issue-%d", i.Number),
			Title:     i.Title,
			Content:   i.Body,
			Author:    i.User,
			URL:       i.URL,
			Timestamp: i.UpdatedAt,
			Metadata:  map[string]string{"kind": "issue", "state": i.State, "stub": "true"},
		})
	}
	return out
}

func (c *GitHubConnector) stubPulls(gc githubConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.Pulls))
	for _, p := range gc.Pulls {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("pr-%d", p.Number),
			Title:     p.Title,
			Content:   p.Body,
			Author:    p.User,
			URL:       p.URL,
			Timestamp: p.UpdatedAt,
			Metadata:  map[string]string{"kind": "pull_request", "state": p.State, "stub": "true"},
		})
	}
	return out
}

func (c *GitHubConnector) stubDiscussions(gc githubConfig) []types.ConnectorMessage {
	out := make([]types.ConnectorMessage, 0, len(gc.Discussions))
	for _, d := range gc.Discussions {
		out = append(out, types.ConnectorMessage{
			ID:        fmt.Sprintf("discussion-%d", d.Number),
			Title:     d.Title,
			Content:   d.Body,
			Author:    d.User,
			URL:       d.URL,
			Timestamp: d.UpdatedAt,
			Metadata:  map[string]string{"kind": "discussion", "stub": "true"},
		})
	}
	return out
}
