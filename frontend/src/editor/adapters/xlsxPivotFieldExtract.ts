/**
 * xlsxPivotFieldExtract — v0.7.110 XLSX Pivot UI 辅助
 *
 * 从 SHEET 当前选中区域（或指定 range）抽取：
 *  - fieldNames：第 1 行（表头）
 *  - rowItems：第 1 列（row dimension field）的去重值
 *  - aggregatedValues：value column 按 aggregation (sum/count/avg/max/min) 聚合的结果
 *
 * 输入是 row-major 的二维数组（每个 inner 是 string）。
 * 输出符合 PivotAddition 的最小 row + 1 value 结构。
 */
export type AggregationKind = 'sum' | 'count' | 'average' | 'max' | 'min'

export interface PivotExtractSeed {
  /** All rows including header (rows[0] = header names). */
  rows: readonly (readonly string[])[]
  /** 0-based column index used as row dimension (e.g. 0 for "Category"). */
  rowDimIdx: number
  /** 0-based column index used as value (e.g. 2 for "Amount"). */
  valueIdxIdx: number
  /** Aggregation kind for value column. */
  agg: AggregationKind
}

export interface PivotExtractResult {
  fieldNames: string[]
  rowItems: string[]
  /** Map rowItem -> aggregated value (number). */
  valuesByRow: Record<string, number>
}

/** Sum / count / average / max / min of a numeric column, ignoring blanks. */
function aggregate(values: number[], agg: AggregationKind): number {
  if (values.length === 0) return 0
  switch (agg) {
    case 'sum': return values.reduce((a, b) => a + b, 0)
    case 'count': return values.length
    case 'average': return values.reduce((a, b) => a + b, 0) / values.length
    case 'max': return values.reduce((a, b) => Math.max(a, b), -Infinity)
    case 'min': return values.reduce((a, b) => Math.min(a, b), Infinity)
  }
}

/** Parse a cell as a number; returns NaN for blanks / non-numeric. */
function parseNum(s: string): number {
  if (!s || !s.trim()) return NaN
  const n = Number(s.replace(/[,，\s]/g, ''))
  return Number.isFinite(n) ? n : NaN
}

export function extractPivotSeed(seed: PivotExtractSeed): PivotExtractResult {
  const { rows, rowDimIdx, valueIdxIdx, agg } = seed
  if (rows.length < 2) {
    return { fieldNames: [], rowItems: [], valuesByRow: {} }
  }
  const header = rows[0]
  const fieldNames = header.map((h) => h || '')
  const seen: string[] = []
  const rowItemIndex = new Map<string, number>()
  const valuesByRow: Record<string, number[]> = {}
  for (let i = 1; i < rows.length; i++) {
    const row = rows[i]
    const key = (row[rowDimIdx] ?? '').trim() || '(Blank)' // empty row dim → common bucket
    if (!rowItemIndex.has(key)) {
      rowItemIndex.set(key, seen.length)
      seen.push(key)
      valuesByRow[key] = []
    }
    const rawVal = row[valueIdxIdx] ?? ''
    const num = parseNum(rawVal)
    if (!Number.isNaN(num)) valuesByRow[key].push(num)
  }
  // aggregate
  const out: Record<string, number> = {}
  for (const k of seen) {
    out[k] = aggregate(valuesByRow[k], agg)
    delete valuesByRow[k] // gc hint
  }
  return { fieldNames, rowItems: seen, valuesByRow: out }
}

/** Build a minimal PivotAddition shape from extract + layout. */
export interface PivotAdditionInput {
  extract: PivotExtractResult
  sourceSheetName: string
  worksheetPath: string
  /** Where to drop the pivot table (1-based cell coords). */
  targetRi: number
  targetCi: number
  agg: AggregationKind
  rowDimIdx: number
  valueIdxIdx: number
}

export function buildPivotAddition(inp: PivotAdditionInput): unknown {
  const { extract, sourceSheetName, worksheetPath, targetRi, targetCi, agg, rowDimIdx, valueIdxIdx } = inp
  const rowItems = extract.rowItems
  const fieldNames = extract.fieldNames
  const headersLen = fieldNames.length
  // Source area covers all rows x fieldNames.length columns
  const sourceArea = {
    startRow: 0,
    startColumn: 0,
    endRow: extract.rowItems.length, // number of data rows
    endColumn: headersLen - 1,
  }
  // Location: target cell + headers + data rows + grand total row
  const locStartRow = targetRi
  const locStartCol = targetCi
  const locEndRow = targetRi + 1 + rowItems.length + 1 // header + rows + grand total
  const locEndCol = targetCi + 1 + 1 // row header + 1 value column
  return {
    worksheetPath,
    sourceSheetName,
    sourceArea,
    location: {
      startRow: locStartRow,
      startColumn: locStartCol,
      endRow: locEndRow,
      endColumn: locEndCol,
    },
    name: 'Pivot1',
    fieldNames,
    rowFieldIndices: [rowDimIdx],
    rowItems,
    values: [{ fieldIndex: valueIdxIdx, agg }],
  }
}
