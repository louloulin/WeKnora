// v0.7.43 — SHEET formula engine (Feishu / Tencent-Docs parity, MVP).
//
// Pure module — no Vue, no DOM. The SHEET editor only renders the result of
// `evaluateFormula` in the formula bar; actual persistence of the formula
// itself happens via the OOXML writer in `xlsxWorksheetIo.ts`.
//
// Scope:
//   - Cross-sheet refs (`Sheet2!A1`, `Sheet2!A1:A10`).
//   - Aggregations: SUM / AVERAGE / COUNT / COUNTA / MIN / MAX.
//   - Conditionals: IF (with cell-ref compares), COUNTIF, SUMIF.
//   - Lookup: VLOOKUP.
//   - String / numeric helpers: CONCAT / CONCATENATE / LEN / ROUND / ABS / TEXT.
//   - Token-level arithmetic with cell refs (`A1 + B2 * 2`).
//
// Out of scope (will land in follow-ups):
//   - Nested IF / IFS / SWITCH.
//   - Array formulas (Ctrl+Shift+Enter).
//   - Real circular-reference detection.

export type SheetLookup = ReadonlyMap<string, string[][]>

/**
 * Convert a column label (A, B, …, Z, AA, AB, …) to a 0-based column index.
 */
export const colNameToIndex = (name: string): number => {
  let n = 0
  for (const ch of name.toUpperCase()) n = n * 26 + (ch.charCodeAt(0) - 64)
  return n - 1
}

/**
 * Resolve a single cell reference (e.g. `B3`) on the given grid and return
 * its numeric value. Returns `0` for empty cells (matches Excel semantics),
 * `NaN` for unparseable refs or non-numeric content.
 */
export const resolveCellRef = (ref: string, grid: string[][]): number => {
  const m = /^([A-Z]+)(\d+)$/i.exec(ref.trim())
  if (!m) return NaN
  const ci = colNameToIndex(m[1])
  const ri = Number(m[2]) - 1
  const v = Number(grid[ri]?.[ci])
  return Number.isNaN(v) ? 0 : v
}

/**
 * Detect a `SheetName!…` reference and split it into `{ sheet, cell }`.
 * Accepts both `Sheet2!A1` and `'Sheet 2'!A1` (quoted form, common in Excel).
 */
export const resolveSheetRef = (raw: string): { sheet: string; cell: string } | null => {
  const m = /^'?([^'!]+)'?!(.+)$/.exec(raw.trim())
  if (!m) return null
  return { sheet: m[1].replace(/^'/, '').replace(/'$/, ''), cell: m[2] }
}

/**
 * Resolve either a cross-sheet ref (`Sheet2!A1`) or a local ref (`A1`).
 * Returns a number for numeric cells or the original string for text cells.
 */
export const resolveAnyCellRef = (
  raw: string,
  currentSheet: string,
  lookup: SheetLookup,
): number | string => {
  const sr = resolveSheetRef(raw)
  if (sr) {
    const grid = lookup.get(sr.sheet)
    if (!grid) throw new Error(`unknown sheet: ${sr.sheet}`)
    return resolveCellRef(sr.cell, grid)
  }
  return resolveCellRef(raw.trim(), lookup.get(currentSheet) ?? [])
}

/**
 * Split a function argument list on top-level commas, honouring quoted
 * strings and nested parens. Empty pieces are skipped.
 */
export const splitFormulaArgs = (raw: string): string[] => {
  const out: string[] = []
  let depth = 0
  let buf = ''
  let inStr = false
  for (const ch of raw) {
    if (ch === '"') { inStr = !inStr; buf += ch; continue }
    if (inStr) { buf += ch; continue }
    if (ch === '(') depth += 1
    else if (ch === ')') depth -= 1
    else if (ch === ',' && depth === 0) { out.push(buf.trim()); buf = ''; continue }
    buf += ch
  }
  if (buf.trim()) out.push(buf.trim())
  return out
}

