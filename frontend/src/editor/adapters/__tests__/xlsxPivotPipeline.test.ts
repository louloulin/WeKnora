// v0.7.57 — pivot additions through the transformPackage pipeline.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { transformPackage, type MutablePackage } from '../xlsxWorksheetIo'
import { applyPivotAdditions, type PivotAddition } from '../xlsxPivotAdd'

class JsZipPkg implements MutablePackage {
  constructor(private zip: JSZip) {}
  async paths(): Promise<readonly string[]> {
    const out: string[] = []
    this.zip.forEach((p) => out.push(p))
    return out.sort()
  }
  async has(path: string): Promise<boolean> {
    return this.zip.file(path) !== null
  }
  async readText(path: string): Promise<string> {
    return (await this.zip.file(path)?.async('text')) ?? ''
  }
  write(path: string, content: string): void {
    this.zip.file(path, content)
  }
  add(path: string, content: string): void {
    this.zip.file(path, content)
  }
  async remove(path: string): Promise<void> {
    this.zip.remove(path)
  }
}

function minimalXlsx(): JSZip {
  const zip = new JSZip()
  zip.file(
    'xl/workbook.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ' +
      'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">' +
      '<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>',
  )
  zip.file(
    'xl/_rels/workbook.xml.rels',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
      '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>' +
      '</Relationships>',
  )
  zip.file(
    'xl/worksheets/sheet1.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">' +
      '<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1"><v>10</v></c></row>' +
      '<row r="2"><c r="A2" t="s"><v>0</v></c><c r="B2" t="s"><v>2</v></c><c r="C2"><v>20</v></c></row></sheetData>' +
      '</worksheet>',
  )
  zip.file(
    'xl/sharedStrings.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="3" uniqueCount="3">' +
      '<si><t>East</t></si><si><t>Q1</t></si><si><t>Q2</t></si></sst>',
  )
  zip.file(
    '[Content_Types].xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
      '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>' +
      '<Default Extension="xml" ContentType="application/xml"/>' +
      '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>' +
      '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>' +
      '</Types>',
  )
  return zip
}

test('applyPivotAdditions registers cache + table parts and workbook.xml', async () => {
  const zip = minimalXlsx()
  const pkg = new JsZipPkg(zip)
  const addition: PivotAddition = {
    worksheetPath: 'xl/worksheets/sheet1.xml',
    sourceSheetName: 'Sheet1',
    sourceArea: { startColumn: 0, startRow: 0, endColumn: 2, endRow: 1 },
    location: { startColumn: 5, startRow: 0, endColumn: 8, endRow: 5 },
    name: 'Pivot1',
    fieldNames: ['Region', 'Quarter', 'Sales'],
    rowFieldIndices: [0],
    columnFieldIndex: 1,
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{ fieldIndex: 2, agg: 'sum' }],
  }
  const workbookXml = await pkg.readText('xl/workbook.xml')
  const touched = new Set<string>()
  const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
  pkg.write('xl/workbook.xml', patched)

  const paths = await pkg.paths()
  assert.ok(paths.some((p) => p.startsWith('xl/pivotCache/pivotCacheDefinition')), 'cache definition part')
  assert.ok(paths.some((p) => p.startsWith('xl/pivotCache/pivotCacheRecords')), 'cache records part')
  assert.ok(paths.some((p) => p.startsWith('xl/pivotTables/pivotTable')), 'pivot table part')
  const wb = await pkg.readText('xl/workbook.xml')
  assert.match(wb, /<pivotCaches>/, 'workbook registers pivot cache')
  const wsRels = await pkg.readText('xl/worksheets/_rels/sheet1.xml.rels')
  assert.match(wsRels, /pivotTable/, 'worksheet rels link the pivot table')
})

test('pivot additions flow through transformPackage', async () => {
  const zip = minimalXlsx()
  const addition: PivotAddition = {
    worksheetPath: 'xl/worksheets/sheet1.xml',
    sourceSheetName: 'Sheet1',
    sourceArea: { startColumn: 0, startRow: 0, endColumn: 2, endRow: 1 },
    location: { startColumn: 5, startRow: 0, endColumn: 8, endRow: 5 },
    name: 'Pivot1',
    fieldNames: ['Region', 'Quarter', 'Sales'],
    rowFieldIndices: [0],
    columnFieldIndex: 1,
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{ fieldIndex: 2, agg: 'sum' }],
  }
  const bytes = await zip.generateAsync({ type: 'uint8array' })
  const out = await transformPackage(bytes, async (pkg) => {
    const workbookXml = await pkg.readText('xl/workbook.xml')
    const touched = new Set<string>()
    const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
    pkg.write('xl/workbook.xml', patched)
  })
  const outZip = await JSZip.loadAsync(out)
  assert.ok(outZip.file('xl/pivotTables/pivotTable1.xml'), 'pivot table part survives transformPackage')
  const wb = await outZip.file('xl/workbook.xml')?.async('string')
  assert.match(wb ?? '', /<pivotCaches>/, 'workbook.xml patched')
})
