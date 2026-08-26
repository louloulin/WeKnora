import assert from 'node:assert/strict'
import test from 'node:test'

import {
  AllAuditActions,
  isTerminalAuditAction,
  jobDrawerTitleTokens,
  shortJobId,
} from './wikiBatchAuditLogic.ts'

test('AllAuditActions lists the seven closed-set actions in lifecycle order', () => {
  assert.deepEqual(AllAuditActions, [
    'enqueue',
    'start',
    'finish',
    'undo_request',
    'undo_done',
    'cancel',
    'expire',
  ])
})

test('isTerminalAuditAction flags finish, undo_done, cancel, expire only', () => {
  for (const a of AllAuditActions) {
    const expected =
      a === 'finish' ||
      a === 'undo_done' ||
      a === 'cancel' ||
      a === 'expire'
    assert.equal(isTerminalAuditAction(a), expected, `action ${a}`)
  }
})

test('isTerminalAuditAction returns false for non-terminal lifecycle steps', () => {
  assert.equal(isTerminalAuditAction('enqueue'), false)
  assert.equal(isTerminalAuditAction('start'), false)
  assert.equal(isTerminalAuditAction('undo_request'), false)
})

test('shortJobId returns the first 8 chars of the UUID', () => {
  assert.equal(
    shortJobId('1234567890abcdef1234567890abcdef'),
    '12345678',
  )
})

test('shortJobId returns "" for empty input so the link cell stays blank', () => {
  assert.equal(shortJobId(''), '')
})

test('jobDrawerTitleTokens wraps the short id for i18n interpolation', () => {
  assert.deepEqual(
    jobDrawerTitleTokens('1234567890abcdef1234567890abcdef'),
    { id: '12345678' },
  )
})

test('jobDrawerTitleTokens returns empty id for empty jobId', () => {
  assert.deepEqual(jobDrawerTitleTokens(''), { id: '' })
})