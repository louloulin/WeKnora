// v0.7.46 — transformPackage multi-file round-trip tests.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { saveXlsxBytes, openXlsx, newXlsxWorkbook } from '../xlsxAdapter'
import { transformPackage, transformWorkbook } from '../xlsxWorksheetIo'
import { applyHyperlinkEdits } from '../xlsxHyperlinks'
import { applySheetNotes } from '../xlsxNotes'
import { applyTableAdditions, type TableAddition } from '../xlsxTableAdd'

test('transformPackage: no-op returns same bytes', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const same = await transformPackage(bytes, async () => {})
  assert.equal(same, bytes, 'identity short-circuit')
})

test('transformPackage: write + read back', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    pkg.write('xl/custom.xml', '<custom/>')
  })
  const zip = await JSZip.loadAsync(next)
  const txt = await zip.file('xl/custom.xml')?.async('text')
  assert.equal(txt, '<custom/>')
})

test('transformPackage: add + remove', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    pkg.add('a.txt', 'A')
    pkg.add('b.txt', 'B')
    pkg.remove('a.txt')
  })
  const zip = await JSZip.loadAsync(next)
  assert.equal(zip.file('a.txt'), null, 'a.txt removed')
  assert.equal(await zip.file('b.txt')?.async('text'), 'B', 'b.txt kept')
})

test('transformPackage + applyHyperlinkEdits: worksheet + rels both written', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 'click', f: '0' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    const wsPath = 'xl/worksheets/sheet1.xml'
    const wsXml = await pkg.readText(wsPath)
    const patch = applyHyperlinkEdits(wsXml, null, [
      { row: 0, column: 0, target: 'https://example.com' },
    ])
    pkg.write(wsPath, patch.worksheetXml)
    if (patch.relsChanged && patch.relsXml !== null) {
      pkg.write('xl/worksheets/_rels/sheet1.xml.rels', patch.relsXml)
    }
  })
  const zip = await JSZip.loadAsync(next)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  const relsXml = await zip.file('xl/worksheets/_rels/sheet1.xml.rels')?.async('text') ?? ''
  assert.ok(wsXml.includes('<hyperlink'), 'worksheet has hyperlink')
  assert.ok(relsXml.includes('example.com'), 'rels has URL')
})

test('transformPackage + applySheetNotes: notes + vml + rels + content_types', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    await applySheetNotes(
      pkg as any,
      'xl/worksheets/sheet1.xml',
      [{ row: 0, column: 0, author: 'Alice', text: 'Hello' }],
      new Set<string>(),
    )
  })
  const zip = await JSZip.loadAsync(next)
  assert.ok(await zip.file('xl/comments1.xml') !== null, 'comments part')
  assert.ok(await zip.file('xl/drawings/vmlDrawing1.vml') !== null, 'vml part')
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('<legacyDrawing'), 'legacyDrawing in worksheet')
  const relsXml = await zip.file('xl/worksheets/_rels/sheet1.xml.rels')?.async('text') ?? ''
  assert.ok(relsXml.includes('comments1.xml'), 'comments rel')
})

test('transformPackage + applyTableAdditions: table part + content_types', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [
    [{ v: 'A1', f: '0' }, { v: 'B1', f: '0' }, { v: 'C1', f: '0' }],
    [{ v: 1, f: '1' }, { v: 2, f: '2' }, { v: 3, f: '3' }],
  ] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    const additions: TableAddition[] = [{
      worksheetPath: 'xl/worksheets/sheet1.xml',
      area: { startRow: 0, startColumn: 0, endRow: 1, endColumn: 2 },
      name: 'T',
      columnNames: ['A', 'B', 'C'],
      bandedRows: true,
    }]
    await applyTableAdditions(pkg as any, additions, new Set<string>())
  })
  const zip = await JSZip.loadAsync(next)
  const paths: string[] = []
  zip.forEach((p) => paths.push(p))
  const tablePath = paths.find((p) => p.startsWith('xl/tables/table') && p.endsWith('.xml'))
  assert.ok(tablePath, 'table part created: ' + tablePath)
  const wsXml = await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? ''
  assert.ok(wsXml.includes('<tableParts'), 'tableParts element')
  const ctXml = await zip.file('[Content_Types].xml')?.async('text') ?? ''
  assert.ok(ctXml.includes('table'), 'content type override')
})

test('transformWorkbook + transformPackage composability', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }]] }]
  const bytes = await saveXlsxBytes(wb)
  // First transformWorkbook (single-file), then transformPackage (multi-file)
  const step1 = await transformWorkbook(bytes, {
    S1: (xml) => xml.replace('S1', 'S1') + '<custom/>',
  })
  assert.notEqual(step1, bytes, 'step1 changed bytes')
  const step2 = await transformPackage(step1, async (pkg) => {
    pkg.add('extra.txt', 'hello')
  })
  const zip = await JSZip.loadAsync(step2)
  assert.equal(await zip.file('extra.txt')?.async('text'), 'hello')
  assert.ok((await zip.file('xl/worksheets/sheet1.xml')?.async('text') ?? '').includes('<custom/>'))
})

test('openXlsx: round-trip works after transformPackage', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [{ name: 'S1', rows: [[{ v: 1, f: '1' }], [{ v: 2, f: '2' }]] }]
  const bytes = await saveXlsxBytes(wb)
  const next = await transformPackage(bytes, async (pkg) => {
    pkg.add('custom-marker.xml', '<marker/>')
  })
  const wb2 = await openXlsx(next)
  assert.equal(wb2.sheets.length, 1)
  assert.equal(wb2.sheets[0].name, 'S1')
})
