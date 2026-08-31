/**
 * SHEET freeze-pane adapter round-trip — Feishu/Lark/Tencent-equivalent
 * "freeze first row / first column / both". Exercises the JSZip-level
 * sheetView/pane write path that xlsxAdapter exposes.
 *
 * Run with:  ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/xlsxFreeze.test.ts
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  openXlsx,
  saveXlsxBytes,
  newXlsxWorkbook,
  readXlsxFreeze,
  applyXlsxFreeze,
  buildSheetViewXml,
  type XlsxFreezeMap,
} from '../xlsxAdapter'

test('buildSheetViewXml emits frozen pane fragment', () => {
  const xml = buildSheetViewXml({ rows: 1, cols: 0 })
  assert.ok(xml.includes('<pane '), 'pane element present')
  assert.ok(xml.includes('state="frozen"'), 'frozen state')
  assert.ok(xml.includes('ySplit="1"'), 'row split')
  assert.ok(xml.includes('topLeftCell="A2"'), 'topLeft after 1 frozen row')
  assert.ok(xml.includes('activePane="bottomLeft"'), 'bottom-left active pane')
})

test('buildSheetViewXml emits empty sheetViews for no freeze', () => {
  const xml = buildSheetViewXml({ rows: 0, cols: 0 })
  assert.ok(xml.includes('<sheetViews>'), 'sheetViews still present')
  assert.ok(!xml.includes('<pane '), 'no pane element when no freeze')
})

test('freeze round-trip: freeze-first-row survives .xlsx byte save & reload', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [{ v: 'h1' }, { v: 'h2' }],
    [{ v: 1 }, { v: 2 }],
    [{ v: 3 }, { v: 4 }],
  ]
  const bytes0 = await saveXlsxBytes(wb)
  // No freeze yet.
  const before = await readXlsxFreeze(bytes0)
  assert.deepEqual(before.freezes, {}, 'no freeze on a fresh workbook')

  // Apply freeze-first-row to Sheet1.
  const freezes: XlsxFreezeMap = { Sheet1: { rows: 1, cols: 0 } }
  const bytes1 = await applyXlsxFreeze(bytes0, freezes)

  // Re-read after write.
  const after = await readXlsxFreeze(bytes1)
  assert.deepEqual(after.sheetNames, ['Sheet1'], 'workbook still has Sheet1')
  assert.deepEqual(after.freezes, freezes, 'freeze pane survived round-trip')

  // SheetJS can still open the file.
  const reopened = await openXlsx(bytes1)
  assert.equal(reopened.sheets.length, 1, 'sheet count preserved')
  assert.equal(reopened.sheets[0].rows[0][0].v, 'h1', 'cell values preserved')
})

test('freeze round-trip: freeze-first-column emits correct xSplit', async () => {
  const wb = newXlsxWorkbook()
  const bytes0 = await saveXlsxBytes(wb)
  const freezes: XlsxFreezeMap = { Sheet1: { rows: 0, cols: 1 } }
  const bytes1 = await applyXlsxFreeze(bytes0, freezes)
  const after = await readXlsxFreeze(bytes1)
  assert.deepEqual(after.freezes, freezes, 'col freeze round-trip')
})

test('freeze round-trip: both axes (corner) emits bottom-right pane', async () => {
  const wb = newXlsxWorkbook()
  const bytes0 = await saveXlsxBytes(wb)
  const freezes: XlsxFreezeMap = { Sheet1: { rows: 2, cols: 3 } }
  const bytes1 = await applyXlsxFreeze(bytes0, freezes)
  const after = await readXlsxFreeze(bytes1)
  assert.deepEqual(after.freezes, freezes, 'both-axis freeze round-trip')
  const xml = buildSheetViewXml({ rows: 2, cols: 3 })
  assert.ok(xml.includes('topLeftCell="D3"'), 'topLeft after 2 rows + 3 cols freeze')
  assert.ok(xml.includes('activePane="bottomRight"'), 'bottom-right active pane')
})

test('freeze round-trip: clear freeze by passing empty map', async () => {
  const wb = newXlsxWorkbook()
  const bytes0 = await saveXlsxBytes(wb)
  const frozen = await applyXlsxFreeze(bytes0, { Sheet1: { rows: 1, cols: 1 } })
  const cleared = await applyXlsxFreeze(frozen, { Sheet1: { rows: 0, cols: 0 } })
  const after = await readXlsxFreeze(cleared)
  assert.deepEqual(after.freezes, {}, 'clear-freeze path')
})
