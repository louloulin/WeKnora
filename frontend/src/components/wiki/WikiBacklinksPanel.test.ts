import assert from 'node:assert/strict'
import test from 'node:test'

import {
  backlinksBodyId,
  backlinksCountLabel,
  emptyStateHint,
  formatBacklinkTimestamp,
} from './wikiBacklinksPanelLogic.ts'
import type { Backlink } from '../../api/wiki/backlinksHelpers'

const fixture: Backlink[] = [
  {
    slug: 'summary/intro',
    title: 'Intro',
    pageType: 'summary',
    status: 'live',
    updatedAt: '2026-08-22T10:00:00Z',
  },
  {
    slug: 'entity/acme',
    title: 'Acme',
    pageType: 'entity',
    status: 'live',
    updatedAt: '2026-08-15T10:00:00Z',
  },
]

test('backlinksCountLabel returns "" when list is undefined (hidden)', () => {
  assert.equal(backlinksCountLabel(undefined), '')
})

test('backlinksCountLabel returns "(0)" for empty list (empty state)', () => {
  assert.equal(backlinksCountLabel([]), '(0)')
})

test('backlinksCountLabel returns "(N)" for populated list', () => {
  assert.equal(backlinksCountLabel(fixture), '(2)')
})

test('formatBacklinkTimestamp parses RFC3339 and returns short date', () => {
  const out = formatBacklinkTimestamp('2026-08-22T10:00:00Z')
  assert.ok(out.length > 0)
  assert.ok(out.includes('2026'))
  assert.ok(/Aug/.test(out))
})

test('formatBacklinkTimestamp returns "" for invalid input', () => {
  assert.equal(formatBacklinkTimestamp(''), '')
  assert.equal(formatBacklinkTimestamp('not-a-date'), '')
})

test('emptyStateHint returns the slug when present', () => {
  assert.equal(emptyStateHint('summary/intro'), 'summary/intro')
})

test('emptyStateHint returns <slug> placeholder when slug is empty', () => {
  assert.equal(emptyStateHint(''), '<slug>')
  assert.equal(emptyStateHint('   '), '<slug>')
})

test('backlinksBodyId sanitizes non-identifier characters', () => {
  assert.equal(
    backlinksBodyId('kb-1', 'summary/intro'),
    'wiki-backlinks-body-kb-1-summary_intro',
  )
  assert.equal(
    backlinksBodyId('kb 1', 'foo bar'),
    'wiki-backlinks-body-kb_1-foo_bar',
  )
})

test('backlinksBodyId is stable per (kb, slug)', () => {
  assert.equal(
    backlinksBodyId('kb1', 'page-a'),
    backlinksBodyId('kb1', 'page-a'),
  )
  assert.notEqual(
    backlinksBodyId('kb1', 'page-a'),
    backlinksBodyId('kb1', 'page-b'),
  )
})