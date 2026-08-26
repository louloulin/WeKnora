import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const dialog = readFileSync(
  new URL('./WikiBatchPreviewDialog.vue', import.meta.url),
  'utf8',
)
const bar = readFileSync(
  new URL('./WikiBulkActionBar.vue', import.meta.url),
  'utf8',
)
const batchTypes = readFileSync(
  new URL('../../api/wiki/batchTypes.ts', import.meta.url),
  'utf8',
)
const indexApi = readFileSync(
  new URL('../../api/wiki/index.ts', import.meta.url),
  'utf8',
)
const locales = [
  readFileSync(new URL('../../i18n/locales/zh-CN.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/en-US.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/ko-KR.ts', import.meta.url), 'utf8'),
  readFileSync(new URL('../../i18n/locales/ru-RU.ts', import.meta.url), 'utf8'),
]

// Build #16 — the dialog renders a summary header with success + fail
// counts and a per-slug table with code badges. The summary keys come
// from t('wiki.batchPreview.{summary,willSucceed,willFail}'). Both
// the dialog and the bar must agree on those keys.
test('WikiBatchPreviewDialog renders summary header + per-slug table', () => {
  assert.match(dialog, /t\('wiki\.batchPreview\.willSucceed'/)
  assert.match(dialog, /t\('wiki\.batchPreview\.willFail'/)
  assert.match(dialog, /t\('wiki\.batchPreview\.summary'/)
  assert.match(dialog, /t\('wiki\.batchPreview\.codeHeader'/)
  assert.match(dialog, /tableRows/)
  assert.match(dialog, /WikiBatchErrorCodeToI18nKey/)
})

// D7 = A — the preview button (vs. original "确认" / "执行") only
// renders when count >= WikiBatchAsyncThreshold (20) and not busy.
test('WikiBulkActionBar gates the preview button on count >= threshold', () => {
  assert.match(bar, /showPreviewButtons\s*=\s*computed\(/)
  assert.match(bar, /count\.value >= WikiBatchAsyncThreshold/)
  assert.match(bar, /bulkPreview/)
  assert.match(bar, /openPreview\(/)
  assert.match(bar, /onPreviewConfirm\(/)
  // Move + Delete dialogs must show "预览" instead of "确认" when gated.
  // (Vue template attribute is multi-line: `confirm-btn="\n showPreviewButtons\n ? t(...)\n ..."`.)
  assert.match(
    bar,
    /showPreviewButtons[\s\S]{0,40}\?[\s\S]{0,40}t\('knowledgeEditor\.wikiBrowser\.bulkPreview'\)/,
  )
  assert.match(
    bar,
    /showPreviewButtons[\s\S]{0,80}\?[\s\S]{0,80}t\('knowledgeEditor\.wikiBrowser\.bulkPreview'\)[\s\S]{0,80}t\('knowledgeEditor\.wikiBrowser\.bulkDeleteConfirm'\)/,
  )
  // Status dropdown must route through onStatusClick → openPreview when gated.
  assert.match(bar, /function onStatusClick\(/)
  assert.match(bar, /openPreview\('status'\)/)
})

// batchTypes.ts must expose WikiBatchPreviewResponse + the new
// WikiBatchAsyncThreshold constant. Both are part of the contract.
test('batchTypes.ts exposes the Build #16 preview types + threshold', () => {
  assert.match(batchTypes, /export interface WikiBatchPreviewResponse/)
  assert.match(batchTypes, /WikiBatchPreviewSummary/)
  assert.match(batchTypes, /will_succeed/)
  assert.match(batchTypes, /will_fail/)
  assert.match(batchTypes, /WikiBatchPreviewType/)
  assert.match(batchTypes, /export const WikiBatchAsyncThreshold\s*=\s*\d+/)
})

// API client must wrap each of the three preview endpoints.
test('api/wiki/index.ts wires the three batchPreview* functions', () => {
  assert.match(indexApi, /export\s+(?:async\s+)?function batchPreviewWikiPagesMove/)
  assert.match(indexApi, /export\s+(?:async\s+)?function batchPreviewWikiPagesDelete/)
  assert.match(indexApi, /export\s+(?:async\s+)?function batchPreviewWikiPagesStatus/)
  assert.match(indexApi, /batch-preview-move/)
  assert.match(indexApi, /batch-preview-delete/)
  assert.match(indexApi, /batch-preview-status/)
})

// All four locales must carry every Build #16 key. The keys are split
// across two namespaces:
//   wiki.batchPreview.{title,summary,willSucceed,willFail,willSucceedTag,
//                        unknownCode,confirm,cancel,empty,codeHeader,
//                        typeLabel.move,typeLabel.delete,typeLabel.status}
//   wikiBrowser.{bulkPreview, batchPreviewLoadFailed}
test('all four locales carry the Build #16 preview keys', () => {
  const previewKeys = [
    'title',
    'summary',
    'willSucceed',
    'willFail',
    'willSucceedTag',
    'unknownCode',
    'confirm',
    'cancel',
    'empty',
    'codeHeader',
  ]
  const barKeys = ['bulkPreview', 'batchPreviewLoadFailed']
  for (const locale of locales) {
    // batchPreview namespace block + typeLabel sub-block.
    assert.match(locale, /batchPreview:\s*{/, 'batchPreview block missing')
    for (const key of previewKeys) {
      assert.match(
        locale,
        new RegExp(`${key}:`),
        `${key} missing in batchPreview block`,
      )
    }
    assert.match(locale, /typeLabel:\s*{/, 'typeLabel sub-block missing')
    assert.match(locale, /move:\s*'[^']+'/)
    assert.match(locale, /delete:\s*'[^']+'/)
    assert.match(locale, /status:\s*'[^']+'/)
    for (const key of barKeys) {
      assert.match(
        locale,
        new RegExp(`${key}:`),
        `${key} missing in wikiBrowser block`,
      )
    }
  }
})