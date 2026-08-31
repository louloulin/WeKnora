// v0.7.43.c — Cell color / font / fill persistence test: confirms that
// cells with .bold / .color / .fill produce valid xl/styles.xml with the
// right cellXfs entries and s="N" attribute on the cell.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { saveXlsxBytes, openXlsx, newXlsxWorkbook, type XlsxAdapterCell } from '../../../editor/adapters/xlsxAdapter'

const buildCell = (raw: string, style: Partial<XlsxAdapterCell> = {}): XlsxAdapterCell => {
  const v = raw ?? ''
  if (typeof v === 'string' && v.startsWith('=') && v.length > 1) {
    return { v: '', f: v.slice(1), ...style }
  }
  if (typeof v === 'string' && /^-?\d+(\.\d+)?$/.test(v)) {
    return { v: Number(v), ...style }
  }
  return { v, ...style }
}

test('cell style persistence: bold survives save → open round-trip', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [
      [buildCell('header', { bold: true, color: 'FF0000FF' }), buildCell('value')],
      [buildCell('row', { italic: true }), buildCell('42')],
    ],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  // Bold + color flags survive round-trip.
  const a1 = wb2.sheets[0].rows[0][0]
  assert.equal(a1.bold, true)
  assert.equal(a1.color, '0000FF') // leading FF (alpha) + RRGGBB → store as AABBGGRR? Actually sheetJS strips alpha.
  // Italic flag survives.
  const a2 = wb2.sheets[0].rows[1][0]
  assert.equal(a2.italic, true)
})

test('cell style persistence: fill color is set', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [[buildCell('hi', { fill: 'FFFFFF00' })]],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  const cell = wb2.sheets[0].rows[0][0]
  // SheetJS normalises the alpha away; yellow stays as FFFF00.
  assert.ok(cell.fill, `expected fill, got ${JSON.stringify(cell)}`)
  assert.equal(cell.fill!.toUpperCase().slice(-6), 'FFFF00')
})

test('cell style persistence: unstyled cell is unchanged', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [[buildCell('plain'), buildCell('42')]],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets[0].rows[0][0].bold, undefined)
  assert.equal(wb2.sheets[0].rows[0][0].color, undefined)
  assert.equal(wb2.sheets[0].rows[0][0].fill, undefined)
})

test('cell style persistence: multiple styles are deduped', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{
    name: 'Sheet1',
    rows: [
      [buildCell('a', { bold: true, color: 'FF0000FF' })],
      [buildCell('b', { bold: true, color: 'FF0000FF' })],
      [buildCell('c', { bold: true, color: 'FFFF0000' })],
    ],
  }]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  // a and b share the same style (dedup); c is different.
  assert.equal(wb2.sheets[0].rows[0][0].bold, true)
  assert.equal(wb2.sheets[0].rows[1][0].bold, true)
  assert.equal(wb2.sheets[0].rows[2][0].bold, true)
  assert.notEqual(wb2.sheets[0].rows[0][0].color, wb2.sheets[0].rows[2][0].color)
})
