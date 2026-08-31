/**
 * PptxShapeAdapter — v0.7.27 飞书级 PPT 编辑器后端。
 *
 * 通过 genoffice 的 pptx-engine 解析上传的 .pptx 字节，把每张 slide
 * 抽取为 `PptxShape[]`（Konva 可绘制的最小模型：text/rect/ellipse/line/
 * picture）。保存时通过同一引擎的 savePptx 把 shape 状态重新序列化回 .pptx，
 * 保留原始未触碰的字节（fill 路径走 byte-identical no-edit round-trip）。
 *
 * 与 `pptxAdapter.ts`（pptxgenjs 文本列表 MVP）的区别：
 *   - 本模块产出形状级数据，Konva 可直接渲染；
 *   - pptxAdapter 仍是 fallback，给只想要 title+bullets 的场景用。
 */
import {
  openPptx as engineOpenPptx,
  savePptx as engineSavePptx,
  createBlankPptx as engineCreateBlankPptx,
  addTable as engineAddTable,
  getSlideNotes as engineGetSlideNotes,
  setSlideNotes as engineSetSlideNotes,
  type OpenedPptx,
  type Slide,
  type TextElement,
  type PictureElement,
  type TableElement,
} from '../engines/pptx-engine/index'

export interface PptxShape {
  /** Unique id within a slide; stable across edits. */
  id: string
  type:
    | 'text'
    | 'rect'
    | 'roundRect'
    | 'ellipse'
    | 'line'
    | 'arrow'
    | 'triangle'
    | 'star'
    | 'hexagon'
    | 'callout'
    | 'table'
    | 'picture'
  /** Position + size in EMU (1 inch = 914400 EMU). */
  x: number
  y: number
  w: number
  h: number
  /** Text content (for text/rect/ellipse with text). */
  text?: string
  /** Hex color without '#'. */
  fill?: string
  /** Hex color without '#'. */
  stroke?: string
  /** Stroke width in EMU. */
  strokeWidth?: number
  /** Font size in points. */
  fontSize?: number
  /** Picture media ref (when type === 'picture'). */
  mediaRef?: string
  /** base64 dataURL for picture (resolved at openPptx time). */
  mediaData?: string
  /** Original element index inside the slide's <p:spTree>. Used for save. */
  spIndex: number
  /** Source element type from the engine (used to skip passthrough on save). */
  sourceType?: string
  /** Preset geometry (rect/ellipse/roundRect/…). */
  preset?: string
  /** Table cells (row-major): cellTexts[r][c] = string. Only set when type === 'table'. */
  cellTexts?: string[][]
  /** Number of rows / columns (table only). */
  rows?: number
  cols?: number
}

export interface PptxShapeSlide {
  /** Slide index (0-based). */
  index: number
  /** Slide size in EMU. */
  width: number
  height: number
  /** Background hex color (if any). */
  background?: string
  /** Shape list (renderable). */
  shapes: PptxShape[]
  /** Original slide model (kept so savePptx can emit byte-identical bytes
   *  for untouched shapes). The editor flips `dirty = true` on the
   *  corresponding element when a shape is mutated. */
  raw: Slide
  /** Speaker notes for this slide (text only, \\n-separated paragraphs). */
  notes?: string
}

export interface PptxShapeDeck {
  slides: PptxShapeSlide[]
  /** Original bytes for byte-identical no-edit round-trip. */
  originalBytes: Uint8Array | null
  /** Engine handle — required to call savePptx. Null when the deck was
   *  built from scratch (createBlank). */
  opened: OpenedPptx | null
}

const EMU_PER_INCH = 914400

function hexFromResolved(c: string | undefined): string | undefined {
  if (!c) return undefined
  if (c.startsWith('#')) return c.slice(1)
  // already without '#'
  if (/^[0-9a-fA-F]{6}$/.test(c)) return c.toUpperCase()
  return undefined
}

function emuTransformToRect(t: { offset?: { x: number; y: number; cx: number; cy: number } } | undefined): {
  x: number
  y: number
  w: number
  h: number
} | null {
  if (!t || !t.offset) return null
  return { x: t.offset.x, y: t.offset.y, w: t.offset.cx, h: t.offset.cy }
}

