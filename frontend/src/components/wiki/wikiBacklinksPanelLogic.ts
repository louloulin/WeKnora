/**
 * Pure helpers extracted from `WikiBacklinksPanel.vue` so the
 * component's visual logic can be tested in Node via `tsx --test`
 * without mounting Vue. Mirrors the pattern in
 * `wikiAclDialogLogic.ts` (Build #10).
 */

import type { Backlink } from '../../api/wiki/backlinksHelpers'

/**
 * Resolve the panel header count label:
 *   - hidden (no slug / no cache)         → ''  (no badge)
 *   - empty  (cache present but [])       → '(0)'
 *   - populated                           → '(N)'
 *
 * Returning a single string keeps the template a one-liner
 * and makes the rule unit-testable.
 */
export function backlinksCountLabel(
  list: Backlink[] | undefined,
): string {
  if (!list) return ''
  return `(${list.length})`
}

/**
 * Format an ISO timestamp as a short date for the panel row.
 * Returns '' for invalid / empty input so the template can
 * hide the field without a v-if.
 */
export function formatBacklinkTimestamp(iso: string): string {
  const ts = Date.parse(iso)
  if (!ts) return ''
  // The component passes an explicit locale when constructing
  // Intl.DateTimeFormat; we keep this function locale-agnostic
  // and let the caller wrap it. Default to 'en-US' for tests
  // — the production component overrides with the user's locale.
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(new Date(ts))
}

/**
 * Compute the hint slug to show in the empty state. If the
 * current page's slug is empty / unknown we show a literal
 * `<slug>` placeholder so the user knows what to type.
 */
export function emptyStateHint(currentSlug: string): string {
  const trimmed = currentSlug.trim()
  if (!trimmed) return '<slug>'
  return trimmed
}

/**
 * Build the body element id used for `aria-controls` linkage.
 * Pure function so the same value can be asserted in tests
 * without depending on Vue's id allocation.
 */
export function backlinksBodyId(kbId: string, slug: string): string {
  const safe = (s: string) => s.replace(/[^a-zA-Z0-9-]/g, '_')
  return `wiki-backlinks-body-${safe(kbId)}-${safe(slug)}`
}