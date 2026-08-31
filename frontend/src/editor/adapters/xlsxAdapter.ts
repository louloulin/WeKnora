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
import JSZip from 'jszip'
import { StylesheetEditor, type WorkbookStyleEdit } from './xlsxStyles'

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
  // SheetJS's cellStyles option overwrites cell.s with a partial style object
  // (just the fill). To recover the full style index per cell we read the
  // worksheet XML directly and look the index up in cellXfs. This is more
  // work than asking SheetJS but it's the only way to get bold/italic/font
  // color back from the style table.
  const wb = XLSX.read(bytes, { type: 'array', cellFormula: true, cellNF: true })
  const styleMap = await readStyleMapFromZip(bytes)
  const sheets: XlsxAdapterSheet[] = wb.SheetNames.map((name) => {
    const ws = wb.Sheets[name]
    const ref = XLSX.utils.decode_range(ws['!ref'] || 'A1')
    const rows: Array<Array<{ v: string | number | boolean | null }>> = []
    for (let r = ref.s.r; r <= ref.e.r; r++) {
      const row: Array<{ v: string | number | boolean | null }> = []
      for (let c = ref.s.c; c <= ref.e.c; c++) {
        const addr = XLSX.utils.encode_cell({ r, c })
        const cell = ws[addr]
        const styleIndex = cell ? styleMap[addr] : undefined
        row.push(cell ? { v: cellValue(cell), ...cellExtras(cell, styleIndex, styleMap) } : { v: '' })
      }
      rows.push(row)
    }
    return { name, rows }
  })
  return { sheets, originalBytes: new Uint8Array(bytes) }
}

interface ParsedStyleEntry {
  bold?: boolean
  italic?: boolean
  fontColor?: string
  fill?: string
}
/** Read xl/styles.xml + each sheet's worksheet XML; return a per-cell map
 *  from cell address → ParsedStyleEntry. Cells without an `s="N"` attribute
 *  are absent from the map. */
const readStyleMapFromZip = async (
  bytes: Uint8Array,
): Promise<Record<string, ParsedStyleEntry>> => {
  const zip = await JSZip.loadAsync(bytes)
  const stylesXml = await zip.file('xl/styles.xml')?.async('text')
  if (!stylesXml) return {}
  const { fonts, fills, cellXfs } = parseStylesheetTables(stylesXml)
  // eslint-disable-next-line no-console
  const out: Record<string, ParsedStyleEntry> = {}
  const sheetFiles = Object.keys(zip.files).filter((p) => /^xl\/worksheets\/sheet\d+\.xml$/.test(p))
  for (const path of sheetFiles) {
    const xml = await zip.file(path)?.async('text')
    if (!xml) continue
    for (const match of xml.matchAll(/<c\s+[^>]*?\br="([A-Z]+\d+)"[^>]*?(?:\s+s="(\d+)")?[^>]*>/g)) {
      const addr = match[1]
      const sIdx = match[2]
      // eslint-disable-next-line no-console
      if (sIdx === undefined) continue
      const idx = Number(sIdx)
      const xf = cellXfs[idx]
      if (!xf) continue
      const entry: ParsedStyleEntry = {}
      const font = xf.fontId !== undefined ? fonts[xf.fontId] : undefined
      if (font) {
        if (font.bold) entry.bold = true
        if (font.italic) entry.italic = true
        if (font.color) entry.fontColor = font.color
      }
      const fill = xf.fillId !== undefined ? fills[xf.fillId] : undefined
      if (fill && fill.patternType && fill.patternType !== 'none' && fill.color) {
        entry.fill = fill.color
      }
      out[addr] = entry
    }
  }
  return out
}
interface ParsedFont { bold?: boolean; italic?: boolean; color?: string }
interface ParsedFill { patternType?: string; color?: string }
interface ParsedCellXf { fontId?: number; fillId?: number }
/** Minimal styles.xml parser — extract fonts / fills / cellXfs enough to map
 *  a style index onto bold/italic/font color/fill color. Anything more
 *  complex (numFmt, border, alignment) is handled by SheetJS. */
