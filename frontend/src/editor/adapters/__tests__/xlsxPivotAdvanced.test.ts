// v0.7.62 — SHEET pivot advanced options (numFmt / showDataAs) round-trip.

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
  async has(path: string): Promise<boolean> { return this.zip.file(path) !== null }
  async readText(path: string): Promise<string> {
    return (await this.zip.file(path)?.async('text')) ?? ''
  }
  write(path: string, content: string): void { this.zip.file(path, content) }
  add(path: string, content: string): void { this.zip.file(path, content) }
  async remove(path: string): Promise<void> { this.zip.remove(path) }
}

function minimalXlsx(): JSZip {
  const zip = new JSZip()
  zip.file('xl/workbook.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ' +
    'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">' +
    '<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>')
  zip.file('xl/_rels/workbook.xml.rels',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
    '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>' +
    '</Relationships>')
  zip.file('xl/worksheets/sheet1.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">' +
    '<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1"><v>10</v></c></row>' +
    '<row r="2"><c r="A1" t="s"><v>0</v></c><c r="C1"><v>20</v></c></row></sheetData>' +
    '</worksheet>')
  zip.file('[Content_Types].xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
    '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>' +
    '<Default Extension="xml" ContentType="application/xml"/>' +
    '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>' +
    '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>' +
    '</Types>')
  return zip
}

test('pivot: numFmt + showDataAs written to pivotTable dataField', async () => {
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
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{
      fieldIndex: 2,
      agg: 'sum',
      numFmt: '#,##0.00',
      showDataAs: 'percentOfTotal',
    }],
  }
  const workbookXml = await pkg.readText('xl/workbook.xml')
  const touched = new Set<string>()
  const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
  pkg.write('xl/workbook.xml', patched)

  const paths = await pkg.paths()
  const tablePath = paths.find((p) => p.startsWith('xl/pivotTables/pivotTable'))
  assert.ok(tablePath)
  const tableXml = await pkg.readText(tablePath!)
  assert.match(tableXml, /numFmtId="\d+"/, 'numFmtId attribute on dataField (mapped from numFmt string)')
  assert.match(tableXml, /showDataAs="percentOfTotal"/, 'showDataAs attribute on dataField')
})

test('pivot: aggregation=count + showDataAs=percentOfRow written', async () => {
  const zip = minimalXlsx()
  const pkg = new JsZipPkg(zip)
  const addition: PivotAddition = {
    worksheetPath: 'xl/worksheets/sheet1.xml',
    sourceSheetName: 'Sheet1',
    sourceArea: { startColumn: 0, startRow: 0, endColumn: 1, endRow: 1 },
    location: { startColumn: 3, startRow: 0, endColumn: 5, endRow: 3 },
    name: 'P2',
    fieldNames: ['Region', 'Qty'],
    rowFieldIndices: [0],
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{ fieldIndex: 1, agg: 'count', showDataAs: 'percentOfRow' }],
  }
  const workbookXml = await pkg.readText('xl/workbook.xml')
  const touched = new Set<string>()
  const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
  pkg.write('xl/workbook.xml', patched)
  const paths = await pkg.paths()
  const tablePath = paths.find((p) => p.startsWith('xl/pivotTables/pivotTable'))
  assert.ok(tablePath)
  const tableXml = await pkg.readText(tablePath!)
  assert.match(tableXml, /subtotal="count"/, 'count aggregation subtotal attribute')
  assert.match(tableXml, /showDataAs="percentOfRow"/, 'showDataAs=percentOfRow')
})
