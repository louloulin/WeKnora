// v0.7.80 — DOC paragraph border group merging (ECMA-376 §17.3.1.24).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  sameBorderGroup,
  borderMergeFlags,
  type ParaBorderAttrs,
} from '../docParaBorderMerge'

function mk(borders: string | null, borderLines?: string, fill?: string): ParaBorderAttrs {
  return { borders, borderLines: borderLines ?? null, shadingFill: fill ?? null }
}

test('sameBorderGroup: both borders null → false (no group)', () => {
  assert.equal(sameBorderGroup(mk(null), mk(null)), false)
})

test('sameBorderGroup: matching borders + fill → true', () => {
  assert.equal(
    sameBorderGroup(mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'), mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff')),
    true,
  )
})

test('sameBorderGroup: same borders, different fill → false', () => {
  assert.equal(
    sameBorderGroup(mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'), mk('all', '{"t":{"color":"#000","szPt":1}}', '#000')),
    false,
  )
})

test('sameBorderGroup: same borders + fill, different line color → false', () => {
  assert.equal(
    sameBorderGroup(
      mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'),
      mk('all', '{"t":{"color":"#f00","szPt":1}}', '#fff'),
    ),
    false,
  )
})

test('sameBorderGroup: borders JSON key order insensitive', () => {
  // top before left in first; left before top in second
  const a = mk('all', '{"t":{"color":"#000","szPt":1},"l":{"color":"#000","szPt":1}}', '#fff')
  const b = mk('all', '{"l":{"color":"#000","szPt":1},"t":{"color":"#000","szPt":1}}', '#fff')
  assert.equal(sameBorderGroup(a, b), true, 'key order should not matter')
})

test('sameBorderGroup: shadingDisplay wins over shadingFill when both present', () => {
  const a: ParaBorderAttrs = { borders: 'all', shadingFill: '#fff', shadingDisplay: '#eee' }
  const b: ParaBorderAttrs = { borders: 'all', shadingFill: '#000', shadingDisplay: '#eee' }
  assert.equal(sameBorderGroup(a, b), true, 'display value equal even though fill differs')
})

test('sameBorderGroup: invalid borderLines JSON → falls back to raw compare', () => {
  const a = mk('all', 'not-json{', '#fff')
  const b = mk('all', 'not-json{', '#fff')
  assert.equal(sameBorderGroup(a, b), true)
})

test('borderMergeFlags: single paragraph → no suppression', () => {
  const paras = [mk('all', '{}', '#fff')]
  const flags = borderMergeFlags(paras)
  assert.deepEqual(flags, [{ suppressTop: false, suppressBottom: false }])
})

test('borderMergeFlags: 3 same-group paragraphs → middle has both suppressed', () => {
  const paras = [
    mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'),
    mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'),
    mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'),
  ]
  assert.deepEqual(borderMergeFlags(paras), [
    { suppressTop: false, suppressBottom: true },
    { suppressTop: true, suppressBottom: true },
    { suppressTop: true, suppressBottom: false },
  ])
})

test('borderMergeFlags: different groups → no suppression between them', () => {
  const paras = [
    mk('all', '{"t":{"color":"#000","szPt":1}}', '#fff'),
    mk('all', '{"t":{"color":"#f00","szPt":1}}', '#fff'),
  ]
  assert.deepEqual(borderMergeFlags(paras), [
    { suppressTop: false, suppressBottom: false },
    { suppressTop: false, suppressBottom: false },
  ])
})

test('borderMergeFlags: empty list → empty flags', () => {
  assert.deepEqual(borderMergeFlags([]), [])
})
