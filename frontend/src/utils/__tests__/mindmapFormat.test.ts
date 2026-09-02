/**
 * mindmapFormat.test — v0.7.111 MindMap / WIKI 打磨 (Phase 1)
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatShortDate, formatLongDate, isMindMapLayout } from '../mindmapFormat'

test('formatShortDate — empty returns empty', () => {
  assert.equal(formatShortDate(''), '')
})

test('formatShortDate — invalid returns empty', () => {
  assert.equal(formatShortDate('not-a-date'), '')
})

test('formatShortDate — ISO timestamp produces a non-empty Chinese-format date', () => {
  const out = formatShortDate('2026-09-02T10:00:00Z')
  assert.ok(out.length > 0, `expected non-empty output, got ${JSON.stringify(out)}`)
  // toLocaleString with zh-CN may include "9月2日" — only assert it has digits.
  assert.ok(/\d/.test(out), `expected digits in ${JSON.stringify(out)}`)
})

test('formatLongDate — ISO timestamp produces full datetime', () => {
  const out = formatLongDate('2026-09-02T10:00:00Z')
  assert.ok(out.length > 0)
  assert.ok(/\d{4}/.test(out) || /\d{2}/.test(out))
})

test('formatLongDate — empty returns empty', () => {
  assert.equal(formatLongDate(''), '')
})

test('isMindMapLayout — known layouts', () => {
  assert.equal(isMindMapLayout('tree'), true)
  assert.equal(isMindMapLayout('fishbone'), true)
  assert.equal(isMindMapLayout('timeline'), true)
  assert.equal(isMindMapLayout('radial'), true)
  assert.equal(isMindMapLayout('free'), true)
})

test('isMindMapLayout — unknown layout', () => {
  assert.equal(isMindMapLayout('unknown'), false)
  assert.equal(isMindMapLayout(''), false)
})
