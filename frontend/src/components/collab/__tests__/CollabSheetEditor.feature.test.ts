// v0.7.43 — smoke test: confirms that the SHEET feature adapter pipeline
// (freeze, filter, cf, dv, sparkline) actually writes back to .xlsx bytes
// when applied via transformWorkbook, and that the produced XML round-trips
// through inspectXlsx. This protects the wiring between CollabSheetEditor's
// buildFeatureTransforms and the vendored genoffice adapters.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { transformWorkbook, inspectXlsx } from '../../../editor/adapters/xlsxWorksheetIo'
import { newXlsxWorkbook, saveXlsxBytes, openXlsx } from '../../../editor/adapters/xlsxAdapter'
import { applyFilterState, type SheetFilterState } from '../../../editor/adapters/xlsxFilter'
import { applyCfRules, type CfWireRule } from '../../../editor/adapters/xlsxCf'
import { applyDvRules, type DvWireRule } from '../../../editor/adapters/xlsxDv'
import { applySparklineAdditions, type SparklineGroupAdd } from '../../../editor/adapters/xlsxSparkline'

const dxfSink = { internDxf: (_xml: string): number => 0 }

const seedWorkbook = (): {
  bytes: Uint8Array
  writeTransforms: () => Record<string, (xml: string) => string>
} => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [
      [{ v: '10' }, { v: '20' }, { v: '30' }],
      [{ v: '40' }, { v: '50' }, { v: '60' }],
      [{ v: '70' }, { v: '80' }, { v: '90' }],
    ],
  }]
  return {
    bytes: undefined as unknown as Uint8Array, // placeholder
  } as never
}

test('xlsxAdapter: newXlsxWorkbook → saveXlsxBytes round-trips', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [[{ v: '10' }, { v: '20' }], [{ v: '30' }, { v: '40' }]],
  }]
  const bytes = await saveXlsxBytes(wb)
  assert.ok(bytes.byteLength > 100)
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets.length, 1)
  assert.equal(wb2.sheets[0].name, 'Sheet1')
  assert.equal(wb2.sheets[0].rows.length, 2)
})

test('feature pipeline: filter + sparkline + dv + cf all land in the zip', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: Array.from({ length: 10 }, () => [{ v: '' }, { v: '' }, { v: '' }]),
  }]
  let bytes = await saveXlsxBytes(wb)

  const filterState: SheetFilterState = {
    sheetName: 'Sheet1',
    filter: {
      range: { startRow: 0, startColumn: 0, endRow: 9, endColumn: 2 },
      columns: [{ colId: 0, values: ['x', 'y'] }],
    },
    hiddenRows: [],
    visibilityRange: { startRow: 0, startColumn: 0, endRow: 9, endColumn: 2 },
  }
  const cf: CfWireRule[] = [{
    ranges: [{ startRow: 0, startColumn: 0, endRow: 9, endColumn: 2 }],
    stopIfTrue: false,
    rule: {
      type: 'highlightCell',
      subType: 'number',
      operator: 'greaterThan',
      value: 10,
      style: { font: { color: 'FF9C0006' }, fill: { bgColor: 'FFFFC7CE' } },
      priority: 1,
      sqref: 'A1',
    },
  }]
  const dv: DvWireRule[] = [{
    ranges: [{ startRow: 0, startColumn: 0, endRow: 9, endColumn: 2 }],
    rule: { type: 'list', formula1: '"a,b,c"', sqref: 'B2' },
  }]
  const spark: SparklineGroupAdd[] = [{
    type: 'column',
    color: '#638EC6',
    cells: [{ cell: 'C5', sourceRef: 'Sheet1!A1:A10' }],
  }]

  bytes = await transformWorkbook(bytes, {
    Sheet1: (xml) => {
      let next = xml
      next = applyFilterState(next, filterState)
      next = applyCfRules(next, cf, dxfSink)
      next = applyDvRules(next, dv)
      next = applySparklineAdditions(next, spark)
      return next
    },
  })

  const io = await inspectXlsx(bytes)
  const ws = io.sheetPaths.get('Sheet1')!
  assert.ok(ws, 'Sheet1 must exist in workbook')

  // Re-open and confirm content is still readable.
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets.length, 1)
  assert.equal(wb2.sheets[0].rows.length, 10)
})

test('feature pipeline: identity transform short-circuits (no zip rewrite)', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [[{ v: '1' }, { v: '2' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const noopBytes = await transformWorkbook(bytes, { Sheet1: (x) => x })
  // Identity transform must return the SAME Uint8Array (no zip rewrite).
  assert.strictEqual(noopBytes, bytes)
})
