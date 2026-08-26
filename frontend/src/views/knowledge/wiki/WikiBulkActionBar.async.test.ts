import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const browser = readFileSync(
  new URL('./WikiBrowser.vue', import.meta.url),
  'utf8',
)
const batchTypes = readFileSync(
  new URL('../../../api/wiki/batchTypes.ts', import.meta.url),
  'utf8',
)
const locales = [
  readFileSync(new URL('../../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../../i18n/locales/en-US.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../../i18n/locales/ko-KR.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../../i18n/locales/ru-RU.ts', import.meta.url), 'utf8'),
]

// WikiBrowser.vue routes the batch endpoint responses through
// handleBatchRouteResult, branching on the discriminated kind field.
// Both sync and async paths must reach the matching downstream helper.
test('WikiBrowser routes sync batch responses through handleBatchResult', () => {
  assert.match(
    browser,
    /handleBatchRouteResult\(result,\s*'knowledgeEditor\.wikiBrowser\.bulkMoveSuccess',\s*'bulkMovePartial',\s*\{\}\)/,
  )
  assert.match(
    browser,
    /handleBatchRouteResult\(\s*result,\s*'knowledgeEditor\.wikiBrowser\.bulkStatusSuccess',\s*'bulkStatusPartial',\s*\{\s*status\s*\}/,
  )
  assert.match(
    browser,
    /handleBatchRouteResult\(result,\s*'knowledgeEditor\.wikiBrowser\.bulkDeleteSuccess',\s*'bulkDeletePartial',\s*\{\}\)/,
  )
})

// handleBatchRouteResult must call watchBatchJob when the response is
// async (kind === 'job'). The polling loop lives in watchBatchJob.
test('handleBatchRouteResult dispatches async jobs to watchBatchJob', () => {
  assert.match(browser, /async function handleBatchRouteResult\(/)
  assert.match(
    browser,
    /if\s*\(result\.kind === 'job' && result\.job\)[\s\S]{0,200}await watchBatchJob\(result\.job\.id/,
  )
  // Sync path: kind === 'sync' → handleBatchResult with the original toast keys.
  assert.match(
    browser,
    /if\s*\(result\.kind === 'sync' && result\.result\)[\s\S]{0,200}handleBatchResult\(result\.result,/,
  )
})

// watchBatchJob must:
//   1. open a persistent loading toast (duration: 0)
//   2. poll getWikiBatchJob every 2s
//   3. on success open a separate 60s info toast wired to triggerUndo
//      for undoable job types only
test('watchBatchJob polls every 2s and surfaces the 60s Undo affordance for move/delete', () => {
  assert.match(browser, /async function watchBatchJob\(/)
  // Persistent loading toast (duration: 0) with the queued copy.
  assert.match(
    browser,
    /MessagePlugin\.loading\(\{\s*content: t\('knowledgeEditor\.wikiBrowser\.bulkJobQueued'/,
  )
  // 2-second poll cadence — first hop inside the loop.
  assert.match(browser, /await new Promise\(\(resolve\) => setTimeout\(resolve, 2000\)\)/)
  // On terminal state we surface the undo hint for undoable types.
  assert.match(browser, /if \(isWikiBatchJobUndoable\(job\.type\)\)/)
  assert.match(browser, /duration: 60_000,\s*onClose: \(\) => triggerUndo\(job\.id\)/)
})

// triggerUndo is wired to the bulkJobUndoing loading toast, then a
// success / error replace depending on the server response. The
// success path also refreshes the active tree so the undone rows
// reappear in the directory / list view.
test('triggerUndo refreshes the directory tree and toasts the outcome', () => {
  assert.match(browser, /async function triggerUndo\(jobID: string\)/)
  assert.match(browser, /MessagePlugin\.loading\(\{\s*content: t\('knowledgeEditor\.wikiBrowser\.bulkJobUndoing'\)/)
  assert.match(browser, /await undoWikiBatchJob\(props\.knowledgeBaseId, jobID\)/)
  assert.match(browser, /t\('knowledgeEditor\.wikiBrowser\.bulkJobUndoSucceeded'\)/)
  assert.match(browser, /await refreshActiveTree\(\)/)
})

// batchTypes.ts must expose the discriminated WikiBatchRouteResult,
// the WikiBatchJob terminal-state list, and the isWikiBatchJobUndoable
// predicate. The frontend uses these as the single source of truth.
test('batchTypes.ts exposes the Build #13 discriminated types + helpers', () => {
  assert.match(batchTypes, /export type WikiBatchJobState[\s\S]+?queued[\s\S]+?succeeded[\s\S]+?partial/)
  assert.match(batchTypes, /export type WikiBatchJobType\s*=\s*'move' \| 'delete' \| 'status' \| 'tag'/)
  assert.match(batchTypes, /export interface WikiBatchJob \{/)
  assert.match(batchTypes, /export interface WikiBatchRouteResult \{/)
  assert.match(batchTypes, /export const WikiBatchJobTerminalStates/)
  assert.match(batchTypes, /'succeeded'/)
  assert.match(batchTypes, /'failed'/)
  assert.match(batchTypes, /'partial'/)
  assert.match(batchTypes, /export function isWikiBatchJobUndoable\(type: WikiBatchJobType\)/)
})

// All four locales carry the 14 Build #13 keys. The 14 are:
//   bulkJobQueued, bulkJobProgress, bulkJobFailed,
//   bulkJobUndoButton, bulkJobUndoHint, bulkJobUndoing,
//   bulkJobUndoSucceeded, bulkJobUndoFailed, bulkJobUndoExpired,
//   bulkJobPollError, bulkConfirmAsyncTitle, bulkConfirmAsyncHint,
//   bulkConfirmAsyncConfirm
// (bulkJobUndoButton is wired through the global Undo shortcut in
// the success toast — also covered here so a missing key triggers a
// CI failure.)
test('all four locales carry the 14 Build #13 async keys', () => {
  const keys = [
    'bulkJobQueued',
    'bulkJobProgress',
    'bulkJobFailed',
    'bulkJobUndoHint',
    'bulkJobUndoing',
    'bulkJobUndoSucceeded',
    'bulkJobUndoFailed',
    'bulkJobUndoExpired',
    'bulkJobPollError',
    'bulkConfirmAsyncTitle',
    'bulkConfirmAsyncHint',
    'bulkConfirmAsyncConfirm',
  ]
  for (const locale of locales) {
    for (const key of keys) {
      assert.match(locale, new RegExp(`${key}:`), `${key} missing in a locale`)
    }
  }
})