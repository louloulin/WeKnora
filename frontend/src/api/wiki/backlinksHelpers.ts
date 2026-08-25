import type { NormalizedBacklink } from './backlinksTypes'

/**
 * Pure helpers for the wiki backlinks panel (Build #11).
 *
 * Extracted to a sibling `.ts` file so they can be unit-tested
 * in Node via `tsx --test` without loading `utils/request.ts`
 * (which transitively pulls in `vue-i18n`). Same pattern as
 * `wikiPageAclConflict.ts` / `wikiAclDialogLogic.ts`.
 */

export type Backlink = NormalizedBacklink

/**
 * Returns the display title for a backlink row, falling back to
 * the raw slug when the server hasn't resolved a title yet (e.g.
 * a race where the source page was just deleted between
 * `GetBySlug` and `ListBySlugs`).
 */
export function formatBacklinkTitle(backlink: Backlink): string {
  const title = backlink.title?.trim()
  if (title) return title
  return backlink.slug
}

/**
 * Stable descending sort by `updatedAt`. Falls back to slug
 * order for equal timestamps so the panel order is deterministic
 * across renders (helps the eye anchor while reading).
 */
export function sortBacklinks(list: Backlink[]): Backlink[] {
  return [...list].sort((a, b) => {
    const at = Date.parse(a.updatedAt) || 0
    const bt = Date.parse(b.updatedAt) || 0
    if (bt !== at) return bt - at
    return a.slug.localeCompare(b.slug)
  })
}

/**
 * Group backlinks by their `pageType` while preserving the
 * within-group ordering from `sortBacklinks`. Used when the
 * panel renders sections per page type.
 */
export function groupBacklinksByPageType(
  list: Backlink[],
): Record<string, Backlink[]> {
  const out: Record<string, Backlink[]> = {}
  for (const b of sortBacklinks(list)) {
    const key = b.pageType || 'other'
    if (!out[key]) out[key] = []
    out[key].push(b)
  }
  return out
}

/**
 * Stable cache key. Single source of truth — used by both the
 * store and the helpers so revalidation paths can't disagree
 * on what's "the same row".
 */
export function backlinkCacheKey(kbId: string, slug: string): string {
  return `${kbId}\x00${slug}`
}

/**
 * Filter out backlinks whose status is non-live. The server
 * already drops orphans (rows pointing at deleted pages), but
 * archived / draft pages may still appear — the panel should
 * show only live links for the reader.
 */
export function liveOnly(list: Backlink[]): Backlink[] {
  return list.filter((b) => !b.status || b.status === 'live')
}

/**
 * Coerce an arbitrary server payload (snake_case `WikiPageBacklink`
 * or anything similar) into a normalized `Backlink[]`. Defensive:
 * the API may evolve and callers should not crash on a missing
 * field.
 */
export function normalizeBacklinks(raw: unknown): Backlink[] {
  if (!Array.isArray(raw)) return []
  const out: Backlink[] = []
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue
    const r = item as Record<string, unknown>
    const slug =
      typeof r.slug === 'string'
        ? r.slug
        : typeof r.Slug === 'string'
          ? r.Slug
          : ''
    if (!slug) continue
    const pageType =
      typeof r.pageType === 'string'
        ? r.pageType
        : typeof r.page_type === 'string'
          ? r.page_type
          : 'other'
    const updatedAt =
      typeof r.updatedAt === 'string'
        ? r.updatedAt
        : typeof r.updated_at === 'string'
          ? r.updated_at
          : new Date(0).toISOString()
    out.push({
      slug,
      title: typeof r.title === 'string' ? r.title : '',
      pageType,
      status: typeof r.status === 'string' ? r.status : 'live',
      updatedAt,
    })
  }
  return sortBacklinks(out)
}

/**
 * Resolve whether the panel should display a section given a
 * KB / slug pair. Pure helper so callers don't have to reason
 * about empty / loading / error states inline.
 */
export type BacklinksVisibility =
  | 'hidden' // no slug to query
  | 'empty' // loaded but list is empty
  | 'populated' // loaded with at least one row

export function backlinksVisibility(
  list: Backlink[] | undefined,
): BacklinksVisibility {
  if (!list) return 'hidden'
  if (list.length === 0) return 'empty'
  return 'populated'
}