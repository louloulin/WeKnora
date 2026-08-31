/**
 * DocxAdapter — browser-friendly wrapper around @genoffice-style docx-engine.
 *
 * Goals:
 *   1. Hide the TS source path so the rest of WeKnora imports a stable, single
 *      entry (`@/editor/adapters/docxAdapter`).
 *   2. Surface only what the collaborative editor needs: open a .docx blob,
 *      flatten to a paragraph list the TipTap editor can consume, save the
 *      patched bytes back, and produce markdown for KB ingestion.
 *   3. Drive every byte-level patch through the same Yjs text channel so two
 *      clients converge on the same .docx (GenOffice's "byte-preserving" model
 *      + WeKnora's CRDT fabric).
 *
 * Wired (v0.7.28):
 *   - Image binary embedding (real PNG/JPEG bytes via JSZip post-process)
 *
 * Still deferred (v0.7.29+):
 *   - Charts / equations (OMML) / tables-of-contents
 *   - Tracked changes (kept verbatim on save)
 */
import JSZip from 'jszip'
import {
  parseDocx,
  saveDocx,
  type Block as EngineBlock,
  type ParsedDocFull,
  type SaveBlock,
  type SaveOptions,
  type Run as EngineRun,
} from '../engines/docx-engine/index'

/**
 * Minimal single-paragraph XML patcher.
 *
 * The docx-engine's patchParagraphTexts() is designed for footnote/endnote
 * *entries* (multi-paragraph bodies); it returns null when the input has
 * a different paragraph count than the new text. For body paragraphs we
 * want a different contract: keep the original <w:p> shell verbatim
 * (pPr, bookmarks, hyperlinks, fields, images) and only rewrite the
 * concatenated <w:t> text inside runs.
 */
function patchSingleParagraphXml(entryXml: string, newText: string): string {
  const textRegex = /<w:t(?:\s[^>]*)?>[\s\S]*?<\/w:t>/g
  const matches: { start: number; end: number; openEnd: number; closeStart: number }[] = []
  let m: RegExpExecArray | null
  while ((m = textRegex.exec(entryXml)) !== null) {
    const tag = m[0]
    const closeStart = entryXml.lastIndexOf('</w:t>', m.index)
    matches.push({
      start: m.index,
      end: m.index + tag.length,
      openEnd: closeStart,
      closeStart,
    })
  }
  if (matches.length === 0) {
    const selfClose = /<w:p([^>]*)\/>/.exec(entryXml)
    if (selfClose) {
      const attrs = selfClose[1] || ''
      const insert = `<w:r><w:t xml:space="preserve">${escapeXmlForT(newText)}</w:t></w:r>`
      return (
        entryXml.slice(0, selfClose.index) +
        `<w:p${attrs}>${insert}</w:p>` +
        entryXml.slice(selfClose.index + selfClose[0].length)
      )
    }
    const end = entryXml.lastIndexOf('</w:p>')
    if (end < 0) return entryXml
    const insert = `<w:r><w:t xml:space="preserve">${escapeXmlForT(newText)}</w:t></w:r>`
    return entryXml.slice(0, end) + insert + entryXml.slice(end)
  }
  const firstOpen = entryXml.slice(matches[0].start, matches[0].openEnd)
  const firstRun = `<w:t xml:space="preserve">${escapeXmlForT(newText)}</w:t>`
  const newFirst = firstOpen + firstRun
  let out = entryXml.slice(0, matches[0].start) + newFirst + entryXml.slice(matches[0].end)
  for (let i = matches.length - 1; i >= 1; i--) {
    const idx = matches[i]
    out =
      out.slice(0, idx.start) +
      out.slice(idx.openEnd, idx.closeStart).replace(/[\s\S]*?/, '') +
      out.slice(idx.end)
  }
  return out
}

