import { get } from '../../utils/request'
import type {
  WikiSearchV2Hit,
  WikiSearchV2Request,
  WikiSearchV2Result,
} from './searchV2Types'

/**
 * Build #19 / P2.x.a — wiki search v2 client.
 *
 * Default behaviour: GET with `?v=2` so the backend routes to the v2
 * service. Falls back to legacy on transport-level failure so the user
 * still gets a usable search box even when the v2 endpoint is down.
 */

const DEFAULT_LIMIT = 20

/**
 * searchWikiPagesV2 hits the v2 endpoint with kb_ids[] cross-KB scope
 * and returns the lightweight `{hits, total, took_ms, kb_ids, query}`
 * payload. Snippet already contains `<mark>` HTML; the caller is
 * expected to render via v-html.
 */
export async function searchWikiPagesV2(
  kbId: string,
  opts: WikiSearchV2Request,
): Promise<WikiSearchV2Result> {
  const params = buildParams(kbId, opts)
  const res = await get<unknown>(params)
  return normalize(res)
}

export function buildParams(kbId: string, opts: WikiSearchV2Request): string {
  const q = (opts.q ?? '').trim()
  const params = new URLSearchParams()
  params.set('v', '2')
  if (q) params.set('q', q)
  if (opts.kb_ids && opts.kb_ids.length > 0) {
    opts.kb_ids.forEach((id) => params.append('kb_ids[]', id))
  }
  if (opts.page_types && opts.page_types.length > 0) {
    opts.page_types.forEach((t) => params.append('page_types[]', t))
  }
  const limit = clampLimit(opts.limit)
  params.set('limit', String(limit))
  if (opts.offset && opts.offset > 0) {
    params.set('offset', String(opts.offset))
  }
  // Build #19.x — fuzzy / partial_match toggles. We only emit the params
  // when the caller flipped them; the server's defaults already match
  // (fuzzy=true, partial_match=false), so absent = use server default.
  if (opts.fuzzy === false) {
    params.set('fuzzy', 'false')
  }
  if (opts.partial_match === true) {
    params.set('partial_match', 'true')
  }
  return `/api/v1/knowledgebase/${kbId}/wiki/search?${params.toString()}`
}

export function clampLimit(input: number | undefined): number {
  if (!input || input <= 0) return DEFAULT_LIMIT
  if (input > 100) return 100
  return input
}

function normalize(raw: unknown): WikiSearchV2Result {
  const r = (raw ?? {}) as Partial<WikiSearchV2Result>
  const hits = Array.isArray(r.hits) ? (r.hits as WikiSearchV2Hit[]) : []
  return {
    hits,
    total: typeof r.total === 'number' ? r.total : hits.length,
    took_ms: typeof r.took_ms === 'number' ? r.took_ms : 0,
    kb_ids: Array.isArray(r.kb_ids) ? r.kb_ids : [],
    query: typeof r.query === 'string' ? r.query : '',
  }
}

/**
 * searchWikiPagesLegacy is the dual-track fallback for the 6-month
 * deprecation window. Returns `{pages}` shaped data so the existing
 * WikiSearchResults can render even if the v2 endpoint is unavailable.
 *
 * Kept in this file so all search v2 callers import a single module.
 */
export async function searchWikiPagesLegacy(
  kbId: string,
  q: string,
  limit: number = DEFAULT_LIMIT,
): Promise<unknown> {
  const trimmed = q.trim()
  const url = `/api/v1/knowledgebase/${kbId}/wiki/search?legacy=1&q=${encodeURIComponent(
    trimmed,
  )}&limit=${limit}`
  return get<unknown>(url)
}