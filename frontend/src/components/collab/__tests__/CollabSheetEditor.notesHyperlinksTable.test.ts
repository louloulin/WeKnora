// v0.7.45 — notes / hyperlinks / tables round-trip tests.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { saveXlsxBytes, openXlsx, newXlsxWorkbook } from '../../../editor/adapters/xlsxAdapter'
import { transformWorkbook } from '../../../editor/adapters/xlsxWorksheetIo'
import { applyHyperlinkEdits } from '../../../editor/adapters/xlsxHyperlinks'

test('hyperlinks: applyHyperlinkEdits writes <hyperlink> with r:id + rels', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [[{ v: 'click here', f: '0' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const zip = await JSZip.loadAsync(bytes)
  const sheetPath = 'xl/worksheets/sheet1.xml'
  const wsXml = await zip.file(sheetPath)?.async('text') ?? ''
  // Apply hyperlink to A1 → external URL
  const patch = applyHyperlinkEdits(wsXml, null, [
    { row: 0, column: 0, target: 'https://example.com' },
  ])
  assert.ok(patch.worksheetXml.includes('<hyperlink'), 'hyperlink element inserted')
  assert.ok(patch.worksheetXml.includes('r:id="rId'), 'r:id attribute present')
  assert.notEqual(patch.relsXml, null, 'rels XML returned')
  assert.ok(patch.relsXml!.includes('example.com'), 'target URL in rels')
  assert.ok(patch.relsChanged, 'rels marked changed')
})

test('hyperlinks: internal anchor uses location attribute (no rel)', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [[{ v: 'go there', f: '0' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const zip = await JSZip.loadAsync(bytes)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  const patch = applyHyperlinkEdits(wsXml, null, [
    { row: 0, column: 0, target: '#Sheet1!B5' },
  ])
  assert.ok(patch.worksheetXml.includes('location="Sheet1!B5"'), 'location attribute')
  assert.equal(patch.relsXml, null, 'no rels needed for internal anchor')
})

test('hyperlinks: target=null removes hyperlink', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [[{ v: 'x', f: '0' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const zip = await JSZip.loadAsync(bytes)
  let wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  // Add
  let patch = applyHyperlinkEdits(wsXml, null, [
    { row: 0, column: 0, target: 'https://foo.com' },
  ])
  wsXml = patch.worksheetXml
  const relsXml = patch.relsXml
  // Remove
  patch = applyHyperlinkEdits(wsXml, relsXml, [
    { row: 0, column: 0, target: null },
  ])
  assert.ok(!patch.worksheetXml.includes('<hyperlink'), 'hyperlink removed')
})

test('identity transform short-circuits even with empty transforms', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'Sheet1', rows: [[{ v: 'x', f: '0' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformWorkbook(bytes, {})
  assert.equal(next, bytes, 'empty transforms short-circuits')
})

test('multi-sheet workbook: identities all return same bytes', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [
    { name: 'Alpha', rows: [[{ v: 'a', f: '0' }]] },
    { name: 'Beta', rows: [[{ v: 'b', f: '0' }]] },
    { name: 'Gamma', rows: [[{ v: 'c', f: '0' }]] },
  ]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformWorkbook(bytes, {
    Alpha: (xml) => xml,
    Beta: (xml) => xml,
    Gamma: (xml) => xml,
  })
  assert.equal(next, bytes)
})
