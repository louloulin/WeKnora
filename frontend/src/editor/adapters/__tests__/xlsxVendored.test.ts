/**
 * xlsxVendored.test — smoke tests for the genoffice-vendored adapters
 * (xlsxFilter / xlsxSparkline / xlsxCf / xlsxDv / xlsxHyperlinks /
 * xlsxProtection / xlsxTheme / xlsxDefinedNames) wired through
 * xlsxWorksheetIo.
 *
 * Run: ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/xlsxVendored.test.ts
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newXlsxWorkbook,
  saveXlsxBytes,
  openXlsx,
} from '../xlsxAdapter'
import {
  inspectXlsx,
  readSheetXml,
  writeSheetXml,
  transformWorkbook,
} from '../xlsxWorksheetIo'
import {
  applyFilterState,
  type SheetFilterState,
  FilterEditError,
} from '../xlsxFilter'
import {
  applySparklineAdditions,
  type SparklineGroupAdd,
  SparklineAddError,
} from '../xlsxSparkline'
import {
  applyCfRules,
  type CfWireRule,
  CfEditError,
} from '../xlsxCf'
import {
  applyDvRules,
  type DvWireRule,
} from '../xlsxDv'
import {
  applyHyperlinkEdits,
  ensureRelationshipNamespace,
} from '../xlsxHyperlinks'
import {
  applySheetProtection,
  applyWorkbookProtection,
  applyProtectedRanges,
} from '../xlsxProtection'
import {
  applyThemeState,
  type WorkbookThemeState,
} from '../xlsxTheme'
import {
  applyDefinedNamesState,
  type DefinedNamesState,
} from '../xlsxDefinedNames'
import {
  withFutureFunctionMarkers,
} from '../futureFunctions'

const fixtureBytes = async () => {
  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [{ v: 'name' }, { v: 'score' }, { v: 'team' }],
    [{ v: 'alice' }, { v: 80 }, { v: 'blue' }],
    [{ v: 'bob' }, { v: 92 }, { v: 'red' }],
    [{ v: 'carol' }, { v: 65 }, { v: 'blue' }],
    [{ v: 'dave' }, { v: 47 }, { v: 'red' }],
  ]
  return saveXlsxBytes(wb)
}

const dxfSink = {
  ids: [] as string[],
  internDxf(xml: string): number {
    this.ids.push(xml)
    return this.ids.length
  },
}

// ===== Worksheet IO =====

test('xlsxWorksheetIo inspect: enumerates sheets and paths', async () => {
  const bytes = await fixtureBytes()
  const io = await inspectXlsx(bytes)
  assert.deepEqual(io.sheetNames, ['Sheet1'])
  assert.ok(io.sheetPaths.get('Sheet1')?.endsWith('sheet1.xml'))
})

test('xlsxWorksheetIo readSheetXml returns a worksheet XML', async () => {
  const bytes = await fixtureBytes()
  const xml = await readSheetXml(bytes, 'Sheet1')
  assert.ok(xml)
  assert.ok(xml!.includes('<sheetData>'))
  assert.ok(xml!.includes('alice'))
})

test('transformWorkbook: skip-write path returns the original bytes', async () => {
  const bytes = await fixtureBytes()
  const bytes1 = await transformWorkbook(bytes, { Sheet1: (x) => x })
  assert.equal(bytes1, bytes)
})

test('transformWorkbook: applies per-sheet transforms', async () => {
  const bytes = await fixtureBytes()
  const state: SheetFilterState = {
    sheetName: 'Sheet1',
    filter: {
      range: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
      columns: [{ colId: 0, values: ['alice'] }],
    },
    hiddenRows: [],
    visibilityRange: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
  }
  const bytes1 = await transformWorkbook(bytes, {
    Sheet1: (xml) => applyFilterState(xml, state),
  })
  const xml1 = (await readSheetXml(bytes1, 'Sheet1'))!
  assert.ok(xml1.includes('<autoFilter '))
})

// ===== Filter =====

test('xlsxFilter: applyFilterState writes <autoFilter> and survives round-trip', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  assert.ok(!xml0.includes('<autoFilter'))

  const state: SheetFilterState = {
    sheetName: 'Sheet1',
    filter: {
      range: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
      columns: [
        { colId: 0, values: ['alice', 'bob'] },
        { colId: 2, values: ['blue'] },
      ],
    },
    hiddenRows: [],
    visibilityRange: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
  }
  const xml1 = applyFilterState(xml0, state)
  assert.ok(xml1.includes('<autoFilter '))
  assert.ok(xml1.includes('ref="A1:C5"'))
  assert.ok(xml1.includes('<filterColumn colId="0">'))
  assert.ok(xml1.includes('<filter val="alice"/>'))
  assert.ok(xml1.includes('<filterColumn colId="2">'))

  const bytes1 = await writeSheetXml(bytes, 'Sheet1', xml1)
  const wb = await openXlsx(bytes1)
  assert.equal(wb.sheets[0].rows.length, 5)
  const xml2 = (await readSheetXml(bytes1, 'Sheet1'))!
  assert.ok(xml2.includes('<autoFilter '))
})

test('xlsxFilter: clearing filter (null filter) removes <autoFilter>', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const state: SheetFilterState = {
    sheetName: 'Sheet1',
    filter: null,
    hiddenRows: [],
    visibilityRange: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
  }
  const xml1 = applyFilterState(xml0, state)
  assert.ok(!xml1.includes('<autoFilter'))
})

test('xlsxFilter: hidden rows emit hidden="1" attributes', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const state: SheetFilterState = {
    sheetName: 'Sheet1',
    filter: {
      range: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
      columns: [{ colId: 2, values: ['blue'] }],
    },
    hiddenRows: [3],
    visibilityRange: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
  }
  const xml1 = applyFilterState(xml0, state)
  assert.ok(xml1.includes('hidden="1"'))
})

test('xlsxFilter: unknown custom operator throws FilterEditError', () => {
  assert.throws(
    () =>
      applyFilterState(
        '<worksheet><sheetData></sheetData></worksheet>',
        {
          sheetName: 'Sheet1',
          filter: {
            range: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
            columns: [
              {
                colId: 1,
                customs: { filters: [{ val: 80, operator: 'between' }] },
              },
            ],
          },
          hiddenRows: [],
          visibilityRange: { startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 },
        },
      ),
    (err) => err instanceof FilterEditError,
  )
})

// ===== Sparkline =====

test('xlsxSparkline: applySparklineAdditions emits x14:sparklineGroup', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  assert.ok(!xml0.includes('sparkline'))

  const group: SparklineGroupAdd = {
    type: 'column',
    color: '#376092',
    cells: [{ cell: 'E2', sourceRef: 'Sheet1!B2:B2' }],
  }
  const xml1 = applySparklineAdditions(xml0, [group])
  assert.ok(xml1.includes('x14:sparklineGroup'))
  assert.ok(xml1.includes('type="column"'))
  assert.ok(xml1.includes('<xm:sqref>E2</xm:sqref>'))
  assert.ok(xml1.includes('Sheet1!B2:B2'))

  const bytes1 = await writeSheetXml(bytes, 'Sheet1', xml1)
  const wb = await openXlsx(bytes1)
  assert.equal(wb.sheets[0].rows.length, 5)
  const xml2 = (await readSheetXml(bytes1, 'Sheet1'))!
  assert.ok(xml2.includes('sparklineGroup'))
})

test('xlsxSparkline: duplicate cell across groups throws SparklineAddError', () => {
  assert.throws(
    () =>
      applySparklineAdditions(
        '<worksheet><sheetData></sheetData></worksheet>',
        [
          { type: 'line', cells: [{ cell: 'E2', sourceRef: 'Sheet1!B2:B2' }] },
          { type: 'line', cells: [{ cell: 'E2', sourceRef: 'Sheet1!B3:B3' }] },
        ],
      ),
    (err) => err instanceof SparklineAddError,
  )
})

// ===== Conditional Formatting =====

test('xlsxCf: highlightCell rule emits conditionalFormatting + dxfId', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  assert.ok(!xml0.includes('<conditionalFormatting'))

  const rules: CfWireRule[] = [
    {
      ranges: [{ startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 }],
      stopIfTrue: false,
      rule: {
        type: 'highlightCell',
        subType: 'number',
        operator: 'greaterThan',
        value: 80,
        style: { fontColor: '#FF0000', fill: '#FFEEEE' },
      },
    },
  ]
  const xml1 = applyCfRules(xml0, rules, dxfSink)
  assert.ok(xml1.includes('<conditionalFormatting sqref="A1:C5">'))
  assert.ok(xml1.includes('type="cellIs"'))
  assert.ok(xml1.includes('operator="greaterThan"'))
  assert.ok(xml1.includes('<formula>80</formula>'))
  assert.ok(xml1.includes('dxfId="1"'))
  assert.equal(dxfSink.ids.length, 1)
})

test('xlsxCf: dataBar rule emits databar element', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const rules: CfWireRule[] = [
    {
      ranges: [{ startRow: 0, endRow: 4, startColumn: 1, endColumn: 1 }],
      stopIfTrue: false,
      rule: {
        type: 'dataBar',
        config: {
          min: { type: 'min' },
          max: { type: 'max' },
          positiveColor: '#638EC6',
        },
      },
    },
  ]
  const xml1 = applyCfRules(xml0, rules, dxfSink)
  assert.ok(xml1.includes('<dataBar>'))
  assert.ok(xml1.includes('<cfvo type="min"/>'))
  assert.ok(xml1.includes('<cfvo type="max"/>'))
})

test('xlsxCf: colorScale rule emits colorScale element', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const rules: CfWireRule[] = [
    {
      ranges: [{ startRow: 0, endRow: 4, startColumn: 1, endColumn: 1 }],
      stopIfTrue: false,
      rule: {
        type: 'colorScale',
        config: [
          { index: 0, value: { type: 'min' }, color: '#F8696B' },
          { index: 1, value: { type: 'percentile', value: 50 }, color: '#FFEB84' },
          { index: 2, value: { type: 'max' }, color: '#63BE7B' },
        ],
      },
    },
  ]
  const xml1 = applyCfRules(xml0, rules, dxfSink)
  assert.ok(xml1.includes('<colorScale>'))
  assert.ok(xml1.includes('<cfvo type="percentile" val="50"/>'))
  assert.ok(xml1.includes('rgb="FFF8696B"'))
})

test('xlsxCf: empty rules removes any existing CF block', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const xml1 = applyCfRules(xml0, [
    {
      ranges: [{ startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 }],
      stopIfTrue: false,
      rule: {
        type: 'highlightCell',
        subType: 'number',
        operator: 'greaterThan',
        value: 80,
        style: { fontColor: '#FF0000' },
      },
    },
  ], dxfSink)
  assert.ok(xml1.includes('<conditionalFormatting'))
  const xml2 = applyCfRules(xml1, [], dxfSink)
  assert.ok(!xml2.includes('<conditionalFormatting'))
})

test('xlsxCf: unknown rule type throws CfEditError', () => {
  assert.throws(
    () =>
      applyCfRules(
        '<worksheet><sheetData></sheetData></worksheet>',
        [
          {
            ranges: [{ startRow: 0, endRow: 4, startColumn: 0, endColumn: 2 }],
            stopIfTrue: false,
            rule: { type: 'aboveAverage' },
          },
        ],
        dxfSink,
      ),
    (err) => err instanceof CfEditError,
  )
})

// ===== Data Validation =====

test('xlsxDv: list validation emits dataValidations + dropdown', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  assert.ok(!xml0.includes('<dataValidations'))

  const rules: DvWireRule[] = [
    {
      ranges: [{ startRow: 1, endRow: 4, startColumn: 2, endColumn: 2 }],
      rule: {
        type: 'list',
        formula1: '"blue,red,green"',
        showInputMessage: true,
        prompt: '请选择颜色',
      },
    },
  ]
  const xml1 = applyDvRules(xml0, rules)
  assert.ok(xml1.includes('<dataValidations count="1">'))
  assert.ok(xml1.includes('type="list"'))
  assert.ok(xml1.includes('blue,red,green'))
  assert.ok(xml1.includes('showInputMessage="1"'))
  assert.ok(xml1.includes('prompt='))
})

test('xlsxDv: greaterThanOrEqual numeric validation emits operator', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const rules: DvWireRule[] = [
    {
      ranges: [{ startRow: 1, endRow: 4, startColumn: 1, endColumn: 1 }],
      rule: { type: 'whole', operator: 'greaterThanOrEqual', formula1: '0' },
    },
  ]
  const xml1 = applyDvRules(xml0, rules)
  assert.ok(xml1.includes('operator="greaterThanOrEqual"'))
  assert.ok(xml1.includes('<formula1>0</formula1>'))
})

test('xlsxDv: empty rules removes dataValidations', async () => {
  const bytes = await fixtureBytes()
  const xml0 = (await readSheetXml(bytes, 'Sheet1'))!
  const xml1 = applyDvRules(xml0, [
    {
      ranges: [{ startRow: 1, endRow: 1, startColumn: 0, endColumn: 0 }],
      rule: { type: 'list', formula1: '"a,b"' },
    },
  ])
  assert.ok(xml1.includes('<dataValidations'))
  const xml2 = applyDvRules(xml1, [])
  assert.ok(!xml2.includes('<dataValidations'))
})

// ===== Hyperlinks =====

test('xlsxHyperlinks: applyHyperlinkEdits inserts <hyperlink> elements', () => {
  const xml = '<worksheet><sheetData></sheetData></worksheet>'
  const patch = applyHyperlinkEdits(xml, null, [
    { row: 0, column: 0, target: 'https://example.com' },
    { row: 1, column: 1, target: "'Sheet2'!A1" },
  ])
  const next = patch.worksheetXml
  assert.ok(next.includes('<hyperlink '))
  assert.ok(next.includes('ref="A1"'))
  assert.ok(next.includes('ref="B2"'))
  assert.ok(patch.relsChanged)
})

test('xlsxHyperlinks: ensureRelationshipNamespace adds xmlns:r when missing', () => {
  const xml = '<worksheet></worksheet>'
  const next = ensureRelationshipNamespace(xml)
  assert.ok(next.includes('xmlns:r='))
  assert.equal(ensureRelationshipNamespace(next), next)
})

// ===== Protection =====

test('xlsxProtection: applySheetProtection toggles sheetProtection element', () => {
  const xml = '<worksheet><sheetData></sheetData></worksheet>'
  const locked = applySheetProtection(xml, true)
  assert.ok(locked.includes('<sheetProtection'))
  const unlocked = applySheetProtection(locked, false)
  assert.ok(!unlocked.includes('<sheetProtection'))
})

test('xlsxProtection: applyWorkbookProtection toggles workbookProtection', () => {
  const xml = '<workbook><sheets></sheets></workbook>'
  const locked = applyWorkbookProtection(xml, true)
  assert.ok(locked.includes('workbookProtection'))
})

test('xlsxProtection: applyProtectedRanges injects <protectedRanges>', () => {
  const xml = '<worksheet><sheetData></sheetData></worksheet>'
  const next = applyProtectedRanges(xml, [
    { name: 'range1', sqref: 'A1:B5' },
    { name: 'range2', sqref: 'D10:F20' },
  ])
  assert.ok(next.includes('<protectedRanges>'))
  assert.ok(next.includes('name="range1"'))
  assert.ok(next.includes('sqref="A1:B5"'))
  assert.ok(next.includes('name="range2"'))
})

// ===== Theme =====

test('xlsxTheme: applyThemeState swaps colors and fonts', () => {
  const xml = `<?xml version="1.0"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<a:themeElements>
<a:clrScheme name="orig">
<a:dk1><a:srgbClr val="000000"/></a:dk1>
<a:lt1><a:srgbClr val="FFFFFF"/></a:lt1>
<a:dk2><a:srgbClr val="1F497D"/></a:dk2>
<a:lt2><a:srgbClr val="EEECE1"/></a:lt2>
<a:accent1><a:srgbClr val="4F81BD"/></a:accent1>
<a:accent2><a:srgbClr val="C0504D"/></a:accent2>
<a:accent3><a:srgbClr val="9BBB59"/></a:accent3>
<a:accent4><a:srgbClr val="8064A2"/></a:accent4>
<a:accent5><a:srgbClr val="4BACC6"/></a:accent5>
<a:accent6><a:srgbClr val="F79646"/></a:accent6>
<a:hlink><a:srgbClr val="0000FF"/></a:hlink>
<a:folHlink><a:srgbClr val="800080"/></a:folHlink>
</a:clrScheme>
<a:fontScheme name="orig">
<a:majorFont><a:latin typeface="Calibri Light"/></a:majorFont>
<a:minorFont><a:latin typeface="Calibri"/></a:minorFont>
</a:fontScheme>
</a:themeElements>
</a:theme>`
  const state: WorkbookThemeState = {
    colors: {
      name: 'Custom',
      values: ['#FFFFFF','#112233','#EEECE1','#1F497D','#4F81BD','#C0504D','#9BBB59','#8064A2','#4BACC6','#F79646','#0000FF','#800080'],
    },
    fonts: { name: 'CustomFonts', major: 'Calibri Light', minor: 'Calibri' },
  }
  const next = applyThemeState(xml, state)
  assert.ok(next.includes('Custom'))
  assert.ok(next.includes('112233'))
  assert.ok(next.includes('Calibri'))
})

// ===== Defined Names =====

test('xlsxDefinedNames: applyDefinedNamesState writes <definedNames>', () => {
  const xml = '<workbook><sheets></sheets></workbook>'
  const state: DefinedNamesState = {
    names: [
      { name: 'TaxRate', formula: 'Sheet1!$B$2' },
      { name: 'MyRange', formula: '$A$1:$D$10', sheetIndex: 0 },
    ],
    preserveNames: [],
  }
  const next = applyDefinedNamesState(xml, state)
  assert.ok(next.includes('<definedNames'))
  assert.ok(next.includes('TaxRate'))
  assert.ok(next.includes('MyRange'))
  assert.ok(next.includes('localSheetId="0"'))
})

test('xlsxDefinedNames: future-function markers prefixed for Excel 365', () => {
  const marked = withFutureFunctionMarkers('=XLOOKUP(A1,B:B,C:C)')
  assert.ok(marked.includes('_xlfn.XLOOKUP'))
  const idem = withFutureFunctionMarkers(marked)
  assert.equal(idem, marked)
})

test('xlsxDefinedNames: FILTER/SORT use _xlfn._xlws prefix', () => {
  const marked = withFutureFunctionMarkers('=FILTER(A1:A10,B1:B10>0)')
  assert.ok(marked.includes('_xlfn._xlws.FILTER'))
})

import {
  applyChartEdit,
  ChartEditError,
} from '../xlsxChart'

// Minimal barChart fixture: 1 series, 3 numeric points.
const barChartXml = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <c:chart>
    <c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Old Title</a:t></a:r></a:p></c:rich></c:tx><c:overlay val="0"/></c:title>
    <c:autoTitleDeleted val="0"/>
    <c:plotArea>
      <c:layout/>
      <c:barChart>
        <c:barDir val="col"/>
        <c:grouping val="clustered"/>
        <c:ser>
          <c:idx val="0"/>
          <c:order val="0"/>
          <c:tx><c:v>Series A</c:v></c:tx>
          <c:cat><c:strRef><c:f>Sheet1!$A$2:$A$4</c:f><c:strCache><c:ptCount val="3"/><c:pt idx="0"><c:v>Jan</c:v></c:pt><c:pt idx="1"><c:v>Feb</c:v></c:pt><c:pt idx="2"><c:v>Mar</c:v></c:pt></c:strCache></c:strRef></c:cat>
          <c:val><c:numRef><c:f>Sheet1!$B$2:$B$4</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="3"/><c:pt idx="0"><c:v>10</c:v></c:pt><c:pt idx="1"><c:v>20</c:v></c:pt><c:pt idx="2"><c:v>30</c:v></c:pt></c:numCache></c:numRef></c:val>
        </c:ser>
        <c:axId val="111"/>
        <c:axId val="222"/>
      </c:barChart>
      <c:catAx><c:axId val="111"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="b"/></c:catAx>
      <c:valAx><c:axId val="222"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="l"/></c:valAx>
    </c:plotArea>
    <c:plotVisOnly val="1"/>
    <c:dispBlanksAs val="gap"/>
  </c:chart>
</c:chartSpace>`

test('xlsxChart: applyChartEdit sets chart title', () => {
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    title: 'New Title',
  })
  assert.ok(next.includes('New Title'), 'new title present')
  assert.ok(!next.includes('Old Title'), 'old title gone')
})

test('xlsxChart: applyChartEdit converts chartType', () => {
  // column → line should remove barChart and add lineChart.
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    chartType: 'line',
  })
  assert.ok(next.includes('<c:lineChart>'), 'lineChart element added')
  assert.ok(!next.includes('<c:barChart>'), 'barChart removed')
})

test('xlsxChart: applyChartEdit sets seriesColors', () => {
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    seriesColors: { '0': '#FF0000' },
  })
  assert.ok(next.includes('FF0000'), 'new color present')
})

test('xlsxChart: applyChartEdit sets grouping', () => {
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    grouping: 'stacked',
  })
  assert.ok(next.includes('grouping val="stacked"'), 'grouping set')
})

test('xlsxChart: applyChartEdit sets value-axis bounds', () => {
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    valueAxis: { min: 0, max: 100 },
  })
  assert.ok(next.includes('<c:min val="0"/>'), 'min set')
  assert.ok(next.includes('<c:max val="100"/>'), 'max set')
})

test('xlsxChart: applyChartEdit replaces series via seriesSet', () => {
  const next = applyChartEdit(barChartXml, {
    chartPath: 'xl/charts/chart1.xml',
    seriesSet: [
      {
        name: 'X',
        values: [1, 2, 3],
        valuesRef: 'Sheet1!$B$2:$B$4',
        categories: ['a', 'b', 'c'],
        categoriesRef: 'Sheet1!$A$2:$A$4',
      },
    ],
  })
  assert.ok(next.includes('<c:v>X</c:v>'), 'new series name')
  assert.ok(next.includes('<c:v>1</c:v>'), 'first value')
})

test('xlsxChart: grouping on non-stackable chart throws', () => {
  // pie charts do not support grouping — should fail closed.
  const pieXml = barChartXml.replace('<c:barChart>', '<c:pieChart>').replace('</c:barChart>', '</c:pieChart>').replace('<c:barDir val="col"/>', '').replace('<c:grouping val="clustered"/>', '')
  assert.throws(
    () => applyChartEdit(pieXml, {
      chartPath: 'xl/charts/chart1.xml',
      grouping: 'stacked',
    }),
    (err) => err instanceof ChartEditError,
  )
})
