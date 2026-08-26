import assert from 'node:assert/strict'
import test from 'node:test'

import {
  AllAuditSources,
  actorKindLabelSuffix,
  emptySourceCounts,
  formatAuditTimestamp,
  opBadgeKind,
  opI18nKey,
  shortActorId,
  sourceLabelI18nKey,
  sourceLabelSuffix,
} from './wikiAuditDrawerLogic.ts'

test('AllAuditSources lists the four closed-set sources in render order', () => {
  assert.deepEqual(AllAuditSources, [
    'audit_logs',
    'wiki_batch_job_audit',
    'wiki_backlinks_cache_invalidation_log',
    'wiki_page_acl_audit',
  ])
})

test('sourceLabelSuffix maps each source to a stable i18n key suffix', () => {
  assert.equal(sourceLabelSuffix('audit_logs'), 'activity')
  assert.equal(sourceLabelSuffix('wiki_batch_job_audit'), 'batch')
  assert.equal(
    sourceLabelSuffix('wiki_backlinks_cache_invalidation_log'),
    'invalidation',
  )
  assert.equal(sourceLabelSuffix('wiki_page_acl_audit'), 'acl')
})

test('actorKindLabelSuffix normalizes unknown kinds to user', () => {
  assert.equal(actorKindLabelSuffix('system'), 'system')
  assert.equal(actorKindLabelSuffix('sweep'), 'sweep')
  assert.equal(actorKindLabelSuffix('user'), 'user')
  assert.equal(actorKindLabelSuffix(''), 'user')
  assert.equal(actorKindLabelSuffix('garbage'), 'user')
})

test('opBadgeKind derives a stable bucket from source, ignoring op', () => {
  assert.equal(opBadgeKind('manual_create', 'audit_logs'), 'activity')
  assert.equal(opBadgeKind('enqueue', 'wiki_batch_job_audit'), 'batch')
  assert.equal(opBadgeKind('set_private', 'wiki_page_acl_audit'), 'acl')
  assert.equal(
    opBadgeKind('acl_change', 'wiki_backlinks_cache_invalidation_log'),
    'invalidation',
  )
})

test('formatAuditTimestamp produces YYYY-MM-DD HH:mm:ss', () => {
  const formatted = formatAuditTimestamp('2026-08-27T12:34:56Z')
  // The exact wall-clock depends on TZ; assert the shape only.
  assert.match(formatted, /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
})

test('formatAuditTimestamp tolerates empty / invalid input', () => {
  assert.equal(formatAuditTimestamp(''), '')
  assert.equal(formatAuditTimestamp('not-a-date'), 'not-a-date')
})

test('shortActorId collapses long ids and passes through system/sweep', () => {
  assert.equal(shortActorId(''), '')
  assert.equal(shortActorId('system'), 'system')
  assert.equal(shortActorId('sweep'), 'sweep')
  assert.equal(shortActorId('short'), 'short')
  assert.equal(shortActorId('a-very-long-user-id-1234567890'), 'a-very-l…')
})

test('sourceLabelI18nKey composes the i18n key path', () => {
  assert.equal(sourceLabelI18nKey('audit_logs'), 'wiki.audit.source.activity')
  assert.equal(sourceLabelI18nKey('wiki_batch_job_audit'), 'wiki.audit.source.batch')
})

test('opI18nKey composes the per-op i18n key', () => {
  assert.equal(opI18nKey('manual_create'), 'wiki.audit.op.manual_create')
  assert.equal(opI18nKey('set_private'), 'wiki.audit.op.set_private')
})

test('emptySourceCounts returns zero-filled record on all four sources', () => {
  const counts = emptySourceCounts()
  assert.equal(counts.audit_logs, 0)
  assert.equal(counts.wiki_batch_job_audit, 0)
  assert.equal(counts.wiki_backlinks_cache_invalidation_log, 0)
  assert.equal(counts.wiki_page_acl_audit, 0)
})