const parseStylesheetTables = (xml: string): {
  fonts: ParsedFont[]
  fills: ParsedFill[]
  cellXfs: ParsedCellXf[]
} => {
  const fonts: ParsedFont[] = []
  const fills: ParsedFill[] = []
  const cellXfs: ParsedCellXf[] = []
  // Scope each section — <cellStyleXfs> uses the same <xf/> element so a
  // global regex would shift our cellXfs index by +1.
  const fontsSection = /<fonts[\s>][\s\S]*?<\/fonts>/.exec(xml)?.[0] ?? ''
  for (const m of fontsSection.matchAll(/<font>([\s\S]*?)<\/font>/g)) {
    const inner = m[1]
    const font: ParsedFont = {}
    if (/<b\/>|<b>\s*<\/b>/.test(inner)) font.bold = true
    if (/<i\/>|<i>\s*<\/i>/.test(inner)) font.italic = true
    const colorMatch = /<color\s+rgb="([0-9A-Fa-f]+)"/.exec(inner)
    if (colorMatch) {
      const hex = colorMatch[1].toUpperCase()
      font.color = hex.length === 8 ? hex.slice(2) : hex
    }
    fonts.push(font)
  }
  const fillsSection = /<fills[\s>][\s\S]*?<\/fills>/.exec(xml)?.[0] ?? ''
  for (const m of fillsSection.matchAll(/<fill>([\s\S]*?)<\/fill>/g)) {
    const inner = m[1]
    const fill: ParsedFill = {}
    const pt = /<patternFill\s+patternType="([^"]+)"|<patternType="([^"]+)"/.exec(inner)
    if (pt) fill.patternType = pt[1] ?? pt[2]
    const colorMatch = /<fgColor\s+rgb="([0-9A-Fa-f]+)"/.exec(inner)
    if (colorMatch) {
      const hex = colorMatch[1].toUpperCase()
      fill.color = hex.length === 8 ? hex.slice(2) : hex
    }
    fills.push(fill)
  }
  const cellXfsSection = /<cellXfs[\s>][\s\S]*?<\/cellXfs>/.exec(xml)?.[0] ?? ''
  for (const m of cellXfsSection.matchAll(/<xf\s+([^/>]*?)\/>/g)) {
    const attrs = m[1]
    const fontId = /fontId="(\d+)"/.exec(attrs)
    const fillId = /fillId="(\d+)"/.exec(attrs)
    cellXfs.push({
      fontId: fontId ? Number(fontId[1]) : undefined,
      fillId: fillId ? Number(fillId[1]) : undefined,
    })
  }
  return { fonts, fills, cellXfs }
}

const cellValue = (cell: XLSX.CellObject): string | number | boolean | null => {
  // SheetJS keeps the underlying value in `v` and the formatted display
  // string in `w`. For formula cells the cached `v` may be stale (Excel
  // recomputes on open) but it's still a number/bool — better to surface
  // it as such than the formatted "0" string.
  if (cell.f && cell.v !== undefined && cell.v !== null) {
    return cell.v as string | number | boolean
  }
  if (cell.v === undefined || cell.v === null) return ''
  return cell.v as string | number | boolean
}