function shapeFromTextElement(el: TextElement): PptxShape | null {
  const rect = emuTransformToRect(el.transform)
  if (!rect) return null
  // Extract first-paragraph text (multi-paragraph becomes joined by \n).
  const text = (el.text?.paragraphs ?? [])
    .map((p) => (p.runs ?? []).map((r) => r.text ?? '').join(''))
    .join('\n')
  const preset = el.presetGeometry ?? 'rect'
  let shapeType: PptxShape['type'] = 'rect'
  if (preset === 'ellipse') shapeType = 'ellipse'
  else if (preset === 'line' || preset === 'straightConnector' || preset === 'bentConnector3' || preset === 'curvedConnector3') shapeType = 'line'
  else if (preset === 'roundRect') shapeType = 'roundRect'
  else if (preset === 'rect') shapeType = 'rect'
  else if (
    preset === 'rightArrow' ||
    preset === 'leftArrow' ||
    preset === 'upArrow' ||
    preset === 'downArrow' ||
    preset === 'leftRightArrow' ||
    preset === 'upDownArrow' ||
    preset === 'bentArrow' ||
    preset === 'curvedRightArrow' ||
    preset === 'circularArrow'
  ) shapeType = 'arrow'
  else if (preset === 'triangle' || preset === 'rtTriangle' || preset === 'isoscelesTriangle') shapeType = 'triangle'
  else if (
    preset === 'star4' ||
    preset === 'star5' ||
    preset === 'star6' ||
    preset === 'star8' ||
    preset === 'star10' ||
    preset === 'star12' ||
    preset === 'star16' ||
    preset === 'star24' ||
    preset === 'star32'
  ) shapeType = 'star'
  else if (
    preset === 'hexagon' ||
    preset === 'octagon' ||
    preset === 'pentagon' ||
    preset === 'heptagon'
  ) shapeType = 'hexagon'
  else if (
    preset === 'callout1' ||
    preset === 'callout2' ||
    preset === 'callout3' ||
    preset === 'wedgeRectCallout' ||
    preset === 'wedgeRoundRectCallout' ||
    preset === 'wedgeEllipseCallout' ||
    preset === 'cloudCallout' ||
    preset === 'cloud' ||
    preset === 'speechBubble'
  ) shapeType = 'callout'
  else shapeType = 'text' // text-only box or unknown preset
  // First run carries the styling.
  const firstRun = el.text?.paragraphs?.[0]?.runs?.[0]
  return {
    id: el.id,
    type: shapeType,
    x: rect.x,
    y: rect.y,
    w: rect.w,
    h: rect.h,
    text,
    fill: hexFromResolved((el.fill as { color?: string } | undefined)?.color),
    stroke: hexFromResolved((el.stroke as { color?: string } | undefined)?.color),
    strokeWidth: el.stroke?.width,
    fontSize: firstRun?.fontSize ? Math.round(firstRun.fontSize / 100) : 18,
    spIndex: el.anchor?.spIndex ?? -1,
    sourceType: el.type,
    preset,
  }
}

function shapeFromPicture(el: PictureElement, media: Map<string, string>): PptxShape | null {
  const rect = emuTransformToRect(el.transform)
  if (!rect) return null
  return {
    id: el.id,
    type: 'picture',
    x: rect.x,
    y: rect.y,
    w: rect.w,
    h: rect.h,
    mediaRef: el.mediaRef,
    mediaData: media.get(el.mediaRef),
    spIndex: el.anchor?.spIndex ?? -1,
    sourceType: el.type,
  }
}

/** Open a .pptx into a shape-level deck ready for Konva rendering. */
export async function openPptxShapes(bytes: Uint8Array): Promise<PptxShapeDeck> {
  const opened = await engineOpenPptx(bytes)
  const slides: PptxShapeSlide[] = []
  // Build a mediaRef -> dataURL map by reading the package's media parts.
  const media: Map<string, string> = new Map()
  for (const slide of opened.deck.slides) {
    for (const el of slide.elements ?? []) {
      const pic = el as PictureElement
      if (pic.type === 'picture' && pic.mediaRef) {
        try {
          const blob = opened.archive.entries.get(pic.mediaRef)
          if (blob) {
            const ext = (pic.mediaRef.split('.').pop() ?? '').toLowerCase()
            const mime = ext === 'png' ? 'image/png'
              : ext === 'jpg' || ext === 'jpeg' ? 'image/jpeg'
              : ext === 'gif' ? 'image/gif'
              : ext === 'webp' ? 'image/webp'
              : ext === 'svg' ? 'image/svg+xml'
              : 'application/octet-stream'
            // Re-encode to base64 for Konva.Image's dataURL src.
            let bin = ''
            for (let i = 0; i < blob.byteLength; i++) bin += String.fromCharCode(blob[i])
            media.set(pic.mediaRef, `data:${mime};base64,${btoa(bin)}`)
          }
        } catch {
          /* skip unreadable media */
        }
      }
    }
  }
  for (let i = 0; i < opened.deck.slides.length; i++) {
    const slide = opened.deck.slides[i]
    const shapes: PptxShape[] = []
    for (const el of slide.elements ?? []) {
      const t = el as TextElement
      const p = el as PictureElement
      if (t.type === 'text' || t.type === 'shape') {
        const s = shapeFromTextElement(t)
        if (s) shapes.push(s)
      } else if (p.type === 'picture') {
        const s = shapeFromPicture(p, media)
        if (s) shapes.push(s)
      } else if (t.type === 'table') {
        const s = shapeFromTable(t as unknown as TableElement)
        if (s) shapes.push(s)
      }
      // tables/charts/groups/passthrough: skip for now (would need
      // dedicated renderers; v0.7.27 ships text/rect/ellipse/line/picture).
    }
    slides.push({
      index: i,
      width: opened.deck.size.cx,
      height: opened.deck.size.cy,
      background: hexFromResolved(
(slide.background as { color?: string } | undefined)?.color,
      ),
      shapes,
      raw: slide,
      notes: readSlideNotesSafe(opened, slide.path),
    })
  }
  return {
    slides,
    originalBytes: new Uint8Array(bytes),
    opened,
  }
}

