// v0.7.44 — vendored xlsxStructure + xlsxSheets + xlsxPageSetup tests.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  shiftFormulaText,
  qualifierMatches,
  FORMULA_REFERENCE_PATTERN,
} from '../xlsxStructure'
import {
  parseSheetElements,
  validateSheetName,
  formatSheetQualifier,
  renameSheetInFormula,
  formulaReferencesSheet,
  renameSheetReferencesInWorksheet,
  buildWorksheetPartXml,
  SheetEditError,
} from '../xlsxSheets'
import {
  applyPageSetupState,
  applyPrintAreas,
  buildHeaderFooterXml,
  PageSetupError,
  type SheetPageSetupState,
  type HeaderFooterParts,
} from '../xlsxPageSetup'

// ===================== xlsxStructure =====================

test('xlsxStructure: FORMULA_REFERENCE_PATTERN matches cell + range + qualified', () => {
  assert.ok('A1'.match(FORMULA_REFERENCE_PATTERN), 'A1 matches')
  assert.ok('A1:B3'.match(FORMULA_REFERENCE_PATTERN), 'range matches')
  assert.ok("'Sheet 1'!A1:B3".match(FORMULA_REFERENCE_PATTERN), 'qualified matches')
  assert.ok('Sheet1!A1'.match(FORMULA_REFERENCE_PATTERN), 'unqualified-sheet matches')
  // The regex needs a lead character (start of string or non-identifier char).
  assert.equal(!!'123'.match(FORMULA_REFERENCE_PATTERN), false, '123 should not match by itself')
})

test('xlsxStructure: qualifierMatches handles quoted + unquoted', () => {
  assert.equal(qualifierMatches("'Sheet 1'", 'Sheet 1'), true)
  assert.equal(qualifierMatches("Sheet1", 'Sheet 1'), false)
  assert.equal(qualifierMatches("'Sheet1'", 'Sheet 1'), false)
})

test('xlsxStructure: shiftFormulaText updates same-sheet refs', () => {
  // LinearShift: insert 1 row at index 0 (above row 0) — all rows shift down by 1.
  const shifted = shiftFormulaText(
    'A1+B1',
    'Sheet1',
    { boundary: 0, delta: 1, deleted: null },
    'row',
  )
  assert.equal(shifted, 'A2+B2')
})

test('xlsxStructure: shiftFormulaText leaves non-overlapping refs alone', () => {
  // shift ABOVE row 50 → A1 unaffected
  const shifted = shiftFormulaText(
    'A1',
    'Sheet1',
    { boundary: 50, delta: 1, deleted: null },
    'row',
  )
  assert.equal(shifted, 'A1')
})

test('xlsxStructure: shiftFormulaText skips string literals', () => {
  const shifted = shiftFormulaText(
    '"A1"&A1',
    'Sheet1',
    { boundary: 0, delta: 1, deleted: null },
    'row',
  )
  assert.equal(shifted, '"A1"&A2')
})

// ===================== xlsxSheets =====================

test('xlsxSheets: validateSheetName accepts plain', () => {
  assert.doesNotThrow(() => validateSheetName('Sheet1'))
})

test('xlsxSheets: validateSheetName rejects /\\?*[]:', () => {
  for (const ch of ['/', '\\', '?', '*', '[', ']', ':']) {
    assert.throws(() => validateSheetName(`Bad${ch}Name`), SheetEditError, `should reject ${ch}`)
  }
})

test('xlsxSheets: validateSheetName rejects empty', () => {
  assert.throws(() => validateSheetName(''), SheetEditError)
})

test('xlsxSheets: formatSheetQualifier quotes only when needed', () => {
  assert.equal(formatSheetQualifier('Sheet1'), 'Sheet1')
  assert.equal(formatSheetQualifier('Sheet 1'), "'Sheet 1'")
  assert.equal(formatSheetQualifier("With'Quote"), "'With''Quote'")
})

