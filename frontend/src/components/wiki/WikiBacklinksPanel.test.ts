import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  GRAPH_COLLAPSE_STORAGE_KEY,
  GRAPH_SECTION_IDS,
  backlinksBodyId,
  backlinksCountLabel,
  displayVia,
  emptyStateHint,
  formatBacklinkTimestamp,
  formatJaccard,
  graphCollapseStorageKey,
  graphSectionLabelKey,
  normaliseCollapseState,
  readGraphCollapseState,
  writeGraphCollapseState,
} from './wikiBacklinksPanelLogic.ts'
import type { Backlink } from '../../api/wiki/backlinksHelpers'

const fixture: Backlink[] = [
  {
    slug: 'summary/intro',
    title: 'Intro',
    pageType: 'summary',
    status: 'live',
    updatedAt: '2026-08-22T10:00:00Z',
  },
  {
    slug: 'entity/acme',
    title: 'Acme',
    pageType: 'entity',
    status: 'live',
    updatedAt: '2026-08-15T10:00:00Z',
  },
]

test('backlinksCountLabel returns "" when list is undefined (hidden)', () => {
  assert.equal(backlinksCountLabel(undefined), '')
})

test('backlinksCountLabel returns "(0)" for empty list (empty state)', () => {
  assert.equal(backlinksCountLabel([]), '(0)')
})

test('backlinksCountLabel returns "(N)" for populated list', () => {
  assert.equal(backlinksCountLabel(fixture), '(2)')
})

test('formatBacklinkTimestamp parses RFC3339 and returns short date', () => {
  const out = formatBacklinkTimestamp('2026-08-22T10:00:00Z')
  assert.ok(out.length > 0)
  assert.ok(out.includes('2026'))
  assert.ok(/Aug/.test(out))
})

test('formatBacklinkTimestamp returns "" for invalid input', () => {
  assert.equal(formatBacklinkTimestamp(''), '')
  assert.equal(formatBacklinkTimestamp('not-a-date'), '')
})

test('emptyStateHint returns the slug when present', () => {
  assert.equal(emptyStateHint('summary/intro'), 'summary/intro')
})

test('emptyStateHint returns <slug> placeholder when slug is empty', () => {
  assert.equal(emptyStateHint(''), '<slug>')
  assert.equal(emptyStateHint('   '), '<slug>')
})

test('backlinksBodyId sanitizes non-identifier characters', () => {
  assert.equal(
    backlinksBodyId('kb-1', 'summary/intro'),
    'wiki-backlinks-body-kb-1-summary_intro',
  )
  assert.equal(
    backlinksBodyId('kb 1', 'foo bar'),
    'wiki-backlinks-body-kb_1-foo_bar',
  )
})

test('backlinksBodyId is stable per (kb, slug)', () => {
  assert.equal(
    backlinksBodyId('kb1', 'page-a'),
    backlinksBodyId('kb1', 'page-a'),
  )
  assert.notEqual(
    backlinksBodyId('kb1', 'page-a'),
    backlinksBodyId('kb1', 'page-b'),
  )
})

// -------------------------------------------------------------------
// Build #20 — graph-view helpers
// -------------------------------------------------------------------

test('GRAPH_SECTION_IDS exposes all four ids in display order', () => {
  assert.deepEqual(GRAPH_SECTION_IDS, [
    'direct',
    'indirect',
    'related',
    'broken',
  ])
})

