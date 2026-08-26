/**
 * Wiki search v2 — Build #19 / P2.x.a
 *
 * Mirrors `internal/types/wiki_search.go`. The v2 endpoint replaces the
 * legacy regex `~*` search with PostgreSQL tsvector + ts_headline +
 * ts_rank, returns a lightweight `{hits, total, took_ms, kb_ids, query}`
 * payload, and applies per-page ACL + KB scoping on the server.
 *
 * `Snippet` already carries `<mark>` HTML from ts_headline; clients must
 * render via `v-html` and not re-highlight on the client.
 */

export interface WikiSearchV2Hit {
  slug: string
  title: string
  /** Server-rendered with `<mark>` highlights; render via v-html. */
  snippet: string
  score: number
  kb_id: string
  kb_name?: string
  page_type: string
  updated_at?: string
}

export interface WikiSearchV2Result {
  hits: WikiSearchV2Hit[]
  total: number
  took_ms: number
  kb_ids: string[]
  query: string
}

export interface WikiSearchV2Request {
  q: string
  /** Optional cross-KB scope. Empty means "all KBs visible to caller". */
  kb_ids?: string[]
  page_types?: string[]
  limit?: number
  offset?: number
}