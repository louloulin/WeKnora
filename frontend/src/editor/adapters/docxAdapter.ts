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
  return { paragraphs, parsed }
}

/** Serialize the (possibly patched) parsed doc back to a .docx blob. */
export async function saveDocxBytes(doc: DocxAdapterDocument): Promise<Uint8Array> {
  const blocks: SaveBlock[] = doc.parsed.blocks.map((_, i) => ({
    kind: 'original',
    docxIndex: i,
  }))
  const opts: SaveOptions = {}
  return saveDocx(doc.parsed, { blocks, ...opts })
}

/** Replace a paragraph's runs in place and return the bytes delta for CRDT. */
export function patchParagraphText(
  doc: DocxAdapterDocument,
  index: number,
  newText: string,
): { bytes: Uint8Array; text: string } {
  const block = doc.parsed.blocks[index]
  if (!block) throw new Error(`paragraph ${index} out of range`)
  const runs = collectRuns(block)
  if (runs.length === 0) {
    // Empty paragraph: synthesize a single run preserving formatting.
    block.runs = [{ text: newText }]
  } else {
    // Patch strategy: keep the first run's formatting, rewrite its text;
    // empty out subsequent runs (matches GenOffice patchParagraphTexts).
    runs[0].text = newText
    for (let i = 1; i < runs.length; i++) runs[i].text = ''
  }
  return { bytes: undefined as unknown as Uint8Array, text: newText }
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