test('normaliseCollapseState(undefined) defaults to direct open, others closed', () => {
  assert.deepEqual(normaliseCollapseState(undefined), {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('normaliseCollapseState(null) behaves like undefined', () => {
  assert.deepEqual(normaliseCollapseState(null), {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('normaliseCollapseState(partial) fills missing keys with defaults', () => {
  assert.deepEqual(normaliseCollapseState({ direct: true }), {
    direct: true,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('normaliseCollapseState(full) returns 1:1', () => {
  assert.deepEqual(
    normaliseCollapseState({
      direct: false,
      indirect: false,
      related: true,
      broken: false,
    }),
    {
      direct: false,
      indirect: false,
      related: true,
      broken: false,
    },
  )
})

test('graphCollapseStorageKey() returns the stable constant', () => {
  assert.equal(graphCollapseStorageKey(), GRAPH_COLLAPSE_STORAGE_KEY)
  assert.equal(
    graphCollapseStorageKey(),
    'wikiBacklinksPanel:collapse',
  )
})

class MemoryStorage {
  private store = new Map<string, string>()
  getItem(k: string): string | null {
    return this.store.get(k) ?? null
  }
  setItem(k: string, v: string): void {
    this.store.set(k, v)
  }
  removeItem(k: string): void {
    this.store.delete(k)
  }
  clear(): void {
    this.store.clear()
  }
  key(): string | null {
    return null
  }
  get length(): number {
    return this.store.size
  }
}

test('readGraphCollapseState(null) returns defaults', () => {
  assert.deepEqual(readGraphCollapseState(null), {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('readGraphCollapseState(empty storage) returns defaults', () => {
  const storage = new MemoryStorage()
  assert.deepEqual(readGraphCollapseState(storage), {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('readGraphCollapseState(parses stored JSON)', () => {
  const storage = new MemoryStorage()
  storage.setItem(
    GRAPH_COLLAPSE_STORAGE_KEY,
    JSON.stringify({
      direct: true,
      indirect: false,
      related: true,
      broken: false,
    }),
  )
  assert.deepEqual(readGraphCollapseState(storage), {
    direct: true,
    indirect: false,
    related: true,
    broken: false,
  })
})

test('readGraphCollapseState(tolerates malformed JSON)', () => {
  const storage = new MemoryStorage()
  storage.setItem(GRAPH_COLLAPSE_STORAGE_KEY, '{not valid json')
  assert.deepEqual(readGraphCollapseState(storage), {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('writeGraphCollapseState(null) is a no-op', () => {
  // Must not throw.
  writeGraphCollapseState(null, {
    direct: false,
    indirect: true,
    related: true,
    broken: true,
  })
})

test('writeGraphCollapseState writes JSON to the documented key', () => {
  const storage = new MemoryStorage()
  writeGraphCollapseState(storage, {
    direct: true,
    indirect: false,
    related: true,
    broken: false,
  })
  const raw = storage.getItem(GRAPH_COLLAPSE_STORAGE_KEY)
  assert.ok(raw)
  const parsed = JSON.parse(raw as string)
  assert.deepEqual(parsed, {
    direct: true,
    indirect: false,
    related: true,
    broken: false,
  })
})

test('formatJaccard rounds to 2 decimals with leading +', () => {
  assert.equal(formatJaccard(0.78), '+0.78')
  assert.equal(formatJaccard(0.333), '+0.33')
  assert.equal(formatJaccard(0.5), '+0.5')
  assert.equal(formatJaccard(1), '+1')
  assert.equal(formatJaccard(0), '+0')
})

test('formatJaccard returns "" for non-finite / missing input', () => {
  assert.equal(formatJaccard(undefined), '')
  assert.equal(formatJaccard(null), '')
  assert.equal(formatJaccard(Number.NaN), '')
  assert.equal(formatJaccard(Number.POSITIVE_INFINITY), '')
})

test('displayVia returns the slug verbatim or "" for falsy', () => {
  assert.equal(displayVia('summary/foo'), 'summary/foo')
  assert.equal(displayVia('entity/acme'), 'entity/acme')
  assert.equal(displayVia(''), '')
  assert.equal(displayVia(undefined), '')
  assert.equal(displayVia(null), '')
})

test('graphSectionLabelKey maps ids to i18n suffix', () => {
  assert.equal(graphSectionLabelKey('direct'), 'direct')
  assert.equal(graphSectionLabelKey('indirect'), 'indirect')
  assert.equal(graphSectionLabelKey('related'), 'related')
  assert.equal(graphSectionLabelKey('broken'), 'broken')
})

// -------------------------------------------------------------------
// Build #20 — template + api + store + i18n wiring (file-level checks)
// -------------------------------------------------------------------

const panel = readFileSync(
  new URL('./WikiBacklinksPanel.vue', import.meta.url),
  'utf8',
)
const logic = readFileSync(
  new URL('./wikiBacklinksPanelLogic.ts', import.meta.url),
  'utf8',
)
const apiIndex = readFileSync(
  new URL('../../api/wiki/index.ts', import.meta.url),
  'utf8',
)
const store = readFileSync(
  new URL('../../stores/wikiBacklinks.ts', import.meta.url),
  'utf8',
)
const locales = [
  readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/en-US.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/ko-KR.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/ru-RU.ts', import.meta.url), 'utf8'),
]

// Panel renders four collapsible sections keyed by GRAPH_SECTION_IDS,
// each one toggling collapse state.
test('panel renders the four graph sections + per-section collapse toggle', () => {
  assert.match(panel, /sectionIds\s*=\s*GRAPH_SECTION_IDS/)
  assert.match(panel, /v-for="sid in sectionIds"/)
  assert.match(panel, /toggleSection\(sid\)/)
  assert.match(panel, /v-show="!collapse\[sid\]"/)
})

// Each section header must show the localised label + the section
// count from `stats`.
test('panel surfaces direct/indirect/related/broken counts from stats', () => {
  assert.match(panel, /stats\.direct_count/)
  assert.match(panel, /stats\.indirect_count/)
  assert.match(panel, /stats\.related_count/)
  assert.match(panel, /stats\.broken_count/)
  assert.match(panel, /countFor\(sid\)/)
})

// D5 — clicking an indirect row must navigate to `via` (the 1-hop slug)
// so the user actually lands on the page that mentioned the target,
// not on the 2-hop target itself.
test('indirect row click targets `via`, not the 2-hop slug (D5)', () => {
  assert.match(panel, /onItemClick\(row\.via\)/)
  // The 2-hop slug is shown in the title via formatBacklinkTitle(row),
  // and the via slug is rendered as the subtitle via $t('...via', { slug: row.via }).
  assert.match(panel, /row\.via/)
  assert.match(panel, /t\('wiki\.backlinksGraph\.via'/)
})

// Direct + Related rows click the page slug, Broken rows are read-only.
test('direct + related rows navigate by slug; broken rows have no click handler', () => {
  assert.match(panel, /onItemClick\(b\.slug\)/)
  assert.match(panel, /onItemClick\(row\.slug\)/)
  // Broken list class is set, but no @click on the broken li.
  assert.match(panel, /wiki-backlinks-panel__item--broken/)
  // The broken row only shows [[slug]] + hint, no onItemClick call.
  assert.doesNotMatch(
    panel,
    /wiki-backlinks-panel__item--broken[\s\S]{0,200}@click="onItemClick/,
  )
})

// Footer "View full graph →" emits `view-full-graph` so the host
// (WikiBrowser) can drive loadEgoGraph(slug).
test('panel emits view-full-graph from the footer button', () => {
  assert.match(panel, /\(e: 'view-full-graph', slug: string\)/)
  assert.match(panel, /onViewFullGraph\(\)/)
  assert.match(panel, /emit\('view-full-graph', props\.slug\)/)
  assert.match(panel, /t\('wiki\.backlinksGraph\.viewFullGraph'\)/)
})

// Load-failed fallback: if the graph request errors, the panel must
// show the Build #11 flat list with a localised toast so users never
// see a blank panel.
test('panel falls back to Build #11 flat list when graph load fails', () => {
  assert.match(panel, /v-if="graphError && !graph"/)
  assert.match(panel, /t\('wiki\.backlinksGraph\.loadFailedToast'\)/)
  // The fallback path keeps using store.backlinksFor + hasCache, not
  // the graph payload.
  assert.match(panel, /store\.backlinksFor\(props\.kbId, props\.slug\)/)
  assert.match(panel, /hasCache\.value/)
})

// Both `loadBacklinkGraph` (graph) and `loadBacklinks` (Build #11 fallback)
// fire when the slug changes — keeps the fallback warm.
test('panel refreshes both graph and flat list on slug change', () => {
  assert.match(panel, /loadBacklinkGraph\(kb, slug\)/)
  assert.match(panel, /loadBacklinks\(kb, slug\)/)
  assert.match(panel, /watch\(\s*\(\)\s*=>\s*\[props\.kbId, props\.slug\]/)
})

// localStorage persistence is wired through the readGraphCollapseState
// / writeGraphCollapseState helpers (not raw getItem/setItem calls).
test('panel persists collapse state through the logic helpers', () => {
  assert.match(panel, /loadCollapse\(\)/)
  assert.match(panel, /readGraphCollapseState\(window\.localStorage\)/)
  assert.match(panel, /writeGraphCollapseState\(window\.localStorage/)
  assert.match(panel, /saveCollapse\(\)/)
  // no raw localStorage.getItem('wikiBacklinksPanel:collapse')
  assert.doesNotMatch(
    panel,
    /localStorage\.getItem\(\s*['"]wikiBacklinksPanel:collapse['"]/,
  )
})

// Logic module exports the storage key + read/write helpers so the
// component template doesn't need its own implementation.
test('logic module exposes the storage key + read/write helpers', () => {
  assert.match(logic, /export const GRAPH_COLLAPSE_STORAGE_KEY/)
  assert.match(logic, /export function readGraphCollapseState/)
  assert.match(logic, /export function writeGraphCollapseState/)
  assert.match(logic, /export function formatJaccard/)
  assert.match(logic, /export function normaliseCollapseState/)
  assert.match(logic, /export const GRAPH_SECTION_IDS/)
})

// API client must expose getWikiBacklinkGraph and route through the
// shared encodeSlugPath helper.
test('api/index.ts exposes getWikiBacklinkGraph with all 3 query params', () => {
  assert.match(apiIndex, /export\s+function getWikiBacklinkGraph/)
  assert.match(apiIndex, /backlinks\/graph/)
  assert.match(apiIndex, /max_indirect/)
  assert.match(apiIndex, /max_related/)
  assert.match(apiIndex, /jaccard/)
})

// Pinia store has a separate graph cache layer so the Build #11 flat
// list cache stays untouched (zero regression).
test('store exposes graphFor / loadBacklinkGraph / isGraphLoading / graphErrorFor', () => {
  assert.match(store, /graphByKey/)
  assert.match(store, /graphLoadingByKey/)
  assert.match(store, /graphErrorByKey/)
  assert.match(store, /function graphFor/)
  assert.match(store, /function isGraphLoading/)
  assert.match(store, /function graphErrorFor/)
  assert.match(store, /function loadBacklinkGraph/)
  assert.match(store, /function invalidateGraph/)
  // Build #11 cache must still be present — no regression.
  assert.match(store, /byKey/)
  assert.match(store, /function backlinksFor/)
})

// All four locales must carry the `backlinksGraph` block with the
// required keys.
test('all four locales carry the backlinksGraph block with required keys', () => {
  const requiredKeys = [
    'direct',
    'indirect',
    'related',
    'broken',
    'viewFullGraph',
    'loadFailedToast',
    'brokenHint',
  ]
  for (const locale of locales) {
    assert.match(
      locale,
      /backlinksGraph:\s*{/,
      'backlinksGraph block missing',
    )
    assert.match(
      locale,
      /sections:\s*{/,
      'sections sub-block missing',
    )
    for (const key of requiredKeys) {
      assert.match(
        locale,
        new RegExp(`${key}:`),
        `${key} missing in backlinksGraph block`,
      )
    }
    // via + jaccard must be template strings so the {slug} / {n} interpolations work.
    assert.match(locale, /via:\s*['`][^'"`]*\{slug\}[^'"`]*['`]/)
    assert.match(locale, /jaccard:\s*['`][^'"`]*\{n\}[^'"`]*['`]/)
  }
})