/** Build a fresh shape deck (one blank slide) using the engine. */
export async function newPptxShapeDeck(): Promise<PptxShapeDeck> {
  const bytes = await engineCreateBlankPptx()
  return openPptxShapes(bytes)
}

/** Helper: convert EMU to inches (used by Konva for readable sizes). */
export function emuToInch(emu: number): number {
  return emu / EMU_PER_INCH
}

/** Helper: convert EMU to px at 96 dpi (CSS-px roughly equivalent). */
export function emuToPx(emu: number): number {
  return (emu / EMU_PER_INCH) * 96
}

/** Serialize the shape deck back to .pptx bytes via pptx-engine. The
 *  caller must mutate `deck.opened.deck.slides[i].elements[j].dirty = true`
 *  on each shape that changed before invoking this; untouched shapes
 *  round-trip byte-identical. */
export async function savePptxShapeBytes(deck: PptxShapeDeck): Promise<Uint8Array> {
  if (!deck.opened) {
    // No engine handle: nothing was opened. Build a fresh deck via the engine.
    const fresh = await engineCreateBlankPptx()
    const freshOpened = await engineOpenPptx(fresh)
    deck.opened = freshOpened
  }
  return engineSavePptx(deck.opened)
}

/** Build a PptxShape from a parsed pptx-engine GraphicFrameElement of
 *  kind === 'table'. We extract cell texts in row-major order so the
 *  Konva renderer can show them; structural edits flow back through
 *  engineAddTable (insertion) and `engine.pptx-engine` cell-level patches
 *  on save (see markDirty). */
function shapeFromTable(el: TableElement): PptxShape | null {
  const rect = emuTransformToRect(el.transform)
  if (!rect) return null
  const rows = el.rowHeights.length
  const cols = el.colWidths.length
  const cellTexts: string[][] = el.rows.map((row) =>
    row.map((cell) => {
      const paras = cell.text?.paragraphs ?? []
      return paras.map((p) => (p.runs ?? []).map((r) => r.text ?? '').join('')).join('\n')
    }),
  )
  return {
    id: el.id,
    type: 'table',
    x: rect.x,
    y: rect.y,
    w: rect.w,
    h: rect.h,
    rows,
    cols,
    cellTexts,
    spIndex: el.anchor?.spIndex ?? -1,
    sourceType: el.type,
    preset: 'table',
  }
}

/** Add a new table to a slide via the pptx-engine's addTable API; the
 *  caller's Yjs layer wraps the returned element id so peers converge. */
export function addTableToSlide(
  deck: PptxShapeDeck,
  slideIndex: number,
  rows: number,
  cols: number,
  offset: { x: number; y: number; w: number; h: number },
): PptxShape | null {
  if (!deck.opened) return null
  const r = engineAddTable(deck.opened, slideIndex, {
    rows,
    cols,
    offset: { x: offset.x, y: offset.y, cx: offset.w, cy: offset.h },
  })
  if (!r) return null
  return {
    id: r.elementId,
    type: 'table',
    x: offset.x,
    y: offset.y,
    w: offset.w,
    h: offset.h,
    rows,
    cols,
    cellTexts: Array.from({ length: rows }, () => Array.from({ length: cols }, () => '')),
    spIndex: r.slide.elements.length - 1,
    sourceType: 'graphicFrame',
    preset: 'table',
  }
}


/** Read a slide's speaker notes via the pptx-engine notes API. Returns
 *  empty string if the engine handle is missing or notes are absent. */
function readSlideNotesSafe(opened: OpenedPptx | null, slidePath: string): string {
  if (!opened || !opened.archive) return ''
  try {
    return engineGetSlideNotes(opened.archive, slidePath)
  } catch {
    return ''
  }
}

/** Persist speaker notes for a slide via the pptx-engine notes API.
 *  Returns true on success, false on engine-handle-missing or failure. */
export function setSlideNotesOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  notes: string,
): boolean {
  if (!deck.opened) return false
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return false
  try {
    engineSetSlideNotes(deck.opened, slide.path, notes)
    const idx = deck.slides.findIndex((s) => s.index === slideIndex)
    if (idx >= 0) deck.slides[idx].notes = notes
    return true
  } catch {
    return false
  }
}
