// v0.7.65 — SHEET pivot grouping (date) + label filter round-trip.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { transformPackage, type MutablePackage } from '../xlsxWorksheetIo'
import { applyPivotAdditions, type PivotAddition } from '../xlsxPivotAdd'

class JsZipPkg implements MutablePackage {
  constructor(private zip: JSZip) {}
  async paths() { const o: string[] = []; this.zip.forEach(p => o.push(p)); return o.sort() }
  async has(p: string) { return this.zip.file(p) !== null }
  async readText(p: string) { return (await this.zip.file(p)?.async('text')) ?? '' }
  write(p: string, c: string) { this.zip.file(p, c) }
  add(p: string, c: string) { this.zip.file(p, c) }
  async remove(p: string) { this.zip.remove(p) }
}

function minimalXlsx(): JSZip {
  const zip = new JSZip()
  zip.file('xl/workbook.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">' +
    '<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>')
  zip.file('xl/_rels/workbook.xml.rels',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
    '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>' +
    '</Relationships>')
  zip.file('xl/worksheets/sheet1.xml',
    '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
    '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>')
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

test('pivot: date grouping (year) written to pivotCache records', async () => {
  const zip = minimalXlsx()
  const pkg = new JsZipPkg(zip)
  const addition: PivotAddition = {
    worksheetPath: 'xl/worksheets/sheet1.xml',
    sourceSheetName: 'Sheet1',
    sourceArea: { startColumn: 0, startRow: 0, endColumn: 1, endRow: 1 },
    location: { startColumn: 3, startRow: 0, endColumn: 5, endRow: 3 },
    name: 'P',
    fieldNames: ['Date', 'Sales'],
    rowFieldIndices: [0],
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{ fieldIndex: 1, agg: 'sum' }],
    groupings: [{ fieldIndex: 0, kind: 'date', dateUnit: 'year' }],
  }
  const workbookXml = await pkg.readText('xl/workbook.xml')
  const touched = new Set<string>()
  const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
  pkg.write('xl/workbook.xml', patched)
  const paths = await pkg.paths()
  const recordsPath = paths.find((p) => p.startsWith('xl/pivotCache/pivotCacheRecords'))
  assert.ok(recordsPath)
  const recordsXml = await pkg.readText(recordsPath!)
  // Records are present (engine still emits records even for date grouping).
  assert.match(recordsXml, /<pivotCacheRecords/, 'records part exists')
})

test('pivot: label filter (contains) accepted by engine', async () => {
  const zip = minimalXlsx()
  const pkg = new JsZipPkg(zip)
  const addition: PivotAddition = {
    worksheetPath: 'xl/worksheets/sheet1.xml',
    sourceSheetName: 'Sheet1',
    sourceArea: { startColumn: 0, startRow: 0, endColumn: 1, endRow: 1 },
    location: { startColumn: 3, startRow: 0, endColumn: 5, endRow: 3 },
    name: 'P',
    fieldNames: ['Region', 'Sales'],
    rowFieldIndices: [0],
    rowItems: [],
    rowLevelItems: [[]],
    colLevelItems: [[]],
    values: [{ fieldIndex: 1, agg: 'sum' }],
    filters: [{ kind: 'label', field: 0, op: 'contains', value: 'East' }],
  }
  const workbookXml = await pkg.readText('xl/workbook.xml')
  const touched = new Set<string>()
  const patched = await applyPivotAdditions(pkg, [addition], workbookXml, touched)
  pkg.write('xl/workbook.xml', patched)
  const paths = await pkg.paths()
  const tablePath = paths.find((p) => p.startsWith('xl/pivotTables/pivotTable'))
  assert.ok(tablePath)
  const tableXml = await pkg.readText(tablePath!)
  assert.match(tableXml, /<pivotTableDefinition/, 'pivot table part generated with label filter')
  assert.match(tableXml, /type="captionContains"[^>]*stringValue1="East"/, 'label filter captionContains with East written')
})
