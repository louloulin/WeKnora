import assert from 'node:assert/strict'
import test from 'node:test'

import {
  searchWikiIndex,
  WIKI_SEARCH_MOCK_PAGES,
} from './wikiSearchIndex.ts'

test('mock index has 20 entries by design', () => {
  assert.equal(WIKI_SEARCH_MOCK_PAGES.length, 20)
})

test('searchWikiIndex returns nothing for empty queries', () => {
  assert.equal(searchWikiIndex('', 50).length, 0)
  assert.equal(searchWikiIndex('   ', 50).length, 0)
})

test('searchWikiIndex finds a single keyword hit', () => {
  const hits = searchWikiIndex('meeting', 50)
  assert.ok(hits.length >= 3)
  for (const h of hits) {
    assert.ok(
      h.title.toLowerCase().includes('meeting') ||
        h.content.toLowerCase().includes('meeting'),
    )
  }
})

test('searchWikiIndex applies AND across multiple keywords', () => {
  const hits = searchWikiIndex('weekly meeting', 50)
  assert.ok(hits.length >= 1)
  for (const h of hits) {
    const blob = (h.title + ' ' + h.content).toLowerCase()
    assert.ok(blob.includes('weekly'))
    assert.ok(blob.includes('meeting'))
  }
})

test('searchWikiIndex ranks title hits above body hits', () => {
  const hits = searchWikiIndex('quarterly', 50)
  assert.ok(hits.length >= 1)
  const first = hits[0]
  assert.ok(first.title.toLowerCase().includes('quarterly'))
})

test('searchWikiIndex limits the result count', () => {
  const hits = searchWikiIndex('wiki', 2)
  assert.ok(hits.length <= 2)
})

test('searchWikiIndex returns an empty array for a query with no hits', () => {
  const hits = searchWikiIndex('xyzzyqwerty', 50)
  assert.equal(hits.length, 0)
})

test('searchWikiIndex sets the score field on every hit', () => {
  const hits = searchWikiIndex('wiki', 50)
  for (const h of hits) {
    assert.ok(typeof h.score === 'number')
    assert.ok(h.score > 0)
  }
})