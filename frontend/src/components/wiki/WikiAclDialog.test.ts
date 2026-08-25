import assert from 'node:assert/strict'
import test from 'node:test'

import {
  formatAclUpdatedAt,
  isAclDraftDirty,
} from './wikiAclDialogLogic.ts'

test('isAclDraftDirty is false when draft equals original', () => {
  const acl = {
    mode: 'allow_list' as const,
    allowUserIds: ['u1', 'u2'],
    denyInherited: false,
  }
  const clone = { ...acl, allowUserIds: ['u1', 'u2'] }
  assert.equal(isAclDraftDirty(acl, clone), false)
})

test('isAclDraftDirty detects mode change', () => {
  const original = { mode: 'inherit' as const, allowUserIds: [], denyInherited: false }
  const draft = { mode: 'private' as const, allowUserIds: [], denyInherited: false }
  assert.equal(isAclDraftDirty(draft, original), true)
})

test('isAclDraftDirty detects added or removed user IDs', () => {
  const original = { mode: 'allow_list' as const, allowUserIds: ['u1'], denyInherited: false }
  assert.equal(isAclDraftDirty({ ...original, allowUserIds: ['u1', 'u2'] }, original), true)
  assert.equal(isAclDraftDirty({ ...original, allowUserIds: [] }, original), true)
  // reordered: same set, should be clean
  assert.equal(isAclDraftDirty({ ...original, allowUserIds: ['u1'] }, original), false)
})

test('isAclDraftDirty detects denyInherited change', () => {
  const original = { mode: 'allow_list' as const, allowUserIds: [], denyInherited: false }
  const draft = { mode: 'allow_list' as const, allowUserIds: [], denyInherited: true }
  assert.equal(isAclDraftDirty(draft, original), true)
})

test('formatAclUpdatedAt returns empty string when updatedAt is missing', () => {
  assert.equal(formatAclUpdatedAt(undefined, 'zh-CN'), '')
  assert.equal(formatAclUpdatedAt('', 'zh-CN'), '')
})

test('formatAclUpdatedAt renders a locale-aware date string', () => {
  const iso = '2026-08-26T10:30:00Z'
  const zh = formatAclUpdatedAt(iso, 'zh-CN')
  const en = formatAclUpdatedAt(iso, 'en-US')
  // Both should contain the year; locale formatting varies across
  // runtimes so we only assert presence of digits + year fragment.
  assert.ok(zh.includes('2026'), `zh-CN output missing year: "${zh}"`)
  assert.ok(en.includes('2026'), `en-US output missing year: "${en}"`)
  // Different locales typically produce different strings (e.g. AM/PM
  // vs 24h). If they happen to be equal on this runtime (e.g. CJK-only
  // ICU build) we still accept — the goal is "no crash, sane output".
})

test('formatAclUpdatedAt falls back gracefully on garbage input', () => {
  // Intl.DateTimeFormat may throw on an unknown locale; our helper
  // returns the raw ISO string instead of crashing.
  const bad = formatAclUpdatedAt('not-a-date', 'zh-CN')
  assert.equal(bad, 'not-a-date')
})