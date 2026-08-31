/**
 * xlsxWorksheetIo — bridge between a SheetJS .xlsx byte buffer and
 * individual worksheet XML strings. The pure-function adapters
 * (xlsxFilter, xlsxSparkline, xlsxCf, xlsxDv — vendored from
 * genoffice) operate on worksheet XML, so this IO layer is the
 * only place that touches JSZip + SheetJS byte plumbing.
 *
 * Vendored from genoffice's xlsx-gateway family — kept thin on
 * purpose so future vendor merges are a single-file diff.
 */
import JSZip from 'jszip'

export interface SheetXmlIo {
  /** Path inside the zip → worksheet XML. */
  readonly sheetPaths: ReadonlyMap<string, string>
  /** Sheet name → index in workbook.xml's `<sheets>` order. */
  readonly sheetNames: readonly string[]
}

const matchAll = (re: RegExp, text: string): RegExpExecArray[] => {
  const out: RegExpExecArray[] = []
  let m: RegExpExecArray | null
  // Make a fresh instance per call so the global lastIndex doesn't leak.
  const fresh = new RegExp(re.source, re.flags.replace(/g$/, '') + 'g')
  while ((m = fresh.exec(text))) out.push(m)
  return out
}

/** Build a SheetXmlIo for an .xlsx byte buffer. */
export async function inspectXlsx(bytes: Uint8Array): Promise<SheetXmlIo> {
  const zip = await JSZip.loadAsync(bytes)
  const workbookXml = await zip.file('xl/workbook.xml')?.async('text')
  const relsXml = await zip.file('xl/_rels/workbook.xml.rels')?.async('text')
  if (!workbookXml || !relsXml) {
    return { sheetPaths: new Map(), sheetNames: [] }
  }
  // rId → target path (relative to xl/).
  const relTargets = new Map<string, string>()
  for (const m of matchAll(/<Relationship\b([^>]*?)\/?>/g, relsXml)) {
    const attrs = m[1] || ''
    const id = (attrs.match(/\bId="([^"]*)"/) || [, ''])[1]
    const target = (attrs.match(/\bTarget="([^"]*)"/) || [, ''])[1]
    if (id && target) {
      relTargets.set(id, target.startsWith('/') ? target.slice(1) : `xl/${target}`)
    }
  }
  // Sheet entries: name + rId, in workbook order.
  const sheetNames: string[] = []
  const sheetPaths = new Map<string, string>()
  for (const m of matchAll(/<sheet\b([^/>]*?)\/?>/g, workbookXml)) {
    const attrs = m[1] || ''
    const name = (attrs.match(/\bname="([^"]*)"/) || [, ''])[1]
    const rid = (attrs.match(/\br:id="([^"]*)"/) || [, ''])[1]
    if (!name) continue
    sheetNames.push(name)
    const target = relTargets.get(rid)
    if (target) sheetPaths.set(name, target)
  }
  return { sheetPaths, sheetNames }
}

/** Read the worksheet XML for one sheet. Returns undefined if not found. */
export async function readSheetXml(
  bytes: Uint8Array,
  sheetName: string,
): Promise<string | undefined> {
  const zip = await JSZip.loadAsync(bytes)
  const io = await inspectXlsx(bytes)
  const path = io.sheetPaths.get(sheetName)
  if (!path) return undefined
  return zip.file(path)?.async('text') ?? undefined
}

/** Replace the worksheet XML for one sheet and return the new byte buffer. */
export async function writeSheetXml(
  bytes: Uint8Array,
  sheetName: string,
  nextXml: string,
): Promise<Uint8Array> {
  const zip = await JSZip.loadAsync(bytes)
  const io = await inspectXlsx(bytes)
  const path = io.sheetPaths.get(sheetName)
  if (!path) {
    throw new Error(`writeSheetXml: sheet "${sheetName}" not found in workbook`)
  }
  zip.file(path, nextXml)
  return zip.generateAsync({ type: 'uint8array' })
}

/** Apply a worksheet-XML transformation across every sheet that has a name
 *  in `freezes` (or `transform` keys). The transform is a pure function
 *  (worksheetXml) → worksheetXml. Sheets whose transform returns the
 *  same string are skipped (no write, no zip rewrite). */
export async function transformWorkbook(
  bytes: Uint8Array,
  transforms: Record<string, (worksheetXml: string) => string>,
): Promise<Uint8Array> {
  const zip = await JSZip.loadAsync(bytes)
  const io = await inspectXlsx(bytes)
  let dirty = false
  for (const [sheetName, transform] of Object.entries(transforms)) {
    const path = io.sheetPaths.get(sheetName)
    if (!path) continue
    const xml = await zip.file(path)?.async('text')
    if (xml === undefined) continue
    const next = transform(xml)
    if (next !== xml) {
      zip.file(path, next)
      dirty = true
    }
  }
  if (!dirty) return bytes
  return zip.generateAsync({ type: 'uint8array' })
}

// ============================================================
// v0.7.46 — multi-file PackageTransform (notes / tables / hyperlinks).
// ============================================================

/** Mutable OOXML package — abstraction over JSZip used by the vendored
 *  adapters (xlsxNotes, xlsxTableAdd, xlsxDrawingAdd). All paths are
 *  zip-absolute (no leading slash). */
export interface MutablePackage {
  paths(): Promise<readonly string[]>
  has(path: string): Promise<boolean>
  readText(path: string): Promise<string>
  write(path: string, content: string): void
  add(path: string, content: string): void
  remove(path: string): void
}

/** Apply a multi-file package transform that may read/write/create/delete
 *  arbitrary zip parts in addition to worksheet XML. The `transformer`
 *  callback runs with a `MutablePackage` view over a JSZip instance;
 *  returning without throwing commits all writes.
 *
 *  Returns the original bytes when the transformer is a no-op (no
 *  writes happen), preserving identity short-circuit for cheap paths.
 */
export async function transformPackage(
  bytes: Uint8Array,
  transformer: (pkg: MutablePackage) => Promise<void> | void,
): Promise<Uint8Array> {
  const zip = await JSZip.loadAsync(bytes)
  const dirty = { value: false }
  const pkg: MutablePackage = {
    async paths() {
      const out: string[] = []
      zip.forEach((p) => out.push(p))
      return out.sort()
    },
    async has(path) {
      return zip.file(path) !== null
    },
    async readText(path) {
      return (await zip.file(path)?.async('text')) ?? ''
    },
    write(path, content) {
      const existing = zip.file(path)
      if (existing && typeof existing === 'object' && '_data' in existing) {
        // Update in place to preserve lastModified / binary mode.
        zip.file(path, content)
      } else {
        zip.file(path, content)
      }
      dirty.value = true
    },
    add(path, content) {
      zip.file(path, content)
      dirty.value = true
    },
    remove(path) {
      zip.remove(path)
      dirty.value = true
    },
  }
  await transformer(pkg)
  if (!dirty.value) return bytes
  return zip.generateAsync({ type: 'uint8array' })
}
