package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// RSSConnector is the v0.7.25 Build #25 (G05) real implementation
// of the RSS / Atom ingestion connector. It uses gofeed to parse the
// feed listed in the connector config, optionally constrained to a
// single category or window of recent items.
//
// Expected config JSON:
//
//	{
//	  "feed_url": "https://blog.example.com/feed.xml",
//	  "category": "release-notes",   // optional: filter by category
//	  "max_items": 50,               // optional: cap (default 50, max 500)
//	  "user_agent": "WeKnora/0.7",   // optional UA override
//	  "since_iso": "2026-08-01T00:00:00Z" // optional: ignore items older than this
//	}
//
// Fetch is read-only; the connector never mutates the source feed.
// Dedup against re-ingest of the same GUID is the caller's job
// (the IngestConnector.Ingest path stores by GUID).
type RSSConnector struct {
	// HTTPClient is the http client used to fetch the feed. Tests
	// inject a stub server here. Defaults to http.DefaultClient.
	HTTPClient *http.Client
	// Now returns the current time. Tests override to make the
	// "since" window deterministic. Defaults to time.Now.
	Now func() time.Time
}

// NewRSSConnector returns an RSSConnector with sensible defaults.
func NewRSSConnector() *RSSConnector {
	return &RSSConnector{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

// Kind implements interfaces.Connector.
func (r *RSSConnector) Kind() types.ConnectorKind { return types.ConnectorRSS }

type rssConfig struct {
	FeedURL    string `json:"feed_url"`
	Category   string `json:"category"`
	MaxItems   int    `json:"max_items"`
	UserAgent  string `json:"user_agent"`
	SinceISO   string `json:"since_iso"`
}

// Fetch implements interfaces.Connector.
func (r *RSSConnector) Fetch(ctx context.Context, cfg interfaces.ConnectorRuntimeConfig) ([]types.ConnectorMessage, error) {
	if r.HTTPClient == nil {
		r.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if r.Now == nil {
		r.Now = time.Now
	}

	var rc rssConfig
	if err := json.Unmarshal([]byte(cfg.ConfigJSON), &rc); err != nil {
		return nil, fmt.Errorf("rss: parse config: %w", err)
	}
	if strings.TrimSpace(rc.FeedURL) == "" {
		return nil, errors.New("rss: feed_url required")
	}

	if rc.MaxItems <= 0 {
		rc.MaxItems = 50
	}
	if rc.MaxItems > 500 {
		rc.MaxItems = 500
	}

	var since time.Time
	if rc.SinceISO != "" {
		t, err := parseEmailDate(rc.SinceISO)
		if err != nil {
			return nil, fmt.Errorf("rss: invalid since_iso: %w", err)
		}
		since = t
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rc.FeedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("rss: build request: %w", err)
	}
	ua := rc.UserAgent
	if ua == "" {
		ua = "WeKnora/0.7.25 (+https://github.com/Tencent/WeKnora)"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.5")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rss: fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rss: feed returned status %d", resp.StatusCode)
	}

	parser := gofeed.NewParser()
	feed, err := parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rss: parse feed: %w", err)
	}

	out := make([]types.ConnectorMessage, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item == nil {
			continue
		}
		if rc.Category != "" && !itemInCategory(item.Categories, rc.Category) {
			continue
		}
		var ts time.Time
		if item.PublishedParsed != nil {
			ts = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			ts = *item.UpdatedParsed
		} else {
			ts = r.Now()
		}
		if !since.IsZero() && ts.Before(since) {
			continue
		}

		id := item.GUID
		if id == "" {
			id = item.Link
		}

		author := ""
		if item.Author != nil {
			author = item.Author.Name
		}

		title := item.Title
		if title == "" {
			title = truncate(item.Link, 60)
		}

		out = append(out, types.ConnectorMessage{
			ID:        id,
			Title:     title,
			Content:   buildRSSBody(item),
			Author:    author,
			URL:       item.Link,
			Timestamp: ts,
			Metadata: map[string]string{
				"source":  "rss",
				"feed":    feed.Title,
				"feedurl": rc.FeedURL,
			},
		})

		if len(out) >= rc.MaxItems {
			break
		}
	}

	logger.Infof(ctx, "rss: connector fetched %d items from %s (feed=%q)",
		len(out), rc.FeedURL, feed.Title)
	return out, nil
}

// itemInCategory returns true when the item lists the given category
// (case-insensitive). Categories may be slice of strings or single
// string per gofeed; here we normalize to []string.
func itemInCategory(cats []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, c := range cats {
		if strings.ToLower(strings.TrimSpace(c)) == want {
			return true
		}
	}
	return false
}

// buildRSSBody assembles a Markdown body from the parsed feed item.
// Title + content + link is the conventional readable shape; we
// drop the description when content is empty so dedup-by-content
// stays stable across re-fetches.
func buildRSSBody(item *gofeed.Item) string {
	var b strings.Builder
	if item.Title != "" {
		b.WriteString("# ")
		b.WriteString(item.Title)
		b.WriteString("\n\n")
	}
	if item.Author != nil && item.Author.Name != "" {
		b.WriteString("_By ")
		b.WriteString(item.Author.Name)
		b.WriteString("_\n\n")
	}
	if item.Content != "" {
		b.WriteString(item.Content)
	} else if item.Description != "" {
		b.WriteString(item.Description)
	}
	if item.Link != "" {
		b.WriteString("\n\n---\n[Source](")
		b.WriteString(item.Link)
		b.WriteString(")")
	}
	return strings.TrimSpace(b.String())
}
