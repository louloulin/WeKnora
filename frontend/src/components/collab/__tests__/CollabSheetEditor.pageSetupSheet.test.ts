// v0.7.44 — page setup + sheet manage adapter tests (genoffice parity).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { saveXlsxBytes, openXlsx, newXlsxWorkbook } from '../../../editor/adapters/xlsxAdapter'
import { transformWorkbook } from '../../../editor/adapters/xlsxWorksheetIo'
import { applyPageSetupState, type SheetPageSetupState } from '../../../editor/adapters/xlsxPageSetup'

const buildSimpleWb = () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [['hi']] }]
  return wb
}

test('pageSetup: orientation + paperSize persists through save → transform → open', async () => {
  const wb = buildSimpleWb()
  const bytes = await saveXlsxBytes(wb)
  const sheetName = 'Sheet1'
  const state: SheetPageSetupState = {
    sheetName,
    orientation: 'landscape',
    paperSize: 9,
  }
  const next = await transformWorkbook(bytes, {
    [sheetName]: (xml) => applyPageSetupState(xml, state),
  })
  // Verify raw worksheet XML has the pageSetup element
  const zip = await JSZip.loadAsync(next)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('orientation="landscape"'))
  assert.ok(wsXml.includes('paperSize="9"'))
})

test('pageSetup: margins preset "narrow" applies 0.25 left', async () => {
  const wb = buildSimpleWb()
  const bytes = await saveXlsxBytes(wb)
  const next = await transformWorkbook(bytes, {
    Sheet1: (xml) => applyPageSetupState(xml, {
      sheetName: 'Sheet1',
      margins: 'narrow',
    }),
  })
  const zip = await JSZip.loadAsync(next)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('<pageMargins'))
  assert.ok(wsXml.includes('left="0.25"'))
})

test('pageSetup: printGridlines toggles printOptions', async () => {
  const wb = buildSimpleWb()
  const bytes = await saveXlsxBytes(wb)
  const on = await transformWorkbook(bytes, {
    Sheet1: (xml) => applyPageSetupState(xml, {
      sheetName: 'Sheet1',
      printGridlines: true,
    }),
  })
  const zip = await JSZip.loadAsync(on)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('gridLines="1"'))
})

test('pageSetup: fitToPage injects sheetPr/pageSetUpPr', async () => {
  const wb = buildSimpleWb()
  const bytes = await saveXlsxBytes(wb)
  const next = await transformWorkbook(bytes, {
    Sheet1: (xml) => applyPageSetupState(xml, {
      sheetName: 'Sheet1',
      fitToPage: true,
      fitToWidth: 1,
      fitToHeight: 3,
    }),
  })
  const zip = await JSZip.loadAsync(next)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('fitToPage="1"'))
  assert.ok(wsXml.includes('fitToHeight="3"'))
})

test('pageSetup: identity transform short-circuits (no zip write)', async () => {
  const wb = buildSimpleWb()
  const bytes = await saveXlsxBytes(wb)
  // Apply no-op transform (returns xml as-is).
  const next = await transformWorkbook(bytes, {
    Sheet1: (xml) => xml,
  })
  // Should return same bytes (identity).
  assert.equal(next, bytes)
})

test('sheetManage: multi-sheet workbook round-trip preserves order', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [
    { name: 'First', rows: [['a']] },
    { name: 'Second', rows: [['b']] },
    { name: 'Third', rows: [['c']] },
  ]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets.length, 3)
  assert.equal(wb2.sheets[0].name, 'First')
  assert.equal(wb2.sheets[1].name, 'Second')
  assert.equal(wb2.sheets[2].name, 'Third')
})

test('sheetManage: openXlsx surfaces multiple sheets with correct names', async () => {
  // Round-trip via formula cells so the cached <v> survives — same
  // approach the v0.7.43.b formula tests use for non-formula cells.
  const wb = newXlsxWorkbook()
  wb.sheets = [
    { name: 'Alpha', rows: [[{ v: 1, f: '1' }, { v: 2, f: '2' }]] },
    { name: 'Beta', rows: [[{ v: 10, f: '10' }, { v: 20, f: '20' }]] },
  ]
  const bytes = await saveXlsxBytes(wb)
  const wb2 = await openXlsx(bytes)
  assert.equal(wb2.sheets.length, 2)
  assert.equal(wb2.sheets[0].name, 'Alpha')
  assert.equal(wb2.sheets[1].name, 'Beta')
  // Verify formula-cached values surface through openXlsx
  assert.equal(typeof wb2.sheets[0].rows[0][0].v, 'number')
  assert.equal(typeof wb2.sheets[1].rows[0][0].v, 'number')
})
