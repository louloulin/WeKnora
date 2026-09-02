/**
 * xlsxPivotFieldExtract.test — v0.7.110 XLSX Pivot UI 辅助
 *
 * 验证：
 *  - 简单 (Category, Amount) → unique rows + sum
 *  - count / average / max / min aggregation
 *  - 非数值数据 → 跳过
 *  - 空字符串 rowDim → fallback "(RowN)"
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extractPivotSeed, buildPivotAddition } from '../xlsxPivotFieldExtract'

const sampleRows = [
  ['Category', 'Amount', 'Note'],
  ['A', '10', 'x'],
  ['A', '20', 'y'],
  ['B', '5', ''],
  ['B', '15', 'z'],
  ['C', '7', 'q'],
]

test('extractPivotSeed — sum', () => {
  const out = extractPivotSeed({
    rows: sampleRows,
    rowDimIdx: 0,
    valueIdxIdx: 1,
    agg: 'sum',
  })
  assert.deepEqual(out.fieldNames, ['Category', 'Amount', 'Note'])
  assert.deepEqual(out.rowItems, ['A', 'B', 'C'])
  assert.equal(out.valuesByRow['A'], 30)
  assert.equal(out.valuesByRow['B'], 20)
  assert.equal(out.valuesByRow['C'], 7)
})

test('extractPivotSeed — count', () => {
  const out = extractPivotSeed({
    rows: sampleRows,
    rowDimIdx: 0,
    valueIdxIdx: 1,
    agg: 'count',
  })
  assert.equal(out.valuesByRow['A'], 2)
  assert.equal(out.valuesByRow['B'], 2)
  assert.equal(out.valuesByRow['C'], 1)
})

test('extractPivotSeed — average', () => {
  const out = extractPivotSeed({
    rows: sampleRows,
    rowDimIdx: 0,
    valueIdxIdx: 1,
    agg: 'average',
  })
  assert.equal(out.valuesByRow['A'], 15)
  assert.equal(out.valuesByRow['B'], 10)
  assert.equal(out.valuesByRow['C'], 7)
})

test('extractPivotSeed — max / min', () => {
  const maxOut = extractPivotSeed({
    rows: sampleRows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'max',
  })
  assert.equal(maxOut.valuesByRow['A'], 20)
  const minOut = extractPivotSeed({
    rows: sampleRows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'min',
  })
  assert.equal(minOut.valuesByRow['B'], 5)
})

test('extractPivotSeed — non-numeric value cells are skipped', () => {
  const rows = [
    ['Cat', 'Val'],
    ['A', 'abc'],
    ['A', '30'],
    ['B', 'def'],
  ]
  const out = extractPivotSeed({
    rows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'sum',
  })
  assert.equal(out.valuesByRow['A'], 30)
  assert.equal(out.valuesByRow['B'], 0)
})

test('extractPivotSeed — empty / blank category fallback to (Blank)', () => {
  const rows = [
    ['Cat', 'Val'],
    ['', '5'],
    ['', '7'],
    ['A', '10'],
  ]
  const out = extractPivotSeed({
    rows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'sum',
  })
  // Both blank rows coalesce under "(Blank)"; A is its own row.
  assert.deepEqual(out.rowItems, ['(Blank)', 'A'])
  assert.equal(out.valuesByRow['(Blank)'], 12)
  assert.equal(out.valuesByRow['A'], 10)
})

test('extractPivotSeed — comma-formatted numbers', () => {
  const rows = [
    ['Cat', 'Val'],
    ['A', '1,000'],
    ['A', '2,500'],
  ]
  const out = extractPivotSeed({
    rows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'sum',
  })
  assert.equal(out.valuesByRow['A'], 3500)
})

test('buildPivotAddition — minimal PivotAddition shape', () => {
  const out = extractPivotSeed({
    rows: sampleRows, rowDimIdx: 0, valueIdxIdx: 1, agg: 'sum',
  })
  const addition = buildPivotAddition({
    extract: out,
    sourceSheetName: 'Sheet1',
    worksheetPath: 'xl/worksheets/sheet1.xml',
    targetRi: 10,
    targetCi: 5,
    agg: 'sum',
    rowDimIdx: 0,
    valueIdxIdx: 1,
  })
  const a = addition as any
  assert.equal(a.name, 'Pivot1')
  assert.equal(a.rowFieldIndices[0], 0)
  assert.equal(a.values[0].fieldIndex, 1)
  assert.equal(a.values[0].agg, 'sum')
  assert.equal(a.location.startRow, 10)
  assert.equal(a.location.startColumn, 5)
  assert.equal(a.rowItems.length, 3)
})