/** Surface formula / number-format / style hints from a SheetJS cell. */
type CellExtras = {
  f?: string
  z?: string
  bold?: boolean
  italic?: boolean
  color?: string
  fill?: string
}
const cellExtras = (
  cell: XLSX.CellObject,
  styleEntry: ParsedStyleEntry | undefined,
  _styleMap: Record<string, ParsedStyleEntry>,
): CellExtras => {
  const out: CellExtras = {}
  if (typeof cell.f === 'string' && cell.f.length > 0) out.f = cell.f
  if (typeof cell.z === 'string' && cell.z.length > 0) out.z = cell.z
  if (cell.t === 'n' && typeof cell.w === 'string' && cell.w.endsWith('%')) {
    out.z = out.z ?? '0.00%'
  }
  if (styleEntry) {
    if (styleEntry.bold) out.bold = true
    if (styleEntry.italic) out.italic = true
    if (styleEntry.fontColor) out.color = styleEntry.fontColor
    if (styleEntry.fill) out.fill = styleEntry.fill
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
          // SheetJS's aoa_to_sheet drops cells with v==='' so we have to
          // seed a placeholder value; Excel recomputes on open.
          if (target.v === undefined || target.v === '') target.v = 0
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
  const initial = new Uint8Array(buffer as ArrayBuffer)
  return await applyCellStyles(initial, wb)
}

/** Minimal xl/styles.xml used when the SheetJS output doesn't carry one
 *  (e.g. an empty newXlsxWorkbook). Mirrors the structure SheetJS produces
 *  for a fresh book. */
const SEED_STYLES_XML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>
  <borders count="1"><border/></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

/** Build the WorkbookStyleEdit delta for one cell, if any. */
const styleEditForCell = (cell: XlsxAdapterCell): WorkbookStyleEdit | null => {
  const edit: WorkbookStyleEdit = {}
  let touched = false
  if (cell.bold) { edit.bold = true; touched = true }
  if (cell.italic) { edit.italic = true; touched = true }
  if (cell.color) { edit.fontColor = cell.color.replace(/^#/, ''); touched = true }
  if (cell.fill) { edit.fillColor = cell.fill.replace(/^#/, ''); touched = true }
  return touched ? edit : null
}

/** Rewrite xl/styles.xml + per-sheet cell s="N" attribute to persist the
 *  color / bold / italic / fill flags on XlsxAdapterCell. Cells without
 *  style flags are skipped. The styles.xml file is left intact when no
 *  cell needs styling. */
const applyCellStyles = async (
  bytes: Uint8Array,
  wb: XlsxAdapterWorkbook,
): Promise<Uint8Array> => {
  // 1) Discover which cells need styling. We rebuild the per-sheet cell
  //    address map because SheetJS's row/col layout may differ from the
  //    input rows array.
  const styleBySheet = new Map<string, Map<string, WorkbookStyleEdit>>()
  for (const sheet of wb.sheets) {
    const cellMap = new Map<string, WorkbookStyleEdit>()
    for (let r = 0; r < sheet.rows.length; r++) {
      for (let c = 0; c < (sheet.rows[r]?.length ?? 0); c++) {
        const cell = sheet.rows[r]?.[c]
        if (!cell) continue
        const edit = styleEditForCell(cell)
        if (!edit) continue
        const addr = XLSX.utils.encode_cell({ r, c })
        cellMap.set(addr, edit)
      }
    }
    if (cellMap.size > 0) styleBySheet.set(sheet.name, cellMap)
  }
  if (styleBySheet.size === 0) return bytes

  // 2) Open the zip and load styles.xml.
  const zip = await JSZip.loadAsync(bytes)
  const stylesPath = 'xl/styles.xml'
  const stylesEntry = zip.file(stylesPath)
  const stylesXml = stylesEntry ? await stylesEntry.async('text') : SEED_STYLES_XML
  const editor = new StylesheetEditor(stylesXml)

  // 3) Resolve a cellXfs index for every (sheet, addr) we need to style.
  const styleIndexBySheet = new Map<string, Map<string, number>>()
  for (const [sheetName, cellMap] of styleBySheet) {
    const idxMap = new Map<string, number>()
    for (const [addr, edit] of cellMap) {
      // base xf 0 (default) — overwrite baseXfId per cell if we ever load
      // styles per-cell from the original sheet XML. For now we always
      // derive from xfId=0 since we control the input cell array.
      const xfId = editor.resolveStyle(0, edit)
      idxMap.set(addr, xfId)
    }
    styleIndexBySheet.set(sheetName, idxMap)
  }

  // 4) Rewrite xl/styles.xml if anything changed.
  let stylesChanged = false
  for (const [, cellMap] of styleBySheet) {
    if (cellMap.size > 0) { stylesChanged = true; break }
  }
  if (stylesChanged) {
    zip.file(stylesPath, editor.serialize())
  }

  // 5) Stamp s="N" on each styled cell inside the sheet XML.
  for (const [sheetName, idxMap] of styleIndexBySheet) {
    const sheetPath = `xl/worksheets/sheet${sheetIndexOf(wb, sheetName)}.xml`
    const sheetEntry = zip.file(sheetPath)
    if (!sheetEntry) continue
    let xmlText = await sheetEntry.async('text')
    for (const [addr, idx] of idxMap) {
      const cellRe = new RegExp(`(<c\\s+[^>]*\\br="${addr}"[^>]*?)(\\s+s="\\d+")?([^>]*>)`)
      const before = xmlText
      xmlText = xmlText.replace(cellRe, (_m, head, _old, tail) => `${head} s="${idx}"${tail}`)
      if (xmlText === before) {
        // eslint-disable-next-line no-console
        console.warn(`[applyCellStyles] failed to rewrite cell ${addr} on ${sheetName}`)
      }
    }
    zip.file(sheetPath, xmlText)
  }
  return zip.generateAsync({ type: 'uint8array' })
}

/** Map sheet display-name → sheetN.xml index in the workbook. */
const sheetIndexOf = (wb: XlsxAdapterWorkbook, name: string): number => {
  const idx = wb.sheets.findIndex((s) => s.name === name)
  return idx < 0 ? 1 : idx + 1
}

export function newXlsxWorkbook(): XlsxAdapterWorkbook {
  const empty = Array.from({ length: 6 }, () =>
    Array.from({ length: 6 }, () => ({ v: '' as string | number | boolean | null })),
  )
  return { sheets: [{ name: 'Sheet1', rows: empty }], originalBytes: null }
}

/**
 * Freeze pane support — Feishu/Lark/Tencent-equivalent "freeze first row" /
 * "freeze first column" / "freeze both".
 *
 * Excel stores freeze state per sheet under
 *   <worksheet><sheetViews><sheetView workbookViewId="0">
 *     <pane xSplit="N" ySplit="M" topLeftCell="A2" activePane="..." state="frozen"/>
 *   </sheetView></sheetViews></worksheet>
 *
 * SheetJS does not expose a typed surface for sheetView/pane, so we
 * read & write the underlying .xlsx zip directly with JSZip. The bytes
 * round-trip cleanly into Excel and Google Sheets.
 */
import { inspectXlsx, readSheetXml, transformWorkbook } from './xlsxWorksheetIo'

/** Frozen rows / columns for a single sheet (0 = none). */
export interface XlsxFreezePane {
  rows: number
  cols: number
}

/** A sheet name → freeze pane mapping (sheetName is the workbook sheet name). */
export type XlsxFreezeMap = Record<string, XlsxFreezePane>

/** OOXML top-left cell for a (rows, cols) freeze, given 1-based cell coords. */
const topLeftFor = (rows: number, cols: number): string => {
  // Convert cols (0-based) to letter, rows (0-based) + 1 → 1-based row index.
  let n = cols
  let s = ''
  do {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  } while (n >= 0)
  return `${s}${rows + 1}`
}

const activePaneFor = (rows: number, cols: number): string => {
  if (rows > 0 && cols > 0) return 'bottomRight'
  if (rows > 0) return 'bottomLeft'
  if (cols > 0) return 'topRight'
  return 'topLeft'
}

/** Build the <sheetView>/<pane> XML fragment for a given freeze pane. */
export const buildSheetViewXml = (pane: XlsxFreezePane): string => {
  if (!pane || (pane.rows <= 0 && pane.cols <= 0)) {
    return '<sheetViews><sheetView workbookViewId="0"/></sheetViews>'
  }
  const xSplit = pane.cols
  const ySplit = pane.rows
  const topLeft = topLeftFor(pane.rows, pane.cols)
  const active = activePaneFor(pane.rows, pane.cols)
  return (
    `<sheetViews><sheetView workbookViewId="0">` +
    `<pane xSplit="${xSplit}" ySplit="${ySplit}" topLeftCell="${topLeft}" activePane="${active}" state="frozen"/>` +
    `<selection pane="${active}" activeCell="${topLeft}" sqref="${topLeft}"/>` +
    `</sheetView></sheetViews>`
  )
}

/** Inject the <sheetViews>...</sheetViews> fragment into a single sheet XML,
 *  preserving the OOXML element order (dimension → sheetViews → sheetFormatPr).
 *  If a <sheetViews> already exists, replace it in place. */
const injectSheetView = (sheetXml: string, fragment: string): string => {
  // 1) Replace existing sheetViews.
  if (/<sheetViews>[\s\S]*?<\/sheetViews>/.test(sheetXml)) {
    return sheetXml.replace(/<sheetViews>[\s\S]*?<\/sheetViews>/, fragment)
  }
  if (/<sheetViews\s*\/>/.test(sheetXml)) {
    return sheetXml.replace(/<sheetViews\s*\/>/, fragment)
  }
  // 2) Insert after <dimension .../>.
  const dim = sheetXml.match(/<dimension[^>]*\/?>(?:<\/dimension>)?/)
  if (dim) {
    return sheetXml.replace(dim[0], `${dim[0]}${fragment}`)
  }
  // 3) Insert before <sheetFormatPr>.
  const fmt = sheetXml.match(/<sheetFormatPr[^>]*\/?>(?:<\/sheetFormatPr>)?/)
  if (fmt) {
    return sheetXml.replace(fmt[0], `${fragment}${fmt[0]}`)
  }
  // 4) Fallback: insert right after the <worksheet ...> opening tag.
  return sheetXml.replace(/(<worksheet[^>]*>)/, `$1${fragment}`)
}

/** Read every sheet's freeze pane from a fresh .xlsx byte buffer. Returns
 *  an empty map (and the parsed workbook's sheet order) when no freeze
 *  state is found. Sheet names not present in the workbook are skipped. */
export async function readXlsxFreeze(bytes: Uint8Array): Promise<{
  sheetNames: string[]
  freezes: XlsxFreezeMap
}> {
  const io = await inspectXlsx(bytes)
  const freezes: XlsxFreezeMap = {}
  for (const sheetName of io.sheetNames) {
    const xml = await readSheetXml(bytes, sheetName)
    if (!xml) continue
    const m = xml.match(/<pane\b([^/>]*)\/?>/)
    if (!m) continue
    const attrs = m[1] || ''
    const xSplit = Number((attrs.match(/\bxSplit="([^"]*)"/) || [, '0'])[1]) || 0
    const ySplit = Number((attrs.match(/\bySplit="([^"]*)"/) || [, '0'])[1]) || 0
    const state = (attrs.match(/\bstate="([^"]*)"/) || [, ''])[1]
    if (state !== 'frozen' && state !== 'frozenSplit') continue
    if (xSplit === 0 && ySplit === 0) continue
    freezes[sheetName] = { rows: ySplit, cols: xSplit }
  }
  return { sheetNames: io.sheetNames, freezes }
}

/** Write the freeze map back into a .xlsx byte buffer (one entry per sheet
 *  by workbook order). Sheets not present in `freezes` are left untouched
 *  (any pre-existing sheetView is preserved verbatim). */
export async function applyXlsxFreeze(
  bytes: Uint8Array,
  freezes: XlsxFreezeMap,
): Promise<Uint8Array> {
  const transforms: Record<string, (xml: string) => string> = {}
  for (const [sheetName, pane] of Object.entries(freezes)) {
    transforms[sheetName] = (xml) => injectSheetView(xml, buildSheetViewXml(pane))
  }
  return transformWorkbook(bytes, transforms)
}
