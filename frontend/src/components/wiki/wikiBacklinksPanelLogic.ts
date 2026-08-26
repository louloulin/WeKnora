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

// Build #20 — graph-view helpers -------------------------------------

/** Section ids for the four collapsible panels under the header. */
export type GraphSectionId = 'direct' | 'indirect' | 'related' | 'broken'

const GRAPH_SECTION_LABELS: Record<GraphSectionId, string> = {
  direct: 'direct',
  indirect: 'indirect',
  related: 'related',
  broken: 'broken',
}

/** All section ids in display order. */
export const GRAPH_SECTION_IDS: GraphSectionId[] = [
  'direct',
  'indirect',
  'related',
  'broken',
]

/**
 * Resolve the per-section collapse state with sensible defaults.
 *   - missing entry → `direct:false, others:true` (direct open,
 *     the rest collapsed) so the panel reveals the strongest signal
 *     first without overwhelming the sidebar
 *   - present entry  → 1:1 returned so persisted choices survive
 */
export function normaliseCollapseState(
  state: Partial<Record<GraphSectionId, boolean>> | undefined | null,
): Record<GraphSectionId, boolean> {
  return {
    direct: state?.direct ?? false,
    indirect: state?.indirect ?? true,
    related: state?.related ?? true,
    broken: state?.broken ?? true,
  }
}

/** LocalStorage key for the panel's collapse state. Stable so user
 * preferences survive across reloads and KB switches. */
export const GRAPH_COLLAPSE_STORAGE_KEY = 'wikiBacklinksPanel:collapse'

/** Build the localStorage key for collapse state, accepting an
 * optional namespace prefix when multiple panels coexist. */
export function graphCollapseStorageKey(
  prefix = GRAPH_COLLAPSE_STORAGE_KEY,
): string {
  return prefix
}

/** Hydrate collapse state from `localStorage`. Returns the normalised
 * shape; consumers should spread into reactive state. */
export function readGraphCollapseState(
  storage: Pick<Storage, 'getItem'> | null | undefined,
): Record<GraphSectionId, boolean> {
  if (!storage) {
    return normaliseCollapseState(undefined)
  }
  let raw: unknown = null
  try {
    raw = JSON.parse(storage.getItem(GRAPH_COLLAPSE_STORAGE_KEY) || 'null')
  } catch {
    raw = null
  }
  if (!raw || typeof raw !== 'object') {
    return normaliseCollapseState(undefined)
  }
  return normaliseCollapseState(raw as Partial<Record<GraphSectionId, boolean>>)
}

/** Persist collapse state to `localStorage`. Failures (quota, private
 * mode) are swallowed so the panel keeps working without persistence. */
export function writeGraphCollapseState(
  storage: Pick<Storage, 'setItem'> | null | undefined,
  state: Record<GraphSectionId, boolean>,
): void {
  if (!storage) return
  try {
    storage.setItem(
      GRAPH_COLLAPSE_STORAGE_KEY,
      JSON.stringify({
        direct: state.direct,
        indirect: state.indirect,
        related: state.related,
        broken: state.broken,
      }),
    )
  } catch {
    // ignore — quota / private mode
  }
}

/** Build the per-section label suffix used by the i18n key
 * (`wiki.backlinksGraph.sections.<id>`). Pure mapping so the
 * template can iterate `GRAPH_SECTION_IDS` without hard-coding
 * keys. */
export function graphSectionLabelKey(id: GraphSectionId): string {
  return GRAPH_SECTION_LABELS[id]
}

/** Round a Jaccard score to 2 decimals for display. */
export function formatJaccard(score: number | undefined | null): string {
  if (typeof score !== 'number' || !Number.isFinite(score)) return ''
  // Use Math.round * 100 / 100 to avoid toFixed() quirks across runtimes.
  const rounded = Math.round(score * 100) / 100
  return `+${rounded}`
}

/** Build a stable localStorage-safe slug display. Slug may contain
 * `/` (Build #11 hierarchical slugs); we keep the rendering literal
 * so the user can match it to the source markdown. */
export function displayVia(via: string | undefined | null): string {
  if (!via) return ''
  return via
}