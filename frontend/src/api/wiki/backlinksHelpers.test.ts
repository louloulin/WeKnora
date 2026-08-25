import assert from 'node:assert/strict'
import test from 'node:test'

import {
  backlinkCacheKey,
  backlinksVisibility,
  formatBacklinkTitle,
  groupBacklinksByPageType,
  liveOnly,
  normalizeBacklinks,
  sortBacklinks,
} from './backlinksHelpers.ts'
import type { Backlink } from './backlinksHelpers.ts'

const sample: Backlink[] = [
  {
    slug: 'summary/intro',
    title: 'Intro',
    pageType: 'summary',
    status: 'live',
    updatedAt: '2026-08-20T10:00:00Z',
  },
  {
    slug: 'entity/acme',
    title: 'Acme Corp',
    pageType: 'entity',
    status: 'live',
    updatedAt: '2026-08-22T10:00:00Z',
  },
  {
    slug: 'entity/globex',
    title: 'Globex',
    pageType: 'entity',
    status: 'archived',
    updatedAt: '2026-08-15T10:00:00Z',
  },
  {
    slug: 'concept/market',
    title: 'Market',
    pageType: 'concept',
    status: 'live',
    updatedAt: '2026-08-22T10:00:00Z',
  },
]

test('formatBacklinkTitle returns the title when present, falls back to slug', () => {
  assert.equal(formatBacklinkTitle(sample[0]), 'Intro')
  assert.equal(
    formatBacklinkTitle({
      slug: 'orphan/page',
      title: '   ',
      pageType: 'summary',
      status: 'live',
      updatedAt: '2026-08-22T10:00:00Z',
    }),
    'orphan/page',
  )
})

test('sortBacklinks orders by updatedAt desc with slug tiebreaker', () => {
  const sorted = sortBacklinks(sample)
  // 2026-08-22 tie → alphabetical slug order: concept < entity
  assert.equal(sorted[0].slug, 'concept/market')
  assert.equal(sorted[1].slug, 'entity/acme')
  assert.equal(sorted[2].slug, 'summary/intro') // 2026-08-20
  assert.equal(sorted[3].slug, 'entity/globex') // 2026-08-15
})

test('sortBacklinks does not mutate the input array', () => {
  const input = [...sample]
  const before = input.map((b) => b.slug).join(',')
  sortBacklinks(input)
  const after = input.map((b) => b.slug).join(',')
  assert.equal(before, after)
})

test('groupBacklinksByPageType preserves the desc order within each group', () => {
  const grouped = groupBacklinksByPageType(sample)
  assert.deepEqual(Object.keys(grouped).sort(), [
    'concept',
    'entity',
    'summary',
  ])
  assert.equal(grouped.entity?.length, 2)
  assert.equal(grouped.entity?.[0].slug, 'entity/acme')
  assert.equal(grouped.entity?.[1].slug, 'entity/globex')
})

test('liveOnly drops archived rows but keeps rows with missing status', () => {
  const filtered = liveOnly([
    ...sample,
    {
      slug: 'mystery',
      title: 'Mystery',
      pageType: 'unknown',
      status: '',
      updatedAt: '2026-08-22T10:00:00Z',
    },
  ])
  const slugs = filtered.map((b) => b.slug)
  assert.ok(!slugs.includes('entity/globex'))
  assert.ok(slugs.includes('mystery'))
})

test('normalizeBacklinks accepts snake_case raw payloads (HTTP shape)', () => {
  const normalized = normalizeBacklinks([
    { slug: 'a', title: 'A', page_type: 'summary', status: 'live', updated_at: '2026-08-22T00:00:00Z' },
    { slug: 'b' }, // missing fields -> defaults
    null,
    'not-an-object',
    { /* missing slug -> dropped */ title: 'X' },
  ])
  assert.equal(normalized.length, 2)
  assert.equal(normalized[0].slug, 'a')
  assert.equal(normalized[0].pageType, 'summary')
  assert.equal(normalized[1].slug, 'b')
  assert.equal(normalized[1].pageType, 'other')
  assert.equal(normalized[1].status, 'live')
})

test('normalizeBacklinks returns [] for non-array input', () => {
  assert.deepEqual(normalizeBacklinks(null), [])
  assert.deepEqual(normalizeBacklinks({}), [])
  assert.deepEqual(normalizeBacklinks('oops'), [])
})

test('backlinksVisibility maps undefined/empty/populated correctly', () => {
  assert.equal(backlinksVisibility(undefined), 'hidden')
  assert.equal(backlinksVisibility([]), 'empty')
  assert.equal(backlinksVisibility(sample), 'populated')
})

test('backlinkCacheKey is stable and unique per (kb, slug)', () => {
  assert.equal(backlinkCacheKey('kb1', 'page-a'), 'kb1\x00page-a')
  assert.notEqual(backlinkCacheKey('kb1', 'page-a'), backlinkCacheKey('kb1', 'page-b'))
  assert.notEqual(backlinkCacheKey('kb1', 'page-a'), backlinkCacheKey('kb2', 'page-a'))
})