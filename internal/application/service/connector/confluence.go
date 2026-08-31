package connector

import (
	"context"
	"encoding/base64"
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

// ConfluenceConnector is the v0.7.25 Build #25 (G05) real
// implementation of the Confluence ingestion connector. It calls
// Confluence Cloud / Server REST API v1 to list pages in a space
// (or globally under CQL) and emits one ConnectorMessage per page.
//
// Expected config JSON:
//
//	{
//	  "base_url": "https://your-domain.atlassian.net/wiki",
//	  "auth": {
//	    "type": "basic",                 // "basic" or "bearer"
//	    "email": "bot@example.com",      // basic: account email
//	    "api_token": "ATATT...",         // basic: API token
//	    "token": "abcd"                  // bearer: PAT or OAuth token
//	  },
//	  "space_key": "ENG",                 // optional: scope to one space
//	  "cql": "type=page AND space=ENG",   // optional: overrides space_key
//	  "max_items": 100,                   // optional
//	  "limit_per_request": 25             // optional, max 100
//	}
type ConfluenceConnector struct {
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewConfluenceConnector() *ConfluenceConnector {
	return &ConfluenceConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

func (c *ConfluenceConnector) Kind() types.ConnectorKind { return types.ConnectorConfluence }

type confluenceAuth struct {
	Type     string `json:"type"`
	Email    string `json:"email"`
	APIToken string `json:"api_token"`
	Token    string `json:"token"`
}

type confluenceConfig struct {
	BaseURL         string         `json:"base_url"`
	Auth            confluenceAuth `json:"auth"`
	SpaceKey        string         `json:"space_key"`
	CQL             string         `json:"cql"`
	MaxItems        int            `json:"max_items"`
	LimitPerRequest int            `json:"limit_per_request"`
}

type confluenceSearchResponse struct {
	Results []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Type      string `json:"type"`
		URL       string `json:"url"`
		Excerpt   string `json:"excerpt"`
		Content   *struct {
			Body string `json:"-"` // body view varies; use content URL fetch below
		} `json:"content"`
		LastModified string `json:"lastModified"`
		History      *struct {
			CreatedBy struct {
				DisplayName string `json:"displayName"`
			} `json:"createdBy"`
		} `json:"history"`
		Space *struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"resultParentContainer"`
		Version *struct {
			Number int       `json:"number"`
			When   time.Time `json:"when"`
		} `json:"version"`
	} `json:"results"`
	Start    int  `json:"start"`
	Limit    int  `json:"limit"`
	Size     int  `json:"size"`
	TotalSize int `json:"totalSize,omitempty"`
}

// Fetch implements interfaces.Connector.
func (c *ConfluenceConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if c.Now == nil {
		c.Now = time.Now
	}

	var cc confluenceConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &cc); err != nil {
		return nil, fmt.Errorf("confluence: parse config: %w", err)
	}
	if strings.TrimSpace(cc.BaseURL) == "" {
		return nil, errors.New("confluence: base_url required")
	}
	if cc.Auth.Type == "" {
		return nil, errors.New("confluence: auth.type required")
	}

	if cc.MaxItems <= 0 {
		cc.MaxItems = 100
	}
	if cc.MaxItems > 1000 {
		cc.MaxItems = 1000
	}
	if cc.LimitPerRequest <= 0 || cc.LimitPerRequest > 100 {
		cc.LimitPerRequest = 25
	}

	cql := cc.CQL
	if cql == "" {
		if cc.SpaceKey != "" {
			cql = fmt.Sprintf("type=page AND space=%s", cc.SpaceKey)
		} else {
			cql = "type=page"
		}
	}

	start := 0
	out := make([]types.ConnectorMessage, 0, cc.MaxItems)

	for len(out) < cc.MaxItems {
		endpoint, err := buildConfluenceSearchURL(cc.BaseURL, cql, start, cc.LimitPerRequest)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("confluence: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "WeKnora/0.7.25")
		if err := applyConfluenceAuth(req, cc.Auth); err != nil {
			return nil, err
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("confluence: request failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("confluence: API status %d: %s", resp.StatusCode, truncate(string(body), 200))
		}

		var sr confluenceSearchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			return nil, fmt.Errorf("confluence: parse response: %w", err)
		}

		if len(sr.Results) == 0 {
			break
		}

		for _, page := range sr.Results {
			if page.Type != "page" && page.Type != "" {
				continue
			}
			ts := c.Now()
			if page.Version != nil && !page.Version.When.IsZero() {
				ts = page.Version.When
			} else if page.LastModified != "" {
				if parsed, perr := time.Parse(time.RFC3339, page.LastModified); perr == nil {
					ts = parsed
				}
			}

			author := ""
			if page.History != nil {
				author = page.History.CreatedBy.DisplayName
			}
			spaceKey := cc.SpaceKey
			if page.Space != nil {
				spaceKey = page.Space.Key
			}

			out = append(out, types.ConnectorMessage{
				ID:        page.ID,
				Title:     page.Title,
				Content:   buildConfluenceBody(page.Title, page.Excerpt, page.URL),
				Author:    author,
				URL:       page.URL,
				Timestamp: ts,
				Metadata: map[string]string{
					"source":    "confluence",
					"space":     spaceKey,
					"base_url":  cc.BaseURL,
					"page_id":   page.ID,
				},
			})
			if len(out) >= cc.MaxItems {
				break
			}
		}

		if len(sr.Results) < cc.LimitPerRequest {
			break
		}
		start += cc.LimitPerRequest
	}

	logger.Infof(ctx, "confluence: connector fetched %d pages (base=%s cql=%q)",
		len(out), cc.BaseURL, cql)
	return out, nil
}

func buildConfluenceSearchURL(baseURL, cql string, start, limit int) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/rest/api/content/search")
	if err != nil {
		return "", fmt.Errorf("confluence: bad base_url: %w", err)
	}
	q := u.Query()
	q.Set("cql", cql)
	q.Set("start", fmt.Sprintf("%d", start))
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("expand", "version,history.createdBy,resultParentContainer")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func applyConfluenceAuth(req *http.Request, a confluenceAuth) error {
	switch strings.ToLower(a.Type) {
	case "basic":
		if a.Email == "" || a.APIToken == "" {
			return errors.New("confluence: basic auth requires email + api_token")
		}
		cred := base64.StdEncoding.EncodeToString([]byte(a.Email + ":" + a.APIToken))
		req.Header.Set("Authorization", "Basic "+cred)
		return nil
	case "bearer", "pat", "oauth":
		if a.Token == "" {
			return errors.New("confluence: bearer auth requires token")
		}
		req.Header.Set("Authorization", "Bearer "+a.Token)
		return nil
	default:
		return fmt.Errorf("confluence: unsupported auth.type=%q", a.Type)
	}
}

func buildConfluenceBody(title, excerpt, pageURL string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	if excerpt != "" {
		// strip any HTML for safer Markdown roundtrip
		b.WriteString(stripSimpleHTML(excerpt))
		b.WriteString("\n\n")
	}
	if pageURL != "" {
		b.WriteString("---\n[Open in Confluence](")
		b.WriteString(pageURL)
		b.WriteString(")\n")
	}
	return strings.TrimSpace(b.String())
}

// stripSimpleHTML removes a tiny subset of HTML tags Confluence
// returns in `excerpt`. It is intentionally conservative — anything
// we don't recognize we leave alone so the downstream Markdown
// viewer can decide. We aim for stable content for dedup-by-hash,
// not a full HTML sanitizer.
func stripSimpleHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
