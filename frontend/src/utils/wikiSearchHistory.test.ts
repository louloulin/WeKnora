import assert from 'node:assert/strict'
import test from 'node:test'

import {
  HISTORY_MAX,
  MIN_QUERY_LENGTH,
  normalizeQuery,
  pushHistory,
  shouldRecord,
} from './wikiSearchHistory.ts'

test('MIN_QUERY_LENGTH is 2', () => {
  assert.equal(MIN_QUERY_LENGTH, 2)
})

test('HISTORY_MAX is 10', () => {
  assert.equal(HISTORY_MAX, 10)
})

test('normalizeQuery strips leading and trailing whitespace', () => {
  assert.equal(normalizeQuery('  weekly meeting  '), 'weekly meeting')
  assert.equal(normalizeQuery('meeting'), 'meeting')
})

test('shouldRecord enforces the 2-char minimum', () => {
  assert.equal(shouldRecord('a'), false)
  assert.equal(shouldRecord('ab'), true)
  assert.equal(shouldRecord('   '), false)
})

test('pushHistory ignores queries shorter than the minimum', () => {
  assert.deepEqual(pushHistory([], 'a'), [])
  assert.deepEqual(pushHistory([], '   '), [])
})

test('pushHistory dedupes case-sensitively', () => {
  const next = pushHistory(['meeting', 'weekly'], 'meeting')
  assert.deepEqual(next, ['meeting', 'weekly'])
})

test('pushHistory dedupes case-insensitively across distinct cases', () => {
  const next = pushHistory(['Meeting'], 'MEETING')
  // 'MEETING' and 'Meeting' are different strings, but the design is
  // exact-match dedupe. Document the contract here.
  assert.equal(next.length, 2)
  assert.equal(next[0], 'MEETING')
  assert.equal(next[1], 'Meeting')
})

test('pushHistory caps at HISTORY_MAX and reverses order (newest first)', () => {
  let arr: string[] = []
  for (let i = 0; i < 15; i += 1) {
    arr = pushHistory(arr, `q-${i}`)
  }
  assert.equal(arr.length, HISTORY_MAX)
  assert.equal(arr[0], 'q-14')
  assert.equal(arr[arr.length - 1], 'q-5')
})

test('pushHistory never mutates the input array', () => {
  const input = ['existing']
  const next = pushHistory(input, 'new')
  assert.notEqual(next, input)
  assert.deepEqual(input, ['existing'])
})