test('xlsxSheets: renameSheetInFormula rewrites qualified range', () => {
  // Renamed has no spaces, so no quoting needed.
  assert.equal(
    renameSheetInFormula("SUM('Sheet 1'!A1:A3)", 'Sheet 1', 'Renamed'),
    "SUM(Renamed!A1:A3)",
  )
  // Single-quote range ref also gets rewritten.
  assert.equal(
    renameSheetInFormula("'Sheet 1'!A1:B2", 'Sheet 1', 'Renamed'),
    "Renamed!A1:B2",
  )
})

test('xlsxSheets: renameSheetInFormula leaves unqualified refs alone', () => {
  assert.equal(renameSheetInFormula('A1+B2', 'Sheet1', 'Renamed'), 'A1+B2')
})

test('xlsxSheets: formulaReferencesSheet detects + ignores cross', () => {
  assert.equal(formulaReferencesSheet('Sheet1!A1', 'Sheet1'), true)
  assert.equal(formulaReferencesSheet('Sheet2!A1', 'Sheet1'), false)
  assert.equal(formulaReferencesSheet('A1', 'Sheet1'), false)
})

test('xlsxSheets: renameSheetReferencesInWorksheet rewrites c.r + formula', () => {
  const xml = '<c r="Sheet1!A1"><f>SUM(Sheet1!A1:A3)</f></c>'
  const out = renameSheetReferencesInWorksheet(xml, 'Sheet1', 'Renamed')
  assert.ok(out.includes('Renamed!A1'), 'cell ref rewritten')
  assert.ok(out.includes('SUM(Renamed!A1:A3)'), 'formula rewritten')
})

test('xlsxSheets: parseSheetElements returns name + hidden + relId', () => {
  const workbookXml = `
    <workbook>
      <sheets>
        <sheet name="Sheet1" sheetId="1" state="visible" r:id="rId1"/>
        <sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/>
        <sheet name="VeryHidden" sheetId="3" state="veryHidden" r:id="rId3"/>
      </sheets>
    </workbook>`
  const sheets = parseSheetElements(workbookXml)
  assert.equal(sheets.length, 3)
  assert.equal(sheets[0].name, 'Sheet1')
  assert.equal(sheets[0].hidden, false)
  assert.equal(sheets[1].name, 'Hidden')
  assert.equal(sheets[1].hidden, true)
  assert.equal(sheets[2].hidden, true)
})

test('xlsxSheets: buildWorksheetPartXml returns blank template', () => {
  const xml = buildWorksheetPartXml()
  assert.ok(xml.startsWith('<?xml'))
  assert.ok(xml.includes('<sheetData/>'))
  assert.ok(xml.includes('xmlns:r='))
})

// ===================== xlsxPageSetup =====================

test('xlsxPageSetup: applyPageSetupState sets orientation + paperSize', () => {
  const wsXml = '<worksheet><sheetData/></worksheet>'
  const state: SheetPageSetupState = {
    sheetName: 'Sheet1',
    orientation: 'landscape',
    paperSize: 9,
  }
  const out = applyPageSetupState(wsXml, state)
  assert.ok(out.includes('orientation="landscape"'), 'orientation set')
  assert.ok(out.includes('paperSize="9"'), 'paperSize set')
})

test('xlsxPageSetup: applyPageSetupState sets fitToPage', () => {
  const wsXml = '<worksheet><sheetData/></worksheet>'
  const out = applyPageSetupState(wsXml, {
    sheetName: 'Sheet1',
    fitToPage: true,
    fitToWidth: 1,
    fitToHeight: 2,
  })
  // sheetPr with pageSetUpPr fitToPage="1" is emitted when fitToPage is true.
  assert.ok(out.includes('<sheetPr') || out.includes('<pageSetup'), 'page setup related element present')
  assert.ok(out.includes('fitToPage="1"'))
  assert.ok(out.includes('fitToHeight="2"'))
})

test('xlsxPageSetup: applyPageSetupState applies margins', () => {
  const wsXml = '<worksheet><sheetData/></worksheet>'
  const out = applyPageSetupState(wsXml, { sheetName: 'Sheet1', margins: 'narrow' })
  assert.ok(out.includes('<pageMargins'), 'pageMargins present')
  assert.ok(out.includes('left="0.25"'), 'narrow left margin')
})

