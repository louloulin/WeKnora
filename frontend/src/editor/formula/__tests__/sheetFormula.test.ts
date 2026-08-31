import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  evaluateFormula,
  collectRangeValues,
  collectRangeStrings,
  colNameToIndex,
  resolveSheetRef,
  resolveCellRef,
  splitFormulaArgs,
  stripStringQuotes,
  type SheetLookup,
} from '../sheetFormula'

// Helper to make a tiny two-sheet workbook for the cross-sheet cases.
const sheets: SheetLookup = new Map([
  ['Sheet1', [
    ['1', '2', '3'],
    ['4', '5', '6'],
    ['7', '8', '9'],
    ['apple', 'banana', 'cherry'],
  ]],
  ['Sheet2', [
    ['10', '20'],
    ['30', '40'],
  ]],
])

test('colNameToIndex: A=0, Z=25, AA=26, AB=27', () => {
  assert.equal(colNameToIndex('A'), 0)
  assert.equal(colNameToIndex('Z'), 25)
  assert.equal(colNameToIndex('AA'), 26)
  assert.equal(colNameToIndex('AB'), 27)
  assert.equal(colNameToIndex('a'), 0) // case-insensitive
})

test('resolveSheetRef: bare and quoted sheet names', () => {
  assert.deepEqual(resolveSheetRef('Sheet2!A1'), { sheet: 'Sheet2', cell: 'A1' })
  assert.deepEqual(resolveSheetRef("'Sheet 2'!B2"), { sheet: 'Sheet 2', cell: 'B2' })
  assert.equal(resolveSheetRef('A1'), null)
})

test('resolveCellRef: numeric vs out-of-range', () => {
  assert.equal(resolveCellRef('A1', sheets.get('Sheet1')!), 1)
  assert.equal(resolveCellRef('C3', sheets.get('Sheet1')!), 9)
  // Out-of-range row or column → 0 (matches Excel numeric-context behaviour).
  assert.equal(resolveCellRef('A99', sheets.get('Sheet1')!), 0)
  assert.equal(resolveCellRef('ZZ1', sheets.get('Sheet1')!), 0)
  // Truly unparseable ref → NaN.
  assert.ok(Number.isNaN(resolveCellRef('not a ref', sheets.get('Sheet1')!)))
})

test('splitFormulaArgs: top-level commas only', () => {
  assert.deepEqual(splitFormulaArgs('A1,A2,A3'), ['A1', 'A2', 'A3'])
  assert.deepEqual(splitFormulaArgs('A1, "hello, world", A2'), ['A1', '"hello, world"', 'A2'])
  assert.deepEqual(splitFormulaArgs('IF(A1>5,B1,C1)'), ['IF(A1>5,B1,C1)'])
})

test('stripStringQuotes: strips paired quotes only', () => {
  assert.equal(stripStringQuotes('"hello"'), 'hello')
  assert.equal(stripStringQuotes('  hello  '), 'hello')
  assert.equal(stripStringQuotes('"unbalanced'), '"unbalanced')
})

test('collectRangeValues: single cell', () => {
  assert.deepEqual(collectRangeValues('A1', 'Sheet1', sheets), [1])
  assert.deepEqual(collectRangeValues('C3', 'Sheet1', sheets), [9])
})

test('collectRangeValues: 2D range with text skipped', () => {
  // A1:C2 = [1,2,3,4,5,6]
  assert.deepEqual(collectRangeValues('A1:C2', 'Sheet1', sheets), [1, 2, 3, 4, 5, 6])
})

test('collectRangeValues: cross-sheet range', () => {
  // Sheet2!A1:B2 = [10,20,30,40]
  assert.deepEqual(collectRangeValues('Sheet2!A1:B2', 'Sheet1', sheets), [10, 20, 30, 40])
})

test('collectRangeStrings: row of strings', () => {
  // A4:C4 = ["apple", "banana", "cherry"]
  assert.deepEqual(collectRangeStrings('A4:C4', 'Sheet1', sheets), ['apple', 'banana', 'cherry'])
})

test('evaluateFormula: SUM range', () => {
  assert.equal(evaluateFormula('=SUM(A1:C2)', 'Sheet1', sheets), '21')
})

