// Package types — Build #19 / P2.x.a wiki search v2 types.
//
// Mirrors frontend/src/api/wiki/searchV2Types.ts. The v2 endpoint replaces
// the regex `~*` Search with PostgreSQL tsvector + ts_headline + ts_rank,
// returns a lightweight payload (snippet + score), and joins wiki_page_acl
// so per-page DENY entries filter out hits before they reach the client.
//
// Build #19.x extends v2 with Chinese tsvector (jieba-tokenized column
// `content_ts_zh`, migration 000096), pg_trgm fuzzy matching for English
// typos, and a `matched_by` discriminator so the client can surface "this
// hit came from fuzzy / partial / zh tsvector" when it matters.
//
// Legacy endpoint (`?legacy=1` or no `v=2` query) is untouched for 6
// months, giving clients a deprecation window.
package types

import "time"

// WikiSearchV2MatchSource names the index arm that produced a hit.
// Build #19.x: clients can render the chip / hint to surface which path
// matched when the user did not get the exact hit they expected.
type WikiSearchV2MatchSource string

const (
	// WikiSearchV2MatchTSZh — Chinese tsvector on jieba-tokenized
	// `content_ts_zh`. Highest priority in the CASE-WHEN short-circuit.
	WikiSearchV2MatchTSZh WikiSearchV2MatchSource = "ts_zh"
	// WikiSearchV2MatchTSSimple — Build #19 default `to_tsvector('simple',
	// coalesce(title,'') || ' ' || coalesce(content,''))` path. English /
	// Western text, numbers, identifiers.
	WikiSearchV2MatchTSSimple WikiSearchV2MatchSource = "ts_simple"
	// WikiSearchV2MatchTrgm — pg_trgm `similarity()` against
	// `lower(title)`. English typos. Only consulted when req.Fuzzy is true.
	WikiSearchV2MatchTrgm WikiSearchV2MatchSource = "trgm"
	// WikiSearchV2MatchPartial — `lower(title) LIKE '%q%'` substring
	// fallback. Only consulted when req.PartialMatch is true. Highest
	// false-positive rate; always last priority.
	WikiSearchV2MatchPartial WikiSearchV2MatchSource = "partial"
)

// WikiSearchV2Hit is one search result row, server-side rendered with
// `<mark>` tags inside Snippet so the client renders via v-html.
type WikiSearchV2Hit struct {
	Slug      string                `json:"slug"`
	Title     string                `json:"title"`
	Snippet   string                `json:"snippet"`
	Score     float64               `json:"score"`
	KBID      string                `json:"kb_id"`
	KBName    string                `json:"kb_name,omitempty"`
	PageType  string                `json:"page_type"`
	MatchedBy WikiSearchV2MatchSource `json:"matched_by,omitempty"`
	UpdatedAt time.Time             `json:"updated_at,omitempty"`
}

// WikiSearchV2Result is the envelope returned by the v2 endpoint. Total
// is the unpaginated row count under the same filters so the client can
// render "showing 1-20 of N".
type WikiSearchV2Result struct {
	Hits   []WikiSearchV2Hit `json:"hits"`
	Total  int               `json:"total"`
	TookMS int               `json:"took_ms"`
	KBIDs  []string          `json:"kb_ids"`
	Query  string            `json:"query"`
}

// WikiSearchV2Request is what the handler parses out of the query string.
//
// KB IDs is the cross-KB scope; nil/empty means "all KBs visible to the
// caller". PageTypes is an optional page-type filter (concept/entity/
// summary/synthesis/comparison/index). Limit/offset standardise
// pagination; defaults are 20 / 0, clamp bounds 1..100 / >=0.
//
// Fuzzy enables the pg_trgm path (English typos); default true so single-
// character mistakes still surface hits.
//
// PartialMatch enables the `LIKE '%q%'` substring fallback. Default false
// — false-positive rate is high; only opt-in when fuzzy + ts_simple have
// already returned nothing.
type WikiSearchV2Request struct {
	Query        string   `json:"q"`
	KBIDs        []string `json:"kb_ids,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
	PageTypes    []string `json:"page_types,omitempty"`
	Fuzzy        bool     `json:"fuzzy,omitempty"`
	PartialMatch bool     `json:"partial_match,omitempty"`
}