test('xlsxPageSetup: applyPageSetupState toggles printGridlines', () => {
  const wsXml = '<worksheet><sheetData/></worksheet>'
  const on = applyPageSetupState(wsXml, { sheetName: 'Sheet1', printGridlines: true })
  assert.ok(on.includes('<printOptions'), 'printOptions present')
  assert.ok(on.includes('gridLines="1"'))
  const off = applyPageSetupState(on, { sheetName: 'Sheet1', printGridlines: false })
  assert.ok(!off.includes('gridLines="1"'), 'gridLines off')
})

test('xlsxPageSetup: applyPageSetupState preserves untouched attributes', () => {
  const wsXml = '<worksheet><dimension ref="A1"/><sheetData/></worksheet>'
  const out = applyPageSetupState(wsXml, { sheetName: 'Sheet1', orientation: 'landscape' })
  assert.ok(out.includes('ref="A1"'), 'dimension preserved')
})

test('xlsxPageSetup: buildHeaderFooterXml emits oddHeader / oddFooter', () => {
  const header: HeaderFooterParts = { left: '&LTitle', center: '&CPage &P', right: '&R&D' }
  const xml = buildHeaderFooterXml(header, null)
  assert.ok(xml.startsWith('<headerFooter>'))
  assert.ok(xml.includes('<oddHeader>'))
  assert.ok(xml.includes('&amp;LTitle'), 'left encoded')
  assert.ok(xml.includes('&amp;CPage &amp;P'), 'center encoded')
  assert.ok(xml.includes('&amp;R&amp;D'), 'right encoded')
})

test('xlsxPageSetup: buildHeaderFooterXml with null + null returns empty string', () => {
  // All-empty input → emit nothing (caller decides whether to clear element).
  assert.equal(buildHeaderFooterXml(null, null), '')
})

test('xlsxPageSetup: applyPageSetupState inserts header/footer', () => {
  const wsXml = '<worksheet><sheetData/></worksheet>'
  const out = applyPageSetupState(wsXml, {
    sheetName: 'Sheet1',
    header: { center: '&CHello' },
    footer: { right: '&RPage &P' },
  })
  assert.ok(out.includes('&amp;CHello'), 'header center text in')
  assert.ok(out.includes('&amp;RPage &amp;P'), 'footer right text in')
})

test('xlsxPageSetup: applyPrintAreas adds _xlnm.Print_Area defined name', () => {
  const workbookXml = `<workbook><sheets><sheet name="Sheet1"/></sheets><definedNames><definedName name="_xlnm._FilterDatabase" localSheetId="0">'Sheet1'!A1:A10</definedName></definedNames></workbook>`
  const out = applyPrintAreas(workbookXml, [{ sheetName: 'Sheet1', printArea: 'A1:F20' }])
  assert.ok(out.includes('Print_Area'), 'Print_Area added')
  assert.ok(out.includes("'Sheet1'!$A$1:$F$20"), 'print area absolute refs')
  // Unrelated defined name preserved
  assert.ok(out.includes('_FilterDatabase'), 'other defined name preserved')
})

test('xlsxPageSetup: applyPrintAreas removes when null', () => {
  const workbookXml = `<workbook><sheets><sheet name="Sheet1"/></sheets><definedNames><definedName name="_xlnm.Print_Area" localSheetId="0">'Sheet1'!$A$1:$F$20</definedName></definedNames></workbook>`
  const out = applyPrintAreas(workbookXml, [{ sheetName: 'Sheet1', printArea: null }])
  assert.ok(!out.includes('Print_Area'), 'Print_Area removed')
})

test('xlsxPageSetup: applyPrintAreas throws for unknown sheet', () => {
  const workbookXml = `<workbook><sheets><sheet name="Sheet1"/></sheets></workbook>`
  assert.throws(
    () => applyPrintAreas(workbookXml, [{ sheetName: 'NoSuch', printArea: 'A1:B2' }]),
    PageSetupError,
  )
})
