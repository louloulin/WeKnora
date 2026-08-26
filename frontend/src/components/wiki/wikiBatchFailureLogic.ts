// Build #15 — shared helpers for the failure-log drawer component.
// Pure functions only — no vue / i18n imports so the helpers can be
// unit-tested with `tsx --test` without a DOM.
//
// The runtime import is pinned to `batchTypes.ts` (NOT `api/wiki/index.ts`)
// because the index re-exports `request.ts`, which imports `@/i18n` via a
// Vite alias that tsx does not resolve in a plain Node test.

import {
  WikiBatchErrorCodeToI18nKey,
} from '../../api/wiki/batchTypes.ts'
import type { WikiBatchFailureGroupCount } from '../../api/wiki/batchTypes.ts'
// The drawer (.vue) imports the same constant via `api/wiki` (re-export
// from index.ts). Keep both paths consistent; tests use batchTypes.ts
// directly to dodge the `@/i18n` chain pulled in by request.ts.

// KnownCodes is the closed set of failure codes the per-slug worker
// can record today. Mirrors WikiBatchErrorCodeToI18nKey keys so the
// code-tabs UI can pick a stable order; unknown codes fall to the
// end via the buckets slice.
export const KnownCodes: string[] = Object.keys(WikiBatchErrorCodeToI18nKey)

// totalFromGroups sums every code bucket — what the "全部" tab badge
// renders. Returns 0 when the server returned no groups (e.g. zero
// failures for this job).
export function totalFromGroups(groups: WikiBatchFailureGroupCount[]): number {
  let acc = 0
  for (const g of groups) acc += g.count
  return acc
}

// normalizeFilter strips empty code + negative page + zero page_size
// before the network round-trip. The server already clamps these but
// normalizing here avoids a `?code=&page=0` URL that triggers
// unnecessary cache misses and a server-side 400 for `page=0`.
export function normalizeFilter(filter: {
  code?: string
  page?: number
  page_size?: number
}): { code?: string; page?: number; page_size?: number } {
  const out: { code?: string; page?: number; page_size?: number } = {}
  if (filter.code && filter.code.trim()) out.code = filter.code.trim()
  if (filter.page !== undefined && filter.page > 0) out.page = filter.page
  if (filter.page_size !== undefined && filter.page_size > 0) {
    out.page_size = filter.page_size
  }
  return out
}

// sortGroups keeps the known codes in `KnownCodes` order first, then
// appends any unknown bucket. Each group is stable across renders so
// the drawer doesn't shuffle on page change.
export function sortGroups(
  groups: WikiBatchFailureGroupCount[],
): WikiBatchFailureGroupCount[] {
  const known: WikiBatchFailureGroupCount[] = []
  const unknown: WikiBatchFailureGroupCount[] = []
  for (const g of groups) {
    if (KnownCodes.includes(g.code)) known.push(g)
    else unknown.push(g)
  }
  known.sort(
    (a, b) => KnownCodes.indexOf(a.code) - KnownCodes.indexOf(b.code),
  )
  unknown.sort((a, b) => (a.code < b.code ? -1 : a.code > b.code ? 1 : 0))
  return [...known, ...unknown]
}

// failureDrawerTitleTokens extracts `{jobId}` for the i18n interpolation
// in wiki.batchFailures.drawerTitle — the header renders the first 8
// chars of the UUID instead of the full id.
export function failureDrawerTitleTokens(jobId: string): { jobId: string } {
  if (!jobId) return { jobId: '' }
  return { jobId: jobId.slice(0, 8) }
}