/**
 * Materialise a range (or single cell) reference into the list of numeric
 * values it covers. Non-numeric cells are skipped (matches Excel SUM etc.).
 */
export const collectRangeValues = (
  ref: string,
  currentSheet: string,
  lookup: SheetLookup,
): number[] => {
  const sr = resolveSheetRef(ref)
  const sheetName = sr ? sr.sheet : currentSheet
  const cellPart = sr ? sr.cell : ref.trim()
  const grid = lookup.get(sheetName)
  if (!grid) throw new Error(`unknown sheet: ${sheetName}`)
  const range = /^([A-Z]+)(\d+):([A-Z]+)(\d+)$/i.exec(cellPart)
  if (!range) {
    const v = resolveCellRef(cellPart, grid)
    return Number.isNaN(v) ? [] : [v]
  }
  const ca = colNameToIndex(range[1].toUpperCase())
  const cb = colNameToIndex(range[3].toUpperCase())
  const ra = Number(range[2]) - 1
  const rb = Number(range[4]) - 1
  const out: number[] = []
  for (let r = ra; r <= rb; r += 1) {
    for (let c = ca; c <= cb; c += 1) {
      const v = Number(grid[r]?.[c])
      if (!Number.isNaN(v)) out.push(v)
    }
  }
  return out
}

/**
 * Materialise a range (or single cell) reference into the list of *string*
 * representations of its cells. Used by COUNTIF / SUMIF / LEN which need the
 * raw textual form for comparison.
 */
export const collectRangeStrings = (
  ref: string,
  currentSheet: string,
  lookup: SheetLookup,
): string[] => {
  const sr = resolveSheetRef(ref)
  const sheetName = sr ? sr.sheet : currentSheet
  const cellPart = sr ? sr.cell : ref.trim()
  const grid = lookup.get(sheetName)
  if (!grid) return []
  const range = /^([A-Z]+)(\d+):([A-Z]+)(\d+)$/i.exec(cellPart)
  if (!range) {
    const m = /^([A-Z]+)(\d+)$/i.exec(cellPart)
    if (!m) return ['']
    const ri = Number(m[2]) - 1
    const ci = colNameToIndex(m[1])
    return [String(grid[ri]?.[ci] ?? '')]
  }
  const ca = colNameToIndex(range[1].toUpperCase())
  const cb = colNameToIndex(range[3].toUpperCase())
  const ra = Number(range[2]) - 1
  const rb = Number(range[4]) - 1
  const out: string[] = []
  for (let r = ra; r <= rb; r += 1) {
    for (let c = ca; c <= cb; c += 1) {
      out.push(String(grid[r]?.[c] ?? ''))
    }
  }
  return out
}

/**
 * Strip surrounding double-quotes from a token, leaving the inner string.
 * Returns the original (trimmed) string when there are no surrounding quotes.
 */
export const stripStringQuotes = (s: string): string => {
  const t = s.trim()
  if (t.length >= 2 && t.startsWith('"') && t.endsWith('"')) return t.slice(1, -1)
  return t
}

/**
 * Evaluate a SHEET formula expression and return its string result.
 * Accepts:
 *   - Function calls (`SUM(A1:A10)`, `IF(A1>5,"big","small")`).
 *   - Token-level arithmetic with cell refs (`A1 + B2 * 2`).
 *
 * Errors are thrown as `Error` — the caller (formula bar) catches and
 * displays nothing in the result span.
 */
