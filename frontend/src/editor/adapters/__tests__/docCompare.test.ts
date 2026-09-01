// v0.7.69 — DOC compare / diff utilities

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  compareParagraphs,
  summarize,
  blockTexts,
  type CompareEntry,
} from '../docCompare'

test('compareParagraphs: identical inputs return only same entries', () => {
  const out = compareParagraphs(['a', 'b', 'c'], ['a', 'b', 'c'])
  assert.equal(out.length, 3)
  assert.ok(out.every((e) => e.kind === 'same'))
})

test('compareParagraphs: completely different inputs', () => {
  const out = compareParagraphs(['a', 'b'], ['x', 'y'])
  // LCS-based diff: removed a, removed b, added x, added y
  // merge: removed(b) + added(x) merges to changed (b->x); added(y) stays
  assert.equal(out.length, 3)
  assert.equal(out[0]!.kind, 'removed')
  assert.equal(out[0]!.left, 'a')
  assert.equal(out[1]!.kind, 'changed')
  assert.equal(out[1]!.left, 'b')
  assert.equal(out[1]!.right, 'x')
  assert.equal(out[2]!.kind, 'added')
  assert.equal(out[2]!.right, 'y')
})

test('compareParagraphs: pure additions', () => {
  const out = compareParagraphs(['a'], ['a', 'b', 'c'])
  assert.equal(out.length, 3)
  assert.equal(out[0]!.kind, 'same')
  assert.equal(out[1]!.kind, 'added')
  assert.equal(out[1]!.right, 'b')
  assert.equal(out[2]!.kind, 'added')
  assert.equal(out[2]!.right, 'c')
})

test('compareParagraphs: pure removals', () => {
  const out = compareParagraphs(['a', 'b', 'c'], ['a'])
  assert.equal(out.length, 3)
  assert.equal(out[0]!.kind, 'same')
  assert.equal(out[1]!.kind, 'removed')
  assert.equal(out[1]!.left, 'b')
  assert.equal(out[2]!.kind, 'removed')
  assert.equal(out[2]!.left, 'c')
})

test('compareParagraphs: edit in the middle', () => {
  const out = compareParagraphs(['a', 'b', 'c'], ['a', 'B', 'c'])
  assert.equal(out.length, 3)
  assert.equal(out[0]!.kind, 'same')
  assert.equal(out[1]!.kind, 'changed')
  assert.equal(out[1]!.left, 'b')
  assert.equal(out[1]!.right, 'B')
  assert.equal(out[2]!.kind, 'same')
})

test('compareParagraphs: empty inputs', () => {
  assert.deepEqual(compareParagraphs([], []), [])
  assert.equal(compareParagraphs([], ['a']).length, 1)
  assert.equal(compareParagraphs(['a'], []).length, 1)
})

test('summarize: counts added / removed / changed', () => {
  const entries: CompareEntry[] = [
    { kind: 'same', left: 'a', right: 'a' },
    { kind: 'added', right: 'b' },
    { kind: 'removed', left: 'c' },
    { kind: 'changed', left: 'd', right: 'D' },
  ]
  const s = summarize(entries)
  assert.equal(s.added, 1)
  assert.equal(s.removed, 1)
  assert.equal(s.changed, 1)
})

test('summarize: zero counts for identical docs', () => {
  const out = compareParagraphs(['x'], ['x'])
  const s = summarize(out)
  assert.equal(s.added + s.removed + s.changed, 0)
})

test('blockTexts: extracts runs text and falls back to previewText', () => {
  const blocks = [
    { hidden: false, runs: [{ text: 'hello' }, { text: ' world' }], previewText: '' },
    { hidden: false, runs: undefined, previewText: 'image preview' },
    { hidden: true, runs: [{ text: 'should be skipped' }], previewText: '' },
  ] as any
  const texts = blockTexts(blocks)
  assert.deepEqual(texts, ['hello world', 'image preview'])
})
