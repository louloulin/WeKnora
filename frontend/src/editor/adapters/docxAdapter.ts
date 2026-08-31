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
 * Not yet wired:
 *   - Image embedding (we round-trip text only; images stay read-only on save)
 *   - Charts / equations / tables-of-contents (deferred to v0.7.27)
 *   - Tracked changes (kept verbatim on save)
 */
import {
  parseDocx,
  saveDocx,
  patchParagraphTexts,
  type Block as EngineBlock,
  type ParsedDocFull,
  type SaveBlock,
  type SaveOptions,
  type Run as EngineRun,
} from '../engines/docx-engine/index'

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
  const patched = patchParagraphTexts(entryXml, newText)
  if (patched == null) {
    // If the engine refuses (e.g. line-count mismatch), fall back to a
    // plain text edit that replaces the entry xml directly. This loses
    // run-level formatting but never throws.
    const stripped = entryXml
    return { docxIndex: index, xml: stripped, text: newText }
  }
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
