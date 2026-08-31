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
  rows: Array<Array<{ v: string | number | boolean | null }>>
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
        row.push({ v: cell ? cellValue(cell) : '' })
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

export async function saveXlsxBytes(wb: XlsxAdapterWorkbook): Promise<Uint8Array> {
  const out = XLSX.utils.book_new()
  for (const sheet of wb.sheets) {
    const aoa: any[][] = sheet.rows.map((r) => r.map((c) => c.v))
    const ws = XLSX.utils.aoa_to_sheet(aoa)
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