export const evaluateFormula = (
  expr: string,
  currentSheet: string,
  lookup: SheetLookup,
): string => {
  const cleaned = expr.replace(/^=/, '').trim()
  if (!cleaned) return ''
  const fnMatch = /^([A-Z]+)\((.*)\)$/i.exec(cleaned)
  if (fnMatch) {
    const fn = fnMatch[1].toUpperCase()
    const raw = fnMatch[2]
    const args = splitFormulaArgs(raw)
    switch (fn) {
      case 'SUM': {
        const flat = args.flatMap((a) => collectRangeValues(a, currentSheet, lookup))
        return String(flat.reduce((x, y) => x + y, 0))
      }
      case 'AVERAGE':
      case 'AVG': {
        const flat = args.flatMap((a) => collectRangeValues(a, currentSheet, lookup))
        return flat.length ? String(flat.reduce((x, y) => x + y, 0) / flat.length) : '0'
      }
      case 'COUNT': {
        const flat = args.flatMap((a) => collectRangeValues(a, currentSheet, lookup))
        return String(flat.length)
      }
      case 'COUNTA': {
        const flat = args.flatMap((a) => collectRangeStrings(a, currentSheet, lookup))
        return String(flat.filter((s) => s !== '').length)
      }
      case 'MIN': {
        const flat = args.flatMap((a) => collectRangeValues(a, currentSheet, lookup))
        return flat.length ? String(Math.min(...flat)) : '0'
      }
      case 'MAX': {
        const flat = args.flatMap((a) => collectRangeValues(a, currentSheet, lookup))
        return flat.length ? String(Math.max(...flat)) : '0'
      }
      case 'COUNTIF': {
        const [rangeArg, criteriaArg] = args
        if (!rangeArg || !criteriaArg) throw new Error('COUNTIF needs range and criteria')
        const values = collectRangeStrings(rangeArg, currentSheet, lookup)
        const criteria = stripStringQuotes(criteriaArg)
        const m = /^([<>=!]+)?(.+)$/.exec(criteria)
        const op = m?.[1] ?? '=='
        const target = Number(m?.[2])
        if (!Number.isNaN(target)) {
          let n = 0
          for (const v of values) {
            const num = Number(v)
            if (Number.isNaN(num)) continue
            if (op === '>' && num > target) n += 1
            else if (op === '<' && num < target) n += 1
            else if ((op === '=' || op === '==') && num === target) n += 1
            else if (op === '>=' && num >= target) n += 1
            else if (op === '<=' && num <= target) n += 1
            else if (op === '!=' && num !== target) n += 1
          }
          return String(n)
        }
        return String(values.filter((v) => v === criteria).length)
      }
      case 'SUMIF': {
        const [rangeArg, criteriaArg] = args
        if (!rangeArg || !criteriaArg) throw new Error('SUMIF needs range and criteria')
        const values = collectRangeStrings(rangeArg, currentSheet, lookup)
        const criteria = stripStringQuotes(criteriaArg)
        let total = 0
        for (let i = 0; i < values.length; i += 1) {
          if (values[i] === criteria) {
            const num = Number(values[i])
            if (!Number.isNaN(num)) total += num
          }
        }
        return String(total)
      }
      case 'IF': {
        const [condArg, thenArg, elseArg] = args
        if (!condArg) throw new Error('IF needs condition')
        const condVal = resolveAnyCellRef(condArg, currentSheet, lookup)
        const cond = ((): boolean => {
          const t = condArg.trim()
          const cmp = /^([A-Z]+\d+|[\d.]+)\s*(>=|<=|>|<|=)\s*([A-Z]+\d+|[\d.]+)$/i.exec(t)
          if (cmp) {
            const lvRaw = Number(cmp[1])
            const rvRaw = Number(cmp[3])
            const lv = Number.isNaN(lvRaw)
              ? Number(resolveAnyCellRef(cmp[1], currentSheet, lookup))
              : lvRaw
            const rv = Number.isNaN(rvRaw)
              ? Number(resolveAnyCellRef(cmp[3], currentSheet, lookup))
              : rvRaw
            return cmp[2] === '=' ? lv === rv
              : cmp[2] === '>' ? lv > rv
              : cmp[2] === '<' ? lv < rv
              : cmp[2] === '>=' ? lv >= rv
              : cmp[2] === '<=' ? lv <= rv
              : false
          }
          return Boolean(Number(condVal))
        })()
        return cond ? stripStringQuotes(thenArg ?? 'TRUE') : stripStringQuotes(elseArg ?? 'FALSE')
      }
      case 'CONCAT':
      case 'CONCATENATE': {
        return args.map((a) => {
          const sr = resolveSheetRef(a)
          if (sr) {
            const m = /^([A-Z]+)(\d+)$/i.exec(sr.cell)
            if (!m) return ''
            const ri = Number(m[2]) - 1
            const ci = colNameToIndex(m[1])
            return String(lookup.get(sr.sheet)?.[ri]?.[ci] ?? '')
          }
          return stripStringQuotes(a)
        }).join('')
      }
      case 'LEN': {
        const flat = args.flatMap((a) => collectRangeStrings(a, currentSheet, lookup))
        return flat.length ? String(flat[0].length) : '0'
      }
      case 'ROUND': {
        const [vArg, dArg] = args
        const v = Number(stripStringQuotes(vArg))
        const d = dArg ? Number(stripStringQuotes(dArg)) : 0
        const m = Math.pow(10, d)
        return String(Math.round(v * m) / m)
      }
      case 'ABS': {
        return String(Math.abs(Number(stripStringQuotes(args[0] ?? '0'))))
      }
      case 'TEXT': {
        const [vArg, fmtArg] = args
        const v = stripStringQuotes(vArg)
        const fmt = stripStringQuotes(fmtArg ?? '')
        if (fmt === '0.00') return Number(v).toFixed(2)
        if (fmt === '0%') return `${Number(v) * 100}%`
        if (fmt === '0.00%') return `${(Number(v) * 100).toFixed(2)}%`
        return v
      }
      case 'VLOOKUP': {
        const [lookupValArg, tableArg, colIdxArg] = args
        if (!lookupValArg || !tableArg || !colIdxArg) throw new Error('VLOOKUP needs lookup, table, col')
        const lookupVal = stripStringQuotes(lookupValArg)
        const sr = resolveSheetRef(tableArg)
        const sheetName = sr ? sr.sheet : currentSheet
        const cellPart = sr ? sr.cell : tableArg.trim()
        const grid = lookup.get(sheetName)
        if (!grid) throw new Error(`unknown sheet: ${sheetName}`)
        const range = /^([A-Z]+)(\d+):([A-Z]+)(\d+)$/i.exec(cellPart)
        if (!range) throw new Error('VLOOKUP table must be a range')
        const ca = colNameToIndex(range[1].toUpperCase())
        const cb = colNameToIndex(range[3].toUpperCase())
        const ra = Number(range[2]) - 1
        const rb = Number(range[4]) - 1
        const colIdx = Number(stripStringQuotes(colIdxArg)) - 1
        for (let r = ra; r <= rb; r += 1) {
          if (String(grid[r]?.[ca] ?? '') === lookupVal) {
            return String(grid[r]?.[ca + colIdx] ?? '')
          }
        }
        return '#N/A'
      }
      default: throw new Error('unknown function: ' + fn)
    }
  }
  // Token-based arithmetic with optional cell references (cross-sheet OK).
  const tokens = cleaned.split(/([+*\-\/])/).map((s) => s.trim()).filter(Boolean)
  let acc: number | null = null
  let op = '+'
  for (const tok of tokens) {
    if (tok === '+' || tok === '-' || tok === '*' || tok === '/') { op = tok; continue }
    const numLit = Number(tok)
    if (!Number.isNaN(numLit)) {
      acc = acc == null
        ? numLit
        : op === '+' ? acc + numLit
        : op === '-' ? acc - numLit
        : op === '*' ? acc * numLit
        : acc / numLit
      continue
    }
    const refV = Number(resolveAnyCellRef(tok, currentSheet, lookup))
    if (Number.isNaN(refV)) throw new Error('bad token: ' + tok)
    acc = acc == null
      ? refV
      : op === '+' ? acc + refV
      : op === '-' ? acc - refV
      : op === '*' ? acc * refV
      : acc / refV
  }
  return acc == null ? '' : String(acc)
}
