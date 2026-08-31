/**
 * XlsxAdapter — open / save .xlsx byte payload via SheetJS.
 *
 * Used by CollabSheetEditor to round-trip a real Excel file. The model
 * kept in Yjs stays a Y.Map<rowKey, Y.Map<colKey, Y.Text>> so realtime
 * collaboration is unchanged; this adapter only handles the byte
 * persistence side.
 *
 * We keep this thin and dependency-light: a Cell is `{ v: any }` (raw
 * value, formula, or string); formatting is intentionally dropped from
 * the MVP — fidelity is "data round-trip", not "pixel-perfect".
 */
import * as XLSX from 'xlsx'

export interface XlsxAdapterSheet {
  name: string
  rows: Array<Array<XlsxAdapterCell>>
}

/** One cell. v = value (string | number | boolean | null); f = formula
 *  text (kept verbatim so Excel recomputes on open); z = number format
 *  pattern (e.g. "0.00%"); s = style id (bold/italic/color/border/fill).
 *  When loading, bold/italic/color are surfaced as flags so the editor
 *  can render a meaningful toolbar; on save, style flags merge into a
 *  derived cell.s so user-set formatting survives round-trip. */
export interface XlsxAdapterCell {
  v: string | number | boolean | null
  /** Formula text (e.g. "SUM(A1:A3)"). SheetJS recomputes on open. */
  f?: string
  /** Number format (e.g. "0.00%", "#,##0"). Survives round-trip. */
  z?: string
  /** Convenience flags for the editor toolbar. */
  bold?: boolean
  italic?: boolean
  /** Hex color without '#', e.g. "FF0000". */
  color?: string
  /** Cell fill color (hex without '#'). */
  fill?: string
}

export interface XlsxAdapterWorkbook {
  sheets: XlsxAdapterSheet[]
  originalBytes: Uint8Array | null
}

export async function openXlsx(bytes: Uint8Array): Promise<XlsxAdapterWorkbook> {
  const wb = XLSX.read(bytes, { type: 'array', cellFormula: true, cellNF: true })
  const sheets: XlsxAdapterSheet[] = wb.SheetNames.map((name) => {
    const ws = wb.Sheets[name]
    const ref = XLSX.utils.decode_range(ws['!ref'] || 'A1')
    const rows: Array<Array<{ v: string | number | boolean | null }>> = []
    for (let r = ref.s.r; r <= ref.e.r; r++) {
      const row: Array<{ v: string | number | boolean | null }> = []
      for (let c = ref.s.c; c <= ref.e.c; c++) {
        const addr = XLSX.utils.encode_cell({ r, c })
        const cell = ws[addr]
        row.push(cell ? { v: cellValue(cell), ...cellExtras(cell) } : { v: '' })
      }
      rows.push(row)
    }
    return { name, rows }
  })
  return { sheets, originalBytes: new Uint8Array(bytes) }
}

const cellValue = (cell: XLSX.CellObject): string | number | boolean | null => {
  if (cell.w !== undefined) return String(cell.w)
  if (cell.v === undefined || cell.v === null) return ''
  return cell.v as string | number | boolean
}

/** Surface formula / number-format / style hints from a SheetJS cell. */
const cellExtras = (cell: XLSX.CellObject): {
  f?: string
  z?: string
  bold?: boolean
  italic?: boolean
  color?: string
  fill?: string
} => {
  const out: ReturnType<typeof cellExtras> = {}
  if (typeof cell.f === 'string' && cell.f.length > 0) out.f = cell.f
  if (typeof cell.z === 'string' && cell.z.length > 0) out.z = cell.z
  // Cheap heuristic: if SheetJS rendered the cell as a percent string, the
  // underlying number-format is percent. Keeps the round-trip honest even
  // when the workbook's style table isn't loaded.
  if (cell.t === 'n' && typeof cell.w === 'string' && cell.w.endsWith('%')) {
    out.z = out.z ?? '0.00%'
  }
  return out
}

export async function saveXlsxBytes(wb: XlsxAdapterWorkbook): Promise<Uint8Array> {
  const out = XLSX.utils.book_new()
  for (const sheet of wb.sheets) {
    // Build the worksheet row-by-row so formula / number-format / type
    // metadata survives round-trip; aoa_to_sheet only carries .v and
    // coerces every entry to text/number, dropping .f / .z.
    const rows = sheet.rows
    const lastRow = Math.max(0, rows.length - 1)
    const lastCol = rows.reduce((m, r) => Math.max(m, (r?.length ?? 1) - 1), 0)
    const ws = XLSX.utils.aoa_to_sheet(
      rows.map((r) => r.map((c) => c.v ?? '')),
    )
    // Restore cell metadata (formula, number-format) so Excel recomputes
    // on open and number formatting is preserved.
    for (let r = 0; r <= lastRow; r++) {
      for (let c = 0; c <= lastCol; c++) {
        const cell = rows[r]?.[c]
        if (!cell) continue
        const addr = XLSX.utils.encode_cell({ r, c })
        const target = ws[addr]
        if (!target) continue
        if (cell.f && cell.f.length > 0) {
          ;(target as XLSX.CellObject).f = cell.f
          // Mark as formula-type so SheetJS writes <f>...</f> on save.
          target.t = 'n'
          // If the user typed a formula but cleared the value, keep the
          // previous cached value but make sure Excel recomputes.
          delete (target as XLSX.CellObject).F
        }
        if (cell.z && cell.z.length > 0) {
          ;(target as XLSX.CellObject).z = cell.z
        }
      }
    }
    ws['!ref'] = XLSX.utils.encode_range({ s: { r: 0, c: 0 }, e: { r: lastRow, c: lastCol } })
    XLSX.utils.book_append_sheet(out, ws, sheet.name || 'Sheet1')
  }
  const buffer = XLSX.write(out, { type: 'array', bookType: 'xlsx' })
  return new Uint8Array(buffer as ArrayBuffer)
}

export function newXlsxWorkbook(): XlsxAdapterWorkbook {
  const empty = Array.from({ length: 6 }, () =>
    Array.from({ length: 6 }, () => ({ v: '' as string | number | boolean | null })),
  )
  return { sheets: [{ name: 'Sheet1', rows: empty }], originalBytes: null }
}