test('evaluateFormula: SUM mixed local + cross-sheet', () => {
  assert.equal(evaluateFormula('=SUM(A1:A3, Sheet2!A1:B1)', 'Sheet1', sheets), '42')
})

test('evaluateFormula: AVERAGE / COUNT / COUNTA / MIN / MAX', () => {
  assert.equal(evaluateFormula('=AVERAGE(A1:C2)', 'Sheet1', sheets), '3.5')
  assert.equal(evaluateFormula('=COUNT(A1:C2)', 'Sheet1', sheets), '6')
  assert.equal(evaluateFormula('=COUNTA(A4:C4)', 'Sheet1', sheets), '3')
  assert.equal(evaluateFormula('=MIN(A1:C2)', 'Sheet1', sheets), '1')
  assert.equal(evaluateFormula('=MAX(A1:C2)', 'Sheet1', sheets), '6')
})

test('evaluateFormula: COUNTIF / SUMIF with operator and equality', () => {
  assert.equal(evaluateFormula('=COUNTIF(A1:C2, ">4")', 'Sheet1', sheets), '2')
  assert.equal(evaluateFormula('=COUNTIF(A4:C4, "banana")', 'Sheet1', sheets), '1')
  assert.equal(evaluateFormula('=SUMIF(A4:C4, "apple", A4:C4)', 'Sheet1', sheets), '0') // apple is not numeric
})

test('evaluateFormula: IF with literal compare', () => {
  assert.equal(evaluateFormula('=IF(A1>0, "pos", "neg")', 'Sheet1', sheets), 'pos')
  assert.equal(evaluateFormula('=IF(A1>9, "pos", "neg")', 'Sheet1', sheets), 'neg')
})

test('evaluateFormula: IF with cell-ref compare', () => {
  // A1=1, B1=2 → A1<B1 → "yes"
  assert.equal(evaluateFormula('=IF(A1<B1, "yes", "no")', 'Sheet1', sheets), 'yes')
})

test('evaluateFormula: CONCAT / LEN / ROUND / ABS / TEXT', () => {
  assert.equal(evaluateFormula('=CONCAT("a", "b", "c")', 'Sheet1', sheets), 'abc')
  assert.equal(evaluateFormula('=LEN(A4)', 'Sheet1', sheets), '5') // "apple"
  assert.equal(evaluateFormula('=ROUND(3.14159, 2)', 'Sheet1', sheets), '3.14')
  assert.equal(evaluateFormula('=ABS(-7)', 'Sheet1', sheets), '7')
  assert.equal(evaluateFormula('=TEXT(0.25, "0%")', 'Sheet1', sheets), '25%')
})

test('evaluateFormula: VLOOKUP matches and #N/A fallback', () => {
  // A4:C4 → looking up "apple" in column A, return column B → "banana"
  assert.equal(evaluateFormula('=VLOOKUP("apple", A4:C4, 2)', 'Sheet1', sheets), 'banana')
  // Missing key → "#N/A"
  assert.equal(evaluateFormula('=VLOOKUP("durian", A4:C4, 2)', 'Sheet1', sheets), '#N/A')
})

test('evaluateFormula: token arithmetic with cell refs', () => {
  assert.equal(evaluateFormula('=A1+B1', 'Sheet1', sheets), '3')   // 1 + 2
  assert.equal(evaluateFormula('=A1*10+B2', 'Sheet1', sheets), '15') // 1*10 + 5
  assert.equal(evaluateFormula('=C3-1', 'Sheet1', sheets), '8')
})

test('evaluateFormula: cross-sheet token arithmetic', () => {
  // Sheet2!A1=10, Sheet2!B2=40, Sheet1!A1=1 → 10+40+1=51
  assert.equal(evaluateFormula('=Sheet2!A1+Sheet2!B2+Sheet1!A1', 'Sheet1', sheets), '51')
})

test('evaluateFormula: empty / unparseable', () => {
  assert.equal(evaluateFormula('', 'Sheet1', sheets), '')
  assert.equal(evaluateFormula('=', 'Sheet1', sheets), '')
  assert.throws(() => evaluateFormula('=FOO()', 'Sheet1', sheets))
})

test('evaluateFormula: throws on unknown sheet', () => {
  assert.throws(
    () => evaluateFormula('=Sheet99!A1', 'Sheet1', sheets),
    /unknown sheet/,
  )
})
