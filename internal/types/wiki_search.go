// Package types — Build #19 / P2.x.a wiki search v2 types.
//
// Mirrors frontend/src/api/wiki/searchV2Types.ts. The v2 endpoint replaces
// the regex `~*` Search with PostgreSQL tsvector + ts_headline + ts_rank,
// returns a lightweight payload (snippet + score), and joins wiki_page_acl
// so per-page DENY entries filter out hits before they reach the client.
//
// Legacy endpoint (`?legacy=1` or no `v=2` query) is untouched for 6
// months, giving clients a deprecation window.
package types

import "time"

// WikiSearchV2Hit is one search result row, server-side rendered with
// `<mark>` tags inside Snippet so the client renders via v-html.
type WikiSearchV2Hit struct {
	Slug     string  `json:"slug"`
	Title    string  `json:"title"`
	Snippet  string  `json:"snippet"`
	Score    float64 `json:"score"`
	KBID     string  `json:"kb_id"`
	KBName   string  `json:"kb_name,omitempty"`
	PageType string  `json:"page_type"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// WikiSearchV2Result is the envelope returned by the v2 endpoint. Total
// is the unpaginated row count under the same filters so the client can
// render "showing 1-20 of N".
type WikiSearchV2Result struct {
	Hits    []WikiSearchV2Hit `json:"hits"`
	Total   int               `json:"total"`
	TookMS  int               `json:"took_ms"`
	KBIDs   []string          `json:"kb_ids"`
	Query   string            `json:"query"`
}

// WikiSearchV2Request is what the handler parses out of the query string.
//
// KB IDs is the cross-KB scope; nil/empty means "all KBs visible to the
// caller". PageTypes is an optional page-type filter (concept/entity/
// summary/synthesis/comparison/index). Limit/offset standardise
// pagination; defaults are 20 / 0, clamp bounds 1..100 / >=0.
type WikiSearchV2Request struct {
	Query     string   `json:"q"`
	KBIDs     []string `json:"kb_ids,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
	PageTypes []string `json:"page_types,omitempty"`
}