function escapeXmlForT(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export interface DocxAdapterParagraph {
  /** Original engine docxIndex, used as the patch anchor. */
  index: number
  /** Block type as classified by the engine: paragraph / heading / listItem / table / etc. */
  kind: string
  /** Heading level (1..9) when kind === 'heading'; undefined otherwise. */
  level?: number
  /** Concatenated plain text (used by the editor and as KB index text). */
  text: string
  /** True when the engine marked the block hidden / structural. */
  hidden: boolean
}

export interface DocxAdapterDocument {
  paragraphs: DocxAdapterParagraph[]
  /** Engine ParsedDoc kept around so we can serialize back. */
  parsed: ParsedDocFull
  /**
   * docxIndex -> patched paragraph XML. Populated by patchParagraphText
   * and consumed by saveDocxBytes to produce SaveBlocks of kind 'xml'.
   * Cleared after each save so the next dirty cycle starts fresh.
   */
  patched: Map<number, string>
}

/** Open a .docx from raw bytes. */
export async function openDocx(bytes: Uint8Array): Promise<DocxAdapterDocument> {
  const parsed = await parseDocx(new Uint8Array(bytes))
  const paragraphs: DocxAdapterParagraph[] = parsed.blocks.map((b, i) => ({
    index: i,
    kind: b.type,
    level: b.level,
    text: paragraphText(b),
    hidden: !!b.hidden,
  }))
  return { paragraphs, parsed, patched: new Map() }
}

/** Serialize the (possibly patched) parsed doc back to a .docx blob. */
export async function saveDocxBytes(
  doc: DocxAdapterDocument,
  patched?: PatchedParagraph[],
): Promise<Uint8Array> {
  const extra = new Map<number, string>()
  if (patched) for (const p of patched) extra.set(p.docxIndex, p.xml)
  const blocks: SaveBlock[] = doc.parsed.blocks.map((_, i) => {
    const xml = extra.get(i) ?? doc.patched.get(i)
    if (xml) return { kind: 'xml', xml, docxIndex: i } as SaveBlock
    return { kind: 'original', docxIndex: i } as SaveBlock
  })
  const opts: SaveOptions = {}
  const bytes = await saveDocx(doc.parsed, blocks, opts)
  doc.patched.clear()
  return bytes
}

/** Replace a paragraph's runs in place and return the bytes delta for CRDT. */
export interface PatchedParagraph {
  docxIndex: number
  xml: string
  text: string
}

/**
 * Replace a paragraph's runs in place. Returns a SavePlan fragment
 * (SaveBlock with kind 'xml') that the caller can splice into the
 * finalBlocks list passed to saveDocx(). The fragment is computed by
 * extracting the paragraph's original XML from parsed.internal.documentXml
 * and running the engine's patchParagraphTexts against it, so the saved
 * file remains byte-faithful outside the changed paragraph.
 */
export function patchParagraphText(
  doc: DocxAdapterDocument,
  index: number,
  newText: string,
): PatchedParagraph {
  const block = doc.parsed.blocks[index]
  if (!block) throw new Error(`paragraph ${index} out of range`)
  const elements = (doc.parsed.extras as { elements?: Array<{ start: number; end: number; name: string }> })
    .elements ?? []
  // Match the engine's hidden-aware ordering: hidden blocks do not occupy a
  // visible slot. The block.docxIndex points into the top-level body
  // elements array.
  const el = elements[index]
  if (!el) throw new Error(`paragraph ${index} has no body element`)
  const entryXml = doc.parsed.internal.documentXml.slice(el.start, el.end)
  // The engine's patchParagraphTexts() is designed for footnote/endnote
  // entries (multi-paragraph bodies) and refuses to patch when the line
  // count changes. For body paragraphs we want byte-fidelity outside the
  // rewritten runs, which patchSingleParagraphXml() gives us by only
  // touching the concatenated <w:t> spans.
  const patched = patchSingleParagraphXml(entryXml, newText)
  doc.patched.set(index, patched)
  return { docxIndex: index, xml: patched, text: newText }
}

/** Build a markdown representation suitable for KB chunking. */
export function docxToMarkdown(doc: DocxAdapterDocument): string {
  const lines: string[] = []
  for (const p of doc.paragraphs) {
    if (p.hidden) continue
    if (p.kind === 'heading') {
      const level = Math.min(Math.max(p.level ?? 1, 1), 6)
      lines.push(`${'#'.repeat(level)} ${p.text}`)
    } else if (p.kind === 'listItem') {
      lines.push(`- ${p.text}`)
    } else {
      lines.push(p.text)
    }
  }
  return lines.join('\n')
}

function paragraphText(block: EngineBlock): string {
  const runs = collectRuns(block)
  if (runs.length > 0) return runs.map((r) => r.text).join('')
  if ('previewText' in block && typeof (block as { previewText?: string }).previewText === 'string') {
    return (block as { previewText?: string }).previewText || ''
  }
  return ''
}

function collectRuns(block: EngineBlock): EngineRun[] {
  if (block.runs && block.runs.length) return block.runs
  if (block.textboxes && block.textboxes.length) {
    const out: EngineRun[] = []
    for (const box of block.textboxes) {
      for (const para of box.paras ?? []) {
        for (const r of para.runs ?? []) out.push(r)
      }
    }
    return out
  }
  return []
}

/**
 * Build a fresh DocxAdapterDocument from raw paragraph text. Useful when
 * the user starts editing a doc that has never had a .docx uploaded (so
 * there's nothing to parse). The blank-docx bytes come from the engine's
 * own `buildBlankDocx`; we then re-parse them into a DocxAdapterDocument
 * so the rest of the editor pipeline (patchParagraphText, saveDocxBytes)
 * stays symmetric with the loaded-from-server path.
 *
 * Each entry in `paragraphs` becomes a real docx-engine paragraph block;
 * entries with kind === 'heading' use their level (default 1).
 */
export async function buildBlankDocxDoc(
  paragraphs: Array<{ text: string; kind?: 'paragraph' | 'heading' | 'listItem'; level?: number }>,
): Promise<DocxAdapterDocument> {
  const { buildBlankDocx } = await import('../engines/docx-engine/index')
  // buildBlankDocx produces a minimal valid .docx with one empty
  // paragraph. We re-parse to seed a DocxAdapterDocument, then patch the
  // first paragraph to carry the user's first line. The TipTap editor
  // continues to drive the second-and-later paragraphs through the
  // standard patchParagraphText + saveDocxBytes round-trip.
  const bytes = await buildBlankDocx()
  const doc = await openDocx(bytes)
  if (paragraphs.length > 0 && doc.paragraphs.length > 0) {
    patchParagraphText(doc, 0, paragraphs[0].text || ' ')
    doc.paragraphs[0].text = paragraphs[0].text || ' '
  }
  return doc
}

// ============================================================================
// v0.7.27 — pmDocToSavePlan: format-preserving DOC round-trip
// ============================================================================
//
// The text-only patchParagraphText() path rewrites the concatenated <w:t>
// inside a paragraph and silently drops any inline format (bold/italic/...)
// that lived on the original runs. This block adds a TipTap -> SavePlan bridge
// that:
//   - keeps the original byte spans verbatim when the user only edited text;
//   - regenerates the paragraph via the engine's run model when inline format
//     (bold/italic/strike/code/link/underline) actually changed.
//
// Tables, images, shapes, fields, and tracked-change wrappings stay
// read-only on save (they are emitted as `kind: 'original'`).
//
// This is a focused port of genoffice's pmDocToSavePlan() covering the
// paragraph/heading/list/quote/code-block paths. Image/textbox/chart/math/
// sdt-shell handling is deferred to v0.7.28+.

import type { GeneratedBlock, Run } from '../engines/docx-engine/index'

/** Minimal ProseMirror/TipTap JSON shape this adapter understands. */
export interface PmNode {
  type: string
  attrs?: Record<string, unknown>
  content?: PmNode[]
  text?: string
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>
}

export interface PmMark {
  type: string
  attrs?: Record<string, unknown>
}

export interface SavePlanFragment {
  /** Engine SaveBlock entries to splice into saveDocx()'s finalBlocks list. */
  blocks: SaveBlock[]
  /** docxIndex -> text; populated for any block whose text actually changed. */
  textByIndex: Map<number, string>
}

interface PmRun {
  text: string
  bold?: boolean
  italic?: boolean
  underline?: boolean
  strike?: boolean
  code?: boolean
  link?: { href: string; tooltip?: string }
}

/**
 * Walk a TipTap document and produce the SaveBlock[] that re-serializes it
 * into the original .docx bytes with the user's edits applied.
 *
 * Diff strategy (per block):
 *   - text unchanged AND run signatures unchanged -> `kind: 'original'`
 *   - text changed AND run signatures unchanged   -> `kind: 'xml'` (fast path)
 *   - text changed AND run signatures changed    -> `kind: 'generated'`
 */
export function pmDocToSavePlan(pmDoc: PmNode, doc: DocxAdapterDocument): SavePlanFragment {
  const originalBlocks = doc.parsed.blocks
  const blocks: SaveBlock[] = []
  const textByIndex = new Map<number, string>()

  const visibleOriginals: EngineBlock[] = []
  const hiddenOriginals: EngineBlock[] = []
  for (const b of originalBlocks) {
    if ((b as { hidden?: boolean }).hidden) hiddenOriginals.push(b)
    else visibleOriginals.push(b)
  }

  const pmParas = (pmDoc.content ?? []).filter(
    (n) =>
      n.type === 'paragraph' ||
      n.type === 'heading' ||
      n.type === 'listItem' ||
      n.type === 'blockquote' ||
      n.type === 'codeBlock' ||
      n.type === 'bulletList' ||
      n.type === 'orderedList',
  )
  // Top-level nodes outside this filter (tables, images, taskList roots,
 // …) are not yet round-trippable through pmDocToSavePlan. We log them
 // once so dev-time users see the gap; full table/image round-trip lands
 // in v0.7.28 alongside the pmDocToSavePlan table/image paths.
  // v0.7.27: tables / images / taskLists now have minimal round-trip
  // paths (XML shim that emits a valid <w:tbl>/<w:drawing>). Other
  // top-level node types fall through to the original-block passthrough
  // so we never silently lose content.
  const supportedNonPara = new Set(['table', 'image', 'taskList'])

  const usedDocxIndexes = new Set<number>()
  let i = 0
  const visibleNodes = (pmDoc.content ?? []).filter((n) => {
    if (supportedNonPara.has(n.type)) return true
    if (
      n.type === 'paragraph' ||
      n.type === 'heading' ||
      n.type === 'listItem' ||
      n.type === 'blockquote' ||
      n.type === 'codeBlock' ||
      n.type === 'bulletList' ||
      n.type === 'orderedList'
    ) {
      return true
    }
    return false
  })
  for (const node of visibleNodes) {
    // Table: emit minimal <w:tbl> as 'xml' SaveBlock (no anchor yet).
    if (node.type === 'table') {
      blocks.push({ kind: 'xml', xml: pmTableToTableXml(node) })
      continue
    }
    if (node.type === 'image') {
      const xml = pmImageToDrawingXml(node)
      if (xml) blocks.push({ kind: 'xml', xml })
      continue
    }
    const pmRuns = flattenNodeRuns(node)
    const text = pmRuns.map((r) => r.text).join('')
    const runSig = pmRuns.map((r) => runSignature(r)).join('|')

    const original = visibleOriginals[i]
    const origRuns = original ? collectRuns(original) : []
    const origText = origRuns.map((r) => r.text).join('')
    const origSig = origRuns.map((r) => runSignatureOfEngineRun(r)).join('|')

    const idx = original ? doc.parsed.blocks.indexOf(original) : -1
    if (idx >= 0) usedDocxIndexes.add(idx)

    if (!original) {
      // Brand-new block (user added a paragraph that has no original anchor).
      blocks.push({ kind: 'generated', block: pmNodeToGeneratedBlock(node, pmRuns) })
      i++
      continue
    }

    if (text === origText && runSig === origSig) {
      blocks.push({ kind: 'original', docxIndex: idx })
    } else if (
      text === origText &&
      text === '' &&
      origRuns.length === 0 &&
      pmRuns.length <= 1
    ) {
      // Truly untouched empty paragraph: keep original bytes.
      blocks.push({ kind: 'original', docxIndex: idx })
    } else if (runSig === origSig) {
      // Text-only edit: keep the original shell, only rewrite <w:t>.
      const xml = patchParagraphText(doc, idx, text).xml
      blocks.push({ kind: 'xml', xml, docxIndex: idx })
      textByIndex.set(idx, text)
    } else {
      // Edge case: blank-docx path. Original block has no runs and the PM
      // doc only carries unstyled text; signatures vacuously differ but
      // no inline format actually changed. Fall back to the text-only
      // patch path so we keep the original pPr/bookmarks/fields verbatim.
      if (
        origRuns.length === 0 &&
        pmRuns.every(
          (r) =>
            !r.bold &&
            !r.italic &&
            !r.underline &&
            !r.strike &&
            !r.code &&
            !r.link,
        )
      ) {
        const xml = patchParagraphText(doc, idx, text).xml
        blocks.push({ kind: 'xml', xml, docxIndex: idx })
        textByIndex.set(idx, text)
      } else {
        // Inline format (or run-count) changed: regenerate the paragraph.
        const generated = pmNodeToGeneratedBlock(node, pmRuns)
        const rawPPr = (original as { rawPPr?: string }).rawPPr
        if (rawPPr) (generated as { rawPPr?: string }).rawPPr = rawPPr
        blocks.push({ kind: 'generated', block: generated })
        textByIndex.set(idx, text)
      }
    }
    i++
  }

  for (let k = 0; k < visibleOriginals.length; k++) {
    const original = visibleOriginals[k]
    const idx = doc.parsed.blocks.indexOf(original)
    if (idx < 0 || usedDocxIndexes.has(idx)) continue
    blocks.push({ kind: 'original', docxIndex: idx })
  }

  for (const h of hiddenOriginals) {
    const idx = doc.parsed.blocks.indexOf(h)
    if (idx >= 0) blocks.push({ kind: 'original', docxIndex: idx })
  }

  return { blocks, textByIndex }
}

/** Map a TipTap paragraph/heading/list node to an engine GeneratedBlock. */
export function pmNodeToGeneratedBlock(node: PmNode, pmRuns?: PmRun[]): GeneratedBlock {
  const runs: Run[] = []
  const effectiveRuns = pmRuns ?? flattenNodeRuns(node)
  for (const r of effectiveRuns) {
    runs.push({
      text: r.text,
      bold: r.bold,
      italic: r.italic,
      underline: r.underline,
      strike: r.strike,
      link: r.link,
    })
  }
  let type: 'paragraph' | 'heading' | 'listItem' = 'paragraph'
  let level: number | undefined
  let list: GeneratedBlock['list']
  if (node.type === 'heading') {
    type = 'heading'
    level = Number(node.attrs?.level ?? 1)
  } else if (node.type === 'listItem') {
    type = 'listItem'
    list = inferListInfo(node)
  }
  return { type, level, list, runs }
}

function flattenNodeRuns(node: PmNode): PmRun[] {
  const out: PmRun[] = []
  const walk = (n: PmNode, inheritedMarks: PmMark[]): void => {
    if (n.type === 'text' && typeof n.text === 'string') {
      const marks = [...inheritedMarks, ...(n.marks ?? [])]
      const r: PmRun = { text: n.text }
      for (const m of marks) applyMark(r, m)
      out.push(r)
      return
    }
    const childMarks = mergeMarks(inheritedMarks, n.marks ?? [])
    for (const c of n.content ?? []) walk(c, childMarks)
  }
  walk(node, [])
  const coalesced: PmRun[] = []
  for (const r of out) {
    const last = coalesced[coalesced.length - 1]
    if (last && runSignature(last) === runSignature(r)) {
      last.text += r.text
    } else {
      coalesced.push({ ...r })
    }
  }
  return coalesced
}

function mergeMarks(parent: PmMark[], child: PmMark[]): PmMark[] {
  const map = new Map<string, PmMark>()
  for (const m of parent) map.set(m.type, m)
  for (const m of child) map.set(m.type, m)
  return Array.from(map.values())
}

function applyMark(r: PmRun, m: PmMark): void {
  switch (m.type) {
    case 'bold': r.bold = true; break
    case 'italic': r.italic = true; break
    case 'underline': r.underline = true; break
    case 'strike': r.strike = true; break
    case 'code': r.code = true; break
    case 'link':
      r.link = {
        href: String(m.attrs?.href ?? ''),
        tooltip: m.attrs?.title as string | undefined,
      }
      break
  }
}

function runSignature(r: PmRun): string {
  const parts = [
    r.bold ? 'b' : '',
    r.italic ? 'i' : '',
    r.underline ? 'u' : '',
    r.strike ? 's' : '',
    r.code ? 'c' : '',
    r.link?.href ?? '',
  ]
  return parts.join(',')
}

function runSignatureOfEngineRun(r: EngineRun): string {
  const parts = [
    r.bold ? 'b' : '',
    r.italic ? 'i' : '',
    r.underline ? 'u' : '',
    r.strike ? 's' : '',
    (r as { code?: boolean }).code ? 'c' : '',
    (r as { link?: { href: string } }).link?.href ?? '',
  ]
  return parts.join(',')
}

function inferListInfo(_node: PmNode): GeneratedBlock['list'] {
  // Sentinel numId; the engine's numbering reconciliation will rewrite it
  // to the real id when the GeneratedBlock declares `list`.
  return { kind: 'bullet', numId: '__pending__', ilvl: 0 }
}

// ============================================================================
// v0.7.27 — TipTap table -> OOXML <w:tbl> shim
// ============================================================================
//
// docx-engine ships a full TableModel -> OOXML generator (generateTableModelXml)
// but expects a richly typed model with column widths, borders, cell merges,
// etc. We don't model all of that in TipTap yet, so for new tables we emit
// a minimum-viable <w:tbl> with equal columns and 1pt borders. When the
// editor grows the surface area (column resize, merge, etc.), swap this for
// a real TableModel builder.
//
// This is intentionally limited to "user just inserted a table" so the saved
// .docx doesn't reject the file in Word; it is NOT pixel-perfect table fidelity.
function escapeXmlTextAttrSafe(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function paragraphRunsToXml(content: PmNode[] | undefined): string {
  if (!content) return ''
  return content
    .map((n) => {
      if (n.type !== 'text' || typeof n.text !== 'string') return ''
      return `<w:r><w:t xml:space="preserve">${escapeXmlTextAttrSafe(n.text)}</w:t></w:r>`
    })
    .join('')
}

function cellToXml(cell: PmNode, header: boolean): string {
  const isHeader = header || cell.type === 'tableHeader'
  const cellTag = isHeader ? 'w:tc' : 'w:tc'
  const paragraphs =
    (cell.content ?? [])
      .filter((p) => p.type === 'paragraph' || p.type === 'heading')
      .map((p) => `<w:p>${paragraphRunsToXml(p.content)}</w:p>`)
      .join('') || '<w:p/>'
  return `<${cellTag}><w:tcPr><w:tcW w:w="2000" w:type="dxa"/></w:tcPr>${paragraphs}</${cellTag}>`
}

/** Render a TipTap `table` node to a minimal <w:tbl>...</w:tbl> XML. */
export function pmTableToTableXml(node: PmNode): string {
  const rows = (node.content ?? []).filter((r) => r.type === 'tableRow')
  if (rows.length === 0) return ''
  const firstRow = rows[0]
  const cellCount = (firstRow.content ?? []).length || 1
  const totalWidth = 2000 * cellCount
  const gridCols = Array.from({ length: cellCount }, () => '<w:gridCol w:w="2000"/>').join('')
  const rowsXml = rows
    .map((r) => {
      const cells = (r.content ?? [])
        .filter((c) => c.type === 'tableCell' || c.type === 'tableHeader')
        .map((c) => cellToXml(c, r === firstRow))
        .join('')
      return `<w:tr>${cells}</w:tr>`
    })
    .join('')
  return (
    `<w:tbl>` +
    `<w:tblPr>` +
    `<w:tblW w:w="${totalWidth}" w:type="dxa"/>` +
    `<w:tblBorders>` +
    `<w:top w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `<w:left w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `<w:bottom w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `<w:right w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `<w:insideH w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `<w:insideV w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
    `</w:tblBorders>` +
    `<w:tblLook w:val="04A0"/>` +
    `</w:tblPr>` +
    `<w:tblGrid>${gridCols}</w:tblGrid>` +
    rowsXml +
    `</w:tbl>`
  )
}

/** Render a TipTap `image` node to a minimal <w:drawing> XML fragment. */
export function pmImageToDrawingXml(node: PmNode, embedId?: string): string {
  const src = String(node.attrs?.src ?? '')
  if (!src) return ''
  const alt = String(node.attrs?.alt ?? '')
  const widthPx = Number(node.attrs?.width ?? 400)
  const heightPx = Number(node.attrs?.height ?? 300)
  const cx = Math.round(widthPx * 9525)
  const cy = Math.round(heightPx * 9525)
  // embedId (e.g. "rId5") links the drawing to the binary image part via
  // [Content_Types].xml + word/_rels/document.xml.rels (see
  // embedImagesInDocx). When no embedId is provided the drawing still emits
  // a valid (but empty) <a:blip/> so the XML is well-formed.
  const blip = embedId
    ? `<a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="${embedId}"/>`
    : `<a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/>`
  return (
    `<w:p><w:r><w:drawing>` +
    `<wp:inline distT="0" distB="0" distL="0" distR="0">` +
    `<wp:extent cx="${cx}" cy="${cy}"/>` +
    `<wp:effectExtent l="0" t="0" r="0" b="0"/>` +
    `<wp:docPr id="1" name="Picture 1" descr="${escapeXmlTextAttrSafe(alt)}"/>` +
    `<wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr>` +
    `<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
    `<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
    `<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
    `<pic:nvPicPr><pic:cNvPr id="1" name="Picture 1"/><pic:cNvPicPr/></pic:nvPicPr>` +
    `<pic:blipFill>` +
    blip +
    `<a:stretch><a:fillRect/></a:stretch>` +
    `</pic:blipFill>` +
    `<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="${cx}" cy="${cy}"/></a:xfrm>` +
    `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>` +
    `</pic:pic>` +
    `</a:graphicData></a:graphic>` +
    `</wp:inline>` +
    `</w:drawing></w:r></w:p>`
  )
}

// ============================================================================
// v0.7.28 — DOC image binary embedding
// ============================================================================
//
// pmImageToDrawingXml emits a <w:drawing>/<pic:blipFill>/<a:blip/> skeleton
// with no embedded image bytes. To make the saved .docx self-contained we:
//   1. Walk the TipTap doc, collect any image node whose src is a data: URL,
//      assign it a stable filename (word/media/imageN.<ext>), and rewrite
//      the node's src to that filename.
//   2. Run saveDocx as usual (it writes the drawing XML referencing the
//      filename as an image part — saveDocx's existing rels machinery then
//      <Relationship Type="...image" Target="..."/>).
//   3. Post-process the saved .docx zip with JSZip: add the binary parts,
//      make sure the rels + content-types carry the new image entries,
//      and emit the patched bytes.
//
// This gives a true "upload → edit → download" round-trip for any .docx
// that contains images.

export interface DocxImageAsset {
  /** word/media/<file>, e.g. media/image1.png */
  filename: string
  /** data:<mime>;base64,... */
  dataUrl: string
}

/** Walk the TipTap doc, gather image dataURLs, and rewrite each node's
 *  src to the assigned filename. */
export function collectImagesFromPmDoc(pmDoc: PmNode): DocxImageAsset[] {
  const assets: DocxImageAsset[] = []
  let counter = 1
  const visit = (node: PmNode | null | undefined) => {
    if (!node) return
    if (node.type === 'image') {
      const src = String(node.attrs?.src ?? '')
      if (src.startsWith('data:')) {
        const m = /^data:image\/([a-zA-Z+]+);base64,(.*)$/.exec(src)
        if (m) {
          const ext = m[1] === 'jpeg' ? 'jpg' : m[1].toLowerCase()
          const filename = `media/image${counter++}.${ext}`
          assets.push({ filename, dataUrl: src })
          node.attrs = { ...node.attrs, src: filename }
        }
      }
    }
    if (node.content) for (const child of node.content) visit(child)
  }
  visit(pmDoc)
  return assets
}

function dataUrlToBytes(dataUrl: string): Uint8Array {
  const m = /^data:[^;]+;base64,(.*)$/.exec(dataUrl)
  if (!m) return new Uint8Array(0)
  const bin = atob(m[1])
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** Embed `assets` into a saved .docx byte stream:
 *   - writes word/media/<file> parts (with the raw image bytes)
 *   - appends <Relationship Type=".../image" Target="<filename>"/> to
 *     word/_rels/document.xml.rels (assigning fresh rId{N})
 *   - adds <Override PartName="/word/media/<filename>"/> + <Default
 *     Extension="..."/> to [Content_Types].xml
 * Returns the new .docx bytes (or the original if there are no assets). */
export async function embedImagesInDocx(
  bytes: Uint8Array,
  assets: DocxImageAsset[],
): Promise<Uint8Array> {
  if (assets.length === 0) return bytes
  const zip = await JSZip.loadAsync(bytes)

  // --- document.xml.rels: find the next free rId ---
  const relsPath = 'word/_rels/document.xml.rels'
  let relsRaw = await zip.file(relsPath)?.async('string') ?? ''
  const existingIds = new Set<string>()
  for (const m of relsRaw.matchAll(/Id="(rId\d+)"/g)) existingIds.add(m[1])
  let nextId = 1
  const nextRid = () => {
    while (existingIds.has(`rId${nextId}`)) nextId++
    const id = `rId${nextId++}`
    existingIds.add(id)
    return id
  }

  // --- [Content_Types].xml: track extension defaults ---
  const ctPath = '[Content_Types].xml'
  let ctRaw = await zip.file(ctPath)?.async('string') ?? ''
  const imageExts = new Set<string>()

  for (const asset of assets) {
    const ext = (asset.filename.split('.').pop() ?? '').toLowerCase()
    imageExts.add(ext)
    const partPath = `word/${asset.filename}`
    const partBytes = dataUrlToBytes(asset.dataUrl)
    if (partBytes.byteLength > 0) zip.file(partPath, partBytes)

    const rid = nextRid()
    const relEntry = `<Relationship Id="${rid}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="${asset.filename}"/>`
    if (relsRaw.includes('</Relationships>')) {
      relsRaw = relsRaw.replace('</Relationships>', relEntry + '</Relationships>')
    } else {
      relsRaw = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${relEntry}</Relationships>`
    }

    const overrideEntry = `<Override PartName="/${partPath}" ContentType="image/${ext === 'jpg' ? 'jpeg' : ext}"/>`
    if (ctRaw.includes('</Types>') && !ctRaw.includes(overrideEntry)) {
      ctRaw = ctRaw.replace('</Types>', overrideEntry + '</Types>')
    }
  }

  // Add Default Extension entries for each new image type if missing
  for (const ext of imageExts) {
    const ct = ext === 'jpg' ? 'image/jpeg' : `image/${ext}`
    const def = `<Default Extension="${ext === 'jpg' ? 'jpg' : ext}" ContentType="${ct}"/>`
    if (!ctRaw.includes(def) && ctRaw.includes('<Types ')) {
      ctRaw = ctRaw.replace(/(<Types[^>]*>)/, `$1${def}`)
    }
  }

  zip.file(relsPath, relsRaw)
  zip.file(ctPath, ctRaw)

  return new Uint8Array(
    await zip.generateAsync({ type: 'uint8array', compression: 'DEFLATE' }),
  )
}


// ============================================================================
// v0.7.28 — high-level save: collect images + embed bytes into the .docx
// ============================================================================
//
// Walks the TipTap doc, collects image dataURLs, assigns them filenames,
// calls saveDocx, then post-processes the bytes to write the image parts +
// relationship entries + content-type overrides. Use this in place of
// raw saveDocx() when the doc may contain images.
export async function saveDocxBytesWithImages(
  doc: DocxAdapterDocument,
  pmDoc: PmNode,
): Promise<Uint8Array> {
  // 1) Collect image dataURLs and rewrite node.src to filenames
  const assets = collectImagesFromPmDoc(pmDoc)
  // 2) Save the .docx (the drawing XML now references filenames like
  //    "media/image1.png" which don't exist as parts yet)
  const baseBytes = await saveDocxBytes(doc)
  // 3) Add the binary parts + rels + content-type overrides
  return embedImagesInDocx(baseBytes, assets)
}
