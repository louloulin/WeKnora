import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildSnippet,
  highlight,
  scoreMatch,
  splitKeywords,
} from './wikiSearch.ts'

test('splitKeywords lowercases and drops empty tokens', () => {
  assert.deepEqual(splitKeywords('Weekly MEETING  '), ['weekly', 'meeting'])
  assert.deepEqual(splitKeywords('  '), [])
  assert.deepEqual(splitKeywords('a b c'), ['a', 'b', 'c'])
})

test('scoreMatch returns null when no keyword matches', () => {
  const result = scoreMatch(
    { title: 'Hello world', content: 'lorem ipsum' },
    ['missing'],
  )
  assert.equal(result, null)
})

test('scoreMatch weights title hits ten times body hits', () => {
  const titleOnly = scoreMatch(
    { title: 'meeting notes', content: 'nothing relevant here' },
    ['meeting'],
  )
  assert.ok(titleOnly)
  assert.equal(titleOnly.score, 10)
  assert.equal(titleOnly.titleHits, 1)
  assert.equal(titleOnly.contentHits, 0)

  const bodyOnly = scoreMatch(
    { title: 'unrelated', content: 'a meeting happened' },
    ['meeting'],
  )
  assert.ok(bodyOnly)
  assert.equal(bodyOnly.score, 1)
  assert.equal(bodyOnly.titleHits, 0)
  assert.equal(bodyOnly.contentHits, 1)

  const bodyMulti = scoreMatch(
    { title: 'unrelated', content: 'meeting meeting meeting' },
    ['meeting'],
  )
  assert.ok(bodyMulti)
  assert.equal(bodyMulti.score, 3)
  assert.equal(bodyMulti.contentHits, 3)
})

test('scoreMatch AND semantics — every keyword must hit at least once', () => {
  const both = scoreMatch(
    { title: 'weekly meeting', content: 'notes from a weekly cycle' },
    ['weekly', 'meeting'],
  )
  assert.ok(both)
  // weekly: title 1 + body 1 → 11; meeting: title 1 → 10; total 21
  assert.equal(both.score, 21)
  assert.equal(both.titleHits, 2)
  assert.equal(both.contentHits, 1)

  const oneMiss = scoreMatch(
    { title: 'weekly meeting', content: 'no such word here' },
    ['weekly', 'missing'],
  )
  assert.equal(oneMiss, null)
})

test('scoreMatch is case-insensitive (lowercase compare)', () => {
  const upper = scoreMatch(
    { title: 'Weekly MEETING', content: 'a meeting happened here' },
    ['WEEKLY', 'meeting'],
  )
  assert.ok(upper)
  // weekly: title 1 (10). meeting: title 1 (10) + body 1 (1) = 11. total 21
  assert.equal(upper.score, 21)
})

test('buildSnippet surrounds the first keyword hit with ellipses', () => {
  const content = 'a'.repeat(200) + ' meeting happens here ' + 'b'.repeat(200)
  const snippet = buildSnippet(content, ['meeting'])
  assert.ok(snippet.includes('meeting'))
  assert.ok(snippet.startsWith('…') || snippet.startsWith('a'))
  assert.ok(snippet.endsWith('…') || snippet.endsWith('b'))
})

test('highlight wraps keyword occurrences with <mark> tags and escapes html', () => {
  const out = highlight('Hello <script>meeting</script> today', ['meeting'])
  assert.equal(out, 'Hello &lt;script&gt;<mark>meeting</mark>&lt;/script&gt; today')
})

test('highlight is empty when no keywords provided', () => {
  assert.equal(highlight('plain text', []), 'plain text')
})

test('highlight prefers the longest keyword when overlapping', () => {
  // The regex sorts longest first so "meeting" wins over "meet".
  const out = highlight('meeting room', ['meet', 'meeting'])
  assert.equal(out, '<mark>meeting</mark> room')
})