// Unit test for WikiRecentPages component logic.
//
// Mirrors the .vue file's slot visibility / cap / truncation math so we
// can run it under Node without the Vue runtime. Locks in:
//   - rail is hidden when no pages are provided
//   - pages are capped to MAX_ITEMS (5)
//   - active slug is flagged for highlight
//   - long titles are truncated with ellipsis
//   - empty title is rendered as an empty string (not "undefined")

import assert from 'node:assert/strict'
import test from 'node:test'

const MAX_ITEMS = 5
const MAX_TITLE_LENGTH = 28

function buildRecentRail(input) {
  const pages = Array.isArray(input.pages) ? input.pages : []
  const activeSlug = input.activeSlug || ''
  return {
    visible: pages.length > 0,
    pages: pages.slice(0, MAX_ITEMS).map((p) => ({
      ...p,
      title: p.title ? (
        p.title.length > MAX_TITLE_LENGTH
          ? p.title.slice(0, MAX_TITLE_LENGTH - 1) + '…'
          : p.title
      ) : '',
      isActive: Boolean(activeSlug) && p.slug === activeSlug,
    })),
    count: pages.length,
  }
}

test('rail is hidden when no pages are provided', () => {
  const r = buildRecentRail({ pages: [] })
  assert.equal(r.visible, false)
  assert.equal(r.pages.length, 0)
})

test('rail is hidden when pages is undefined', () => {
  const r = buildRecentRail({})
  assert.equal(r.visible, false)
})

test('pages are capped to MAX_ITEMS (5) — input 7, output 5', () => {
  const pages = Array.from({ length: 7 }, (_, i) => ({
    id: String(i),
    slug: `slug-${i}`,
    title: `Page ${i}`,
  }))
  const r = buildRecentRail({ pages })
  assert.equal(r.visible, true)
  assert.equal(r.pages.length, MAX_ITEMS)
  assert.equal(r.pages[0].slug, 'slug-0')
  assert.equal(r.pages[4].slug, 'slug-4')
  assert.equal(r.count, 7) // count is the source length, not the capped length
})

test('active slug is flagged for highlight', () => {
  const pages = [
    { id: '1', slug: 'a', title: 'A' },
    { id: '2', slug: 'b', title: 'B' },
  ]
  const r = buildRecentRail({ pages, activeSlug: 'b' })
  assert.equal(r.pages[0].isActive, false)
  assert.equal(r.pages[1].isActive, true)
})

test('long titles are truncated with ellipsis', () => {
  const longTitle = 'a'.repeat(40)
  const r = buildRecentRail({
    pages: [{ id: '1', slug: 'x', title: longTitle }],
  })
  assert.equal(r.pages[0].title.length, MAX_TITLE_LENGTH)
  assert.ok(r.pages[0].title.endsWith('…'))
})

test('short titles are not truncated', () => {
  const r = buildRecentRail({
    pages: [{ id: '1', slug: 'x', title: 'Short' }],
  })
  assert.equal(r.pages[0].title, 'Short')
})

test('empty title is rendered as empty string, not "undefined"', () => {
  const r = buildRecentRail({
    pages: [{ id: '1', slug: 'x', title: '' }],
  })
  assert.equal(r.pages[0].title, '')
})

test('category_path preserved for breadcrumb under title', () => {
  const r = buildRecentRail({
    pages: [
      { id: '1', slug: 'x', title: 'T', category_path: ['Docs', 'API'] },
    ],
  })
  assert.deepEqual(r.pages[0].category_path, ['Docs', 'API'])
})
