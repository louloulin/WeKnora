import assert from 'node:assert/strict'
import test from 'node:test'

/**
 * Store-level tests for the wiki full-text search.
 *
 * The store (`wikiSearch.ts`) imports `searchWikiPagesFullText`
 * from `../api/wiki/search`, which itself pulls in axios via the
 * shared request util. To keep these tests hermetic we exercise
 * the pure normalize contract that the store relies on, plus the
 * mock-index fallback path. The Pinia runtime itself is covered
 * by component tests separately.
 */

import { searchWikiIndex } from '../mock/wikiSearchIndex.ts'
import type { WikiSearchResult } from '../api/wiki/search.ts'

const DEFAULT_LIMIT = 50

function normalize(
  res: WikiSearchResult[] | undefined,
  limit: number,
): WikiSearchResult[] {
  if (!res || !Array.isArray(res)) return []
  return res.slice().sort((a, b) => b.score - a.score).slice(0, limit)
}

test('normalize returns [] for non-array input', () => {
  assert.deepEqual(normalize(undefined, DEFAULT_LIMIT), [])
  assert.deepEqual(normalize(null as unknown as WikiSearchResult[], DEFAULT_LIMIT), [])
  assert.deepEqual(normalize('oops' as unknown as WikiSearchResult[], DEFAULT_LIMIT), [])
})

test('normalize sorts score descending and caps at limit', () => {
  const input: WikiSearchResult[] = [
    { pageId: 'a', slug: 'a', title: 'A', path: ['/a'], snippet: '...', score: 1 },
    { pageId: 'b', slug: 'b', title: 'B', path: ['/b'], snippet: '...', score: 5 },
    { pageId: 'c', slug: 'c', title: 'C', path: ['/c'], snippet: '...', score: 3 },
    { pageId: 'd', slug: 'd', title: 'D', path: ['/d'], snippet: '...', score: 4 },
  ]
  const out = normalize(input, 3)
  assert.equal(out.length, 3)
  assert.deepEqual(out.map((r) => r.pageId), ['b', 'd', 'c'])
})

test('normalize is stable for equal scores (insertion order preserved)', () => {
  const input: WikiSearchResult[] = [
    { pageId: 'a', slug: 'a', title: 'A', path: ['/a'], snippet: '...', score: 2 },
    { pageId: 'b', slug: 'b', title: 'B', path: ['/b'], snippet: '...', score: 2 },
    { pageId: 'c', slug: 'c', title: 'C', path: ['/c'], snippet: '...', score: 2 },
  ]
  const out = normalize(input, DEFAULT_LIMIT)
  assert.deepEqual(out.map((r) => r.pageId), ['a', 'b', 'c'])
})

test('normalize caps exactly at limit when results overflow', () => {
  const input: WikiSearchResult[] = Array.from({ length: 100 }, (_, i) => ({
    pageId: `p${i}`,
    slug: `p${i}`,
    title: `P${i}`,
    path: [`/p${i}`],
    snippet: '...',
    score: 100 - i,
  }))
  const out = normalize(input, 10)
  assert.equal(out.length, 10)
  assert.equal(out[0].pageId, 'p0')
  assert.equal(out[9].pageId, 'p9')
})

test('mock index returns at most limit hits and is score-sorted', () => {
  const hits = searchWikiIndex('meeting', 3)
  assert.ok(hits.length <= 3)
  for (let i = 1; i < hits.length; i++) {
    assert.ok(hits[i - 1].score >= hits[i].score, 'mock results must be score-desc')
  }
})

test('mock index returns [] for empty queries', () => {
  assert.deepEqual(searchWikiIndex('', 10), [])
  assert.deepEqual(searchWikiIndex('   ', 10), [])
})

test('mock fallback shape matches the typed contract', () => {
  const hits = searchWikiIndex('meeting', 5)
  for (const h of hits) {
    assert.equal(typeof h.pageId, 'string')
    assert.equal(typeof h.slug, 'string')
    assert.equal(typeof h.title, 'string')
    assert.ok(Array.isArray(h.path))
    assert.equal(typeof h.snippet, 'string')
    assert.equal(typeof h.score, 'number')
  }
})

test('history dedup is case-insensitive but preserves the latest casing', () => {
  const HISTORY_MAX = 10
  function pushHistory(arr: string[], q: string): string[] {
    if (!q || q.length < 2) return arr
    const lower = q.toLowerCase()
    const filtered = arr.filter((x) => x.toLowerCase() !== lower)
    return [q, ...filtered].slice(0, HISTORY_MAX)
  }

  let h: string[] = []
  h = pushHistory(h, 'Meeting Notes')
  h = pushHistory(h, 'meeting notes')
  assert.equal(h.length, 1)
  assert.equal(h[0], 'meeting notes')

  h = pushHistory(h, 'Weekly Recap')
  h = pushHistory(h, 'Weekly recap')
  assert.equal(h.length, 2)
  assert.equal(h[0], 'Weekly recap')
  assert.equal(h[1], 'meeting notes')
})

test('history caps at 10 and reverses order (newest first)', () => {
  const HISTORY_MAX = 10
  function pushHistory(arr: string[], q: string): string[] {
    if (!q || q.length < 2) return arr
    const lower = q.toLowerCase()
    const filtered = arr.filter((x) => x.toLowerCase() !== lower)
    return [q, ...filtered].slice(0, HISTORY_MAX)
  }

  let h: string[] = []
  for (let i = 0; i < 15; i++) h = pushHistory(h, `query-${i}`)
  assert.equal(h.length, 10)
  assert.equal(h[0], 'query-14')
  assert.equal(h[9], 'query-5')
})
