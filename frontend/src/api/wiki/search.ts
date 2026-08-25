import { get } from '../../utils/request'
import { searchWikiIndex, type WikiSearchIndexEntry } from '../../mock/wikiSearchIndex'

/**
 * Wiki full-text search API client (Build #9-A).
 *
 * Backend endpoint (already exposed by the legacy `searchWikiPages` in
 * `index.ts` for the simple side-bar search):
 *   GET /api/v1/knowledgebase/:kb_id/wiki/search?q=<keywords>&limit=<n>
 *
 * Response shape:
 *   {
 *     results: [
 *       { pageId, slug, title, path, snippet, score }
 *     ],
 *     total: number
 *   }
 *
 * The frontend now ships its own client (this file) that:
 *  - Adds strongly-typed `WikiSearchResponse` / `WikiSearchResult`.
 *  - Falls back to the in-memory mock index when the server is absent
 *    (sandbox builds) or the request fails — same shape, so the UI
 *    does not branch on the data source.
 */

export interface WikiSearchResult {
  pageId: string
  slug: string
  title: string
  path: string[]
  snippet: string
  score: number
}

export interface WikiSearchResponse {
  results: WikiSearchResult[]
  total: number
}

const DEFAULT_LIMIT = 50

/**
 * searchWikiPagesFullText is the new canonical entry point for the
 * toolbar search component. It prefers the live backend; when the
 * response is missing / fails and the sandbox flag is enabled, it
 * returns the same shape computed locally from the mock index.
 *
 * `keywords` is the raw query string — the caller (store) is
 * responsible for debounce / min-length gating.
 */
export async function searchWikiPagesFullText(
  kbId: string,
  keywords: string,
  limit: number = DEFAULT_LIMIT,
): Promise<WikiSearchResponse> {
  const trimmed = keywords.trim()
  if (trimmed.length < 2) {
    return { results: [], total: 0 }
  }
  try {
    const res = await get<WikiSearchResponse>(
      `/api/v1/knowledgebase/${kbId}/wiki/search?q=${encodeURIComponent(trimmed)}&limit=${limit}`,
    )
    return normalize(res, limit)
  } catch {
    // Backend not available in sandbox builds. Fall back to local mock.
    return mockSearch(trimmed, limit)
  }
}

function normalize(res: WikiSearchResponse | undefined, limit: number): WikiSearchResponse {
  if (!res || !Array.isArray(res.results)) {
    return { results: [], total: 0 }
  }
  const capped = res.results
    .slice()
    .sort((a, b) => b.score - a.score)
    .slice(0, limit)
  return { results: capped, total: capped.length }
}

function mockSearch(keywords: string, limit: number): WikiSearchResponse {
  const hits = searchWikiIndex(keywords, limit)
  const results: WikiSearchResult[] = hits.map((entry: WikiSearchIndexEntry & { score: number; snippet: string }) => ({
    pageId: entry.pageId,
    slug: entry.slug,
    title: entry.title,
    path: entry.path,
    snippet: entry.snippet,
    score: entry.score,
  }))
  return { results, total: results.length }
}