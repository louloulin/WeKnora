/**
 * xlsxPivotApply.test.ts — v0.7.110.1
 *
 * Mirrors the runtime path the 「应用」 button takes in
 * CollabSheetEditor.onPivotApply:
 *   1. build a workbook from raw rows via newXlsxWorkbook + buildCell
 *   2. saveXlsxBytes -> Uint8Array (.xlsx)
 *   3. inspectXlsx -> worksheetPath
 *   4. extractPivotSeed + buildPivotAddition
 *   5. transformPackage -> applyPivotAdditions
 *   6. unzip result -> assert pivot parts exist and workbook.xml has
 *      <pivotCaches> entry
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { newXlsxWorkbook, saveXlsxBytes, type XlsxAdapterCell } from '../xlsxAdapter'
import { extractPivotSeed, buildPivotAddition } from '../xlsxPivotFieldExtract'
import { transformPackage } from '../xlsxWorksheetIo'

const buildCell = (s: string): XlsxAdapterCell => ({ v: s })

async function unzip(buf: Uint8Array): Promise<Map<string, string>> {
  const zip = await JSZip.loadAsync(buf)
  const out = new Map<string, string>()
  const entries: string[] = []
  zip.forEach((p) => entries.push(p))
  for (const path of entries) {
    const file = zip.file(path)
    if (!file) continue
    out.set(path, await file.async('text'))
  }
  return out
}

test('onPivotApply pipeline writes pivot table parts + workbook pivotCaches', async () => {
  // 1. workbook with header row + 3 data rows: Category, Region, Sales
  const wb = newXlsxWorkbook()
  wb.sheets = [
    {
      name: 'Sheet1',
      rows: [
        [buildCell('Category'), buildCell('Region'), buildCell('Sales')],
        [buildCell('A'), buildCell('East'), buildCell('10')],
        [buildCell('A'), buildCell('West'), buildCell('20')],
        [buildCell('B'), buildCell('East'), buildCell('30')],
        [buildCell('B'), buildCell('West'), buildCell('40')],
      ].map((r) => r.map(buildCell)),
    },
  ]
  const bytes = await saveXlsxBytes(wb)

  // 2. inspect worksheet path
  const inspectOut = await transformPackage(bytes, async (pkg) => {
    const workbookXml = await pkg.readText('xl/workbook.xml')
    const extract = extractPivotSeed({
      rows: wb.sheets![0]!.rows.map((r) => r.map((c) => String((c as any).v ?? ''))),
      rowDimIdx: 0,
      valueIdxIdx: 2,
      agg: 'sum',
    })
    const addition: any = buildPivotAddition({
      extract,
      sourceSheetName: 'Sheet1',
      worksheetPath: 'xl/worksheets/sheet1.xml',
      targetRi: 8,
      targetCi: 1,
      agg: 'sum',
      rowDimIdx: 0,
      valueIdxIdx: 2,
    })
    const { applyPivotAdditions } = await import('../xlsxPivotAdd')
    const patched = await applyPivotAdditions(pkg, [addition], workbookXml, new Set<string>())
    pkg.write('xl/workbook.xml', patched)
  })

  // 3. unzip and check pivot parts
  const parts = await unzip(inspectOut)
  const pivots = [...parts.keys()].filter((p) => p.startsWith('xl/pivotTables/pivotTable'))
  const caches = [...parts.keys()].filter((p) => p.startsWith('xl/pivotCache/pivotCacheDefinition'))
  const records = [...parts.keys()].filter((p) => p.startsWith('xl/pivotCache/pivotCacheRecords'))
  assert.ok(pivots.length >= 1, 'at least one pivotTable part')
  assert.ok(caches.length >= 1, 'at least one pivotCacheDefinition part')
  assert.ok(records.length >= 1, 'at least one pivotCacheRecords part')
  // 4. workbook.xml registers pivotCaches
  const workbookXml = parts.get('xl/workbook.xml') ?? ''
  assert.match(workbookXml, /<pivotCaches>/, 'workbook.xml declares pivotCaches')
  assert.match(workbookXml, /pivotCache/, 'workbook.xml references pivot cache rId')
})

test('applyPivotAdditions rejects duplicate pivot name', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets = [
    {
      name: 'Sheet1',
      rows: [
        [buildCell('A'), buildCell('1')],
        [buildCell('A'), buildCell('2')],
      ].map((r) => r.map(buildCell)),
    },
  ]
  const bytes = await saveXlsxBytes(wb)
  const extract = extractPivotSeed({
    rows: [['A', '1'], ['A', '2']],
    rowDimIdx: 0,
    valueIdxIdx: 1,
    agg: 'sum',
  })
  const addition = buildPivotAddition({
    extract,
    sourceSheetName: 'Sheet1',
    worksheetPath: 'xl/worksheets/sheet1.xml',
    targetRi: 5,
    targetCi: 1,
    agg: 'sum',
    rowDimIdx: 0,
    valueIdxIdx: 1,
  })
  const { applyPivotAdditions } = await import('../xlsxPivotAdd')
  await assert.rejects(
    async () => {
      await transformPackage(bytes, async (pkg) => {
        const workbookXml = await pkg.readText('xl/workbook.xml')
        await applyPivotAdditions(pkg, [addition, addition], workbookXml, new Set<string>())
      })
    },
    /already taken|Pivot/,
    'second addition with same name must throw',
  )
})
