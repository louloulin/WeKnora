import assert from 'node:assert/strict'
import test from 'node:test'

import {
  KnownCodes,
  failureDrawerTitleTokens,
  normalizeFilter,
  sortGroups,
  totalFromGroups,
} from './wikiBatchFailureLogic.ts'
import type { WikiBatchFailureGroupCount } from '../../api/wiki/batchTypes.ts'

test('KnownCodes mirrors the closed set the worker emits', () => {
  assert.deepEqual(KnownCodes, [
    'not_found',
    'folder_not_found',
    'folder_conflict',
    'folder_not_empty',
    'kb_mismatch',
    'internal',
  ])
})

test('totalFromGroups sums every bucket — drives the "全部" tab badge', () => {
  const groups: WikiBatchFailureGroupCount[] = [
    { code: 'not_found', count: 4 },
    { code: 'internal', count: 1 },
    { code: 'kb_mismatch', count: 2 },
  ]
  assert.equal(totalFromGroups(groups), 7)
})

test('totalFromGroups returns 0 for an empty group slice', () => {
  assert.equal(totalFromGroups([]), 0)
})

test('normalizeFilter drops empty code so the URL omits ?code=', () => {
  assert.deepEqual(
    normalizeFilter({ code: '   ', page: 1, page_size: 50 }),
    { page: 1, page_size: 50 },
  )
})

test('normalizeFilter trims whitespace on a non-empty code', () => {
  assert.deepEqual(
    normalizeFilter({ code: '  not_found  ', page: 2, page_size: 100 }),
    { code: 'not_found', page: 2, page_size: 100 },
  )
})

test('normalizeFilter drops page=0 and page_size=0', () => {
  assert.deepEqual(
    normalizeFilter({ code: 'internal', page: 0, page_size: 0 }),
    { code: 'internal' },
  )
})

test('normalizeFilter preserves a fully-populated filter unchanged', () => {
  assert.deepEqual(
    normalizeFilter({ code: 'kb_mismatch', page: 3, page_size: 200 }),
    { code: 'kb_mismatch', page: 3, page_size: 200 },
  )
})

test('sortGroups keeps the KnownCodes order first, unknown codes appended', () => {
  const groups: WikiBatchFailureGroupCount[] = [
    { code: 'kb_mismatch', count: 1 },
    { code: 'not_found', count: 4 },
    { code: 'internal', count: 2 },
    { code: 'unknown_code', count: 1 },
    { code: 'folder_not_found', count: 3 },
  ]
  const sorted = sortGroups(groups)
  assert.deepEqual(
    sorted.map((g) => g.code),
    [
      'not_found',
      'folder_not_found',
      'kb_mismatch',
      'internal',
      'unknown_code',
    ],
  )
})

test('sortGroups returns an empty list when no groups', () => {
  assert.deepEqual(sortGroups([]), [])
})

test('sortGroups sorts multiple unknown codes alphabetically among themselves', () => {
  const groups: WikiBatchFailureGroupCount[] = [
    { code: 'zeta', count: 1 },
    { code: 'alpha', count: 1 },
    { code: 'beta', count: 1 },
  ]
  const sorted = sortGroups(groups)
  assert.deepEqual(
    sorted.map((g) => g.code),
    ['alpha', 'beta', 'zeta'],
  )
})

test('failureDrawerTitleTokens returns the first 8 chars of the UUID', () => {
  assert.deepEqual(
    failureDrawerTitleTokens('1234567890abcdef1234567890abcdef'),
    { jobId: '12345678' },
  )
})

test('failureDrawerTitleTokens returns empty jobId for empty input', () => {
  assert.deepEqual(failureDrawerTitleTokens(''), { jobId: '' })
})

test('failureDrawerTitleTokens returns the full id when shorter than 8 chars', () => {
  assert.deepEqual(
    failureDrawerTitleTokens('abc12345'),
    { jobId: 'abc12345' },
  )
})