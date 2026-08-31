// v0.7.43 — formula persistence smoke test: confirms that the cell-value
// → formula-text pipeline (the =SUM(A1:B1) → cell.f = "SUM(A1:B1)")
// round-trips through saveXlsxBytes → openXlsx and that XLSX keeps the
// formula alongside the cached numeric result.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  saveXlsxBytes,
  openXlsx,
  newXlsxWorkbook,
  type XlsxAdapterCell,
} from '../../../editor/adapters/xlsxAdapter'

const buildCell = (raw: string): XlsxAdapterCell => {
  const v = raw ?? ''
  if (typeof v === 'string' && v.startsWith('=') && v.length > 1) {
    return { v: '', f: v.slice(1) }
  }
  if (typeof v === 'string' && /^-?\d+(\.\d+)?$/.test(v)) {
    return { v: Number(v) }
  }
  return { v }
}

test('formula persistence: SUM round-trips through saveXlsxBytes', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [
      [buildCell('10'), buildCell('20'), buildCell('=SUM(A1:B1)')],
      [buildCell('30'), buildCell('40'), buildCell('=AVERAGE(A2:B2)')],
    ],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  // The third column (ci=2) of row 0 should carry the formula text.
  const cell = wb2.sheets[0].rows[0][2]
  assert.ok(typeof cell.f === 'string', `expected formula, got ${JSON.stringify(cell)}`)
  assert.equal(cell.f, 'SUM(A1:B1)')
  // XLSX recomputes the cached result on open, so .v should be a number.
  assert.equal(typeof cell.v, 'number')
})

test('formula persistence: cross-sheet ref round-trips', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [
    {
      name: 'Sheet1',
      rows: [[buildCell('5')], [buildCell('10')], [buildCell('=SUM(Sheet2!A1:A2)')]],
    },
    { name: 'Sheet2', rows: [[buildCell('100')], [buildCell('200')]] },
  ]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets.length, 2)
  const cell = wb2.sheets[0].rows[2][0]
  assert.equal(cell.f, 'SUM(Sheet2!A1:A2)')
})

test('formula persistence: non-formula cells survive untouched', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [
      [buildCell('hello'), buildCell('42'), buildCell('3.14')],
    ],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  const row = wb2.sheets[0].rows[0]
  // SheetJS always returns a formatted display string when there's no
  // explicit format hint, so .v lands back as the formatted text.
  assert.equal(String(row[0].v), 'hello')
  assert.equal(Number(row[1].v), 42)
  assert.equal(Number(row[2].v), 3.14)
})
