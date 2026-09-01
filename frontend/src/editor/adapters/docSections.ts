/**
 * docSections — DOC multi-section helpers (v0.7.72).
 *
 * Vendored from genoffice `packages/docx-engine/src/section.ts` —
 * the engine-level export is reused as-is (readSections, applySectionSettings,
 * applyPageNumType, applySectionStartType); this adapter only adds UI-friendly
 * helpers the editor component needs (no XML in component code).
 *
 * Adapted: genoffice's editor used a custom "docHeading" tip; we map onto
 * the engine's `SectionInfo` / `SectionSettings` types so the UI can stay
 * declarative and persist via saveDocx section patches.
 */
import type { ParsedDoc } from '../engines/docx-engine/types'
import type { SectionInfo, SectionSettings } from '../engines/docx-engine/types'
import { readSections, DEFAULT_SECTION } from '../engines/docx-engine/section'

export interface DocSectionSummary {
  /** 0-based section index in document order */
  index: number
  /** section-break type; 'nextPage' is the most common (starts a new page) */
  startType: SectionInfo['startType']
  /** docxIndex range — useful for "apply to selection" matching */
  firstBlockIndex: number
  lastBlockIndex: number
  /** whether this section has a different-first-page header/footer (w:titlePg) */
  titlePg: boolean
  /** page number renumbering (start value) if set; undefined = continue */
  pageNumberStart?: number
  /** page number format (decimal / lowerRoman / upperLetter …) */
  pageNumberFmt?: string
  /** raw settings used by format helpers (paper / margins / orientation / columns) */
  settings: SectionSettings
}

export type UnitSystem = 'twips' | 'inches' | 'mm'

/** Read all sections in document order, with a copy of settings per section. */
export function getDocumentSections(parsed: ParsedDoc): DocSectionSummary[] {
  return readSections(parsed).map((s, i) => ({
    index: i,
    startType: s.startType,
    firstBlockIndex: s.firstBlockIndex,
    lastBlockIndex: s.lastBlockIndex,
    titlePg: s.titlePg,
    pageNumberStart: s.pageNumberStart,
    pageNumberFmt: s.pageNumberFmt,
    settings: { ...s.settings },
  }))
}

/** Which section owns a given docxIndex (0-based). Returns -1 when parsed is empty. */
export function findSectionOfBlock(sections: DocSectionSummary[], blockIndex: number): number {
  if (sections.length === 0) return -1
  for (let i = 0; i < sections.length; i++) {
    const s = sections[i]!
    if (blockIndex >= s.firstBlockIndex && blockIndex <= s.lastBlockIndex) return i
  }
  return sections.length - 1
}

/** Whether a section is portrait. (We treat UNKNOWN as portrait = safe default.) */
export function isPortrait(s: SectionSettings): boolean {
  return s.orientation !== 'landscape'
}

/** Paper size label (US Letter / A4 / A3 / Legal / Custom). Best-effort guess. */
export function paperLabel(s: SectionSettings): string {
  const w = s.pageWidth
  const h = s.pageHeight
  // Values from genoffice packages/docx-engine/types
  const LETTER = [12240, 15840]
  const A4 = [11906, 16838]
  const A3 = [16838, 23811]
  const LEGAL = [12240, 20160]
  const sorted = w <= h ? [w, h] : [h, w]
  if (sorted[0] === LETTER[0] && sorted[1] === LETTER[1]) return 'US Letter'
  if (sorted[0] === A4[0] && sorted[1] === A4[1]) return 'A4'
  if (sorted[0] === A3[0] && sorted[1] === A3[1]) return 'A3'
  if (sorted[0] === LEGAL[0] && sorted[1] === LEGAL[1]) return 'US Legal'
  return `${(w / 1440).toFixed(2)}×${(h / 1440).toFixed(2)} in`
}

/** Convert twips → inches (or mm). 1 twip = 1/1440 inch. */
export function fromTwips(twips: number, unit: UnitSystem = 'inches'): number {
  if (unit === 'twips') return twips
  const inches = twips / 1440
  if (unit === 'mm') return +(inches * 25.4).toFixed(2)
  return +inches.toFixed(3)
}

/** Convert inches (or mm) → twips, with sensible bounds. */
export function toTwips(value: number, unit: UnitSystem = 'inches'): number {
  let inches = value
  if (unit === 'mm') inches = value / 25.4
  else if (unit === 'twips') return Math.round(value)
  return Math.max(0, Math.round(inches * 1440))
}

/** Default section settings factory — useful for "insert section break" UI. */
export function defaultSectionSettings(): SectionSettings {
  return { ...DEFAULT_SECTION }
}

/** True when the two settings describe the same paper & orientation. */
export function samePaper(a: SectionSettings, b: SectionSettings): boolean {
  return a.pageWidth === b.pageWidth && a.pageHeight === b.pageHeight && a.orientation === b.orientation
}

/** True when the two settings describe the same margins. */
export function sameMargins(a: SectionSettings, b: SectionSettings): boolean {
  return (
    a.marginTop === b.marginTop &&
    a.marginBottom === b.marginBottom &&
    a.marginLeft === b.marginLeft &&
    a.marginRight === b.marginRight &&
    (a.headerDist ?? 720) === (b.headerDist ?? 720) &&
    (a.footerDist ?? 720) === (b.footerDist ?? 720)
  )
}

/** Human-readable summary line for sidebar/toolbar display. */
export function formatSectionSummary(s: DocSectionSummary): string {
  const orient = isPortrait(s.settings) ? '纵向' : '横向'
  const paper = paperLabel(s.settings)
  const cols = s.settings.columns > 1 ? ` · ${s.settings.columns}栏` : ''
  const tp = s.titlePg ? ' · 首页独立' : ''
  const pn = s.pageNumberStart != null ? ` · 第${s.pageNumberStart}页起` : ''
  return `第${s.index + 1}节 · ${paper} · ${orient}${cols}${tp}${pn}`
}

/** Total section count shortcut. */
export function sectionCount(parsed: ParsedDoc): number {
  return readSections(parsed).length
}
