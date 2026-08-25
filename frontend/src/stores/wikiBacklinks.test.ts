import assert from 'node:assert/strict'
import test from 'node:test'

/**
 * Store tests for the wiki backlinks panel.
 *
 * We import only from the helper module (no axios / vue-i18n
 * chain) and exercise the public reducer logic with hand-rolled
 * fakes. Pinia runtime semantics are covered by the component
 * test in `WikiBacklinksPanel.test.ts`.
 */

import {
  backlinkCacheKey,
  liveOnly,
  normalizeBacklinks,
  sortBacklinks,
} from '../api/wiki/backlinksHelpers.ts'

// The store itself imports `getWikiPageBacklinks` from
// `../api/wiki/index.ts`, which transitively pulls in axios. To
// keep Node tests fast and isolated we test the public reducer
// logic directly here. The Pinia runtime is exercised in the
// component test below.

test('backlinkCacheKey is stable per (kbId, slug)', () => {
  const k1 = backlinkCacheKey('kb1', 'page-a')
  const k2 = backlinkCacheKey('kb1', 'page-a')
  const k3 = backlinkCacheKey('kb1', 'page-b')
  const k4 = backlinkCacheKey('kb2', 'page-a')
  assert.equal(k1, k2)
  assert.notEqual(k1, k3)
  assert.notEqual(k1, k4)
})

test('normalizeBacklinks → sortBacklinks → liveOnly pipeline is stable', () => {
  const raw = [
    { slug: 'a', title: 'A', page_type: 'summary', status: 'live', updated_at: '2026-08-22T00:00:00Z' },
    { slug: 'b', title: 'B', page_type: 'entity', status: 'archived', updated_at: '2026-08-21T00:00:00Z' },
    { slug: 'c', title: 'C', page_type: 'concept', status: 'live', updated_at: '2026-08-22T00:00:00Z' },
  ]
  const normalized = normalizeBacklinks(raw)
  const sorted = sortBacklinks(normalized)
  // a and c tie on date; 'a' < 'c' alphabetically → a first.
  assert.deepEqual(
    sorted.map((b) => b.slug),
    ['a', 'c', 'b'],
  )
  const live = liveOnly(sorted)
  assert.deepEqual(
    live.map((b) => b.slug),
    ['a', 'c'],
  )
})

test('normalizeBacklinks returns [] for non-array input', () => {
  assert.deepEqual(normalizeBacklinks(undefined), [])
  assert.deepEqual(normalizeBacklinks(null), [])
  assert.deepEqual(normalizeBacklinks('oops'), [])
  assert.deepEqual(normalizeBacklinks({ not: 'array' }), [])
})

test('normalizeBacklinks tolerates mixed / missing fields', () => {
  const result = normalizeBacklinks([
    { slug: 'ok', title: 'OK', page_type: 'summary', status: 'live', updated_at: '2026-08-22T00:00:00Z' },
    { slug: 'minimal' },
    { title: 'no-slug' },
    null,
    undefined,
    'string',
  ])
  // Only `ok` and `minimal` survive; `no-slug` is dropped (no slug),
  // null/undefined/string are skipped.
  assert.equal(result.length, 2)
  assert.equal(result[0].slug, 'ok')
  assert.equal(result[1].slug, 'minimal')
  assert.equal(result[1].pageType, 'other')
  assert.equal(result[1].status, 'live')
  assert.equal(result[1].updatedAt, '1970-01-01T00:00:00.000Z')
})

test('normalizeBacklinks writes sorted output (most recent first)', () => {
  const result = normalizeBacklinks([
    { slug: 'old', title: 'Old', page_type: 'entity', status: 'live', updated_at: '2026-01-01T00:00:00Z' },
    { slug: 'new', title: 'New', page_type: 'entity', status: 'live', updated_at: '2026-08-22T00:00:00Z' },
  ])
  assert.equal(result[0].slug, 'new')
  assert.equal(result[1].slug, 'old')
})