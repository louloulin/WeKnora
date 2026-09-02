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
  getSlideComments as engineGetSlideComments,
  addSlideComment as engineAddSlideComment,
  deleteSlideComment as engineDeleteSlideComment,
  addChart as engineAddChart,
  getSlideAnimations as engineGetSlideAnimations,
  setSlideAnimations as engineSetSlideAnimations,
  listMasterParts as engineListMasterParts,
  parseMasterPart as engineParseMasterPart,
  applyThemeToArchive as engineApplyThemeToArchive,
  remapDeckColors as engineRemapDeckColors,
  shouldOfferBuiltinLayouts as engineShouldOfferBuiltinLayouts,
  builtinLayoutInfos as engineBuiltinLayoutInfos,
  ensureBuiltinLayout as engineEnsureBuiltinLayout,
  BUILTIN_LAYOUTS,
  type OpenedPptx,
  type Slide,
  type TextElement,
  type PictureElement,
  type TableElement,
  type NewChartKind,
  type ThemeSpec as EngineThemeSpec,
  type SlideLayoutInfo as EngineSlideLayoutInfo,
  type SlideSize as EngineSlideSize,
  type MasterPartInfo as EngineMasterPartInfo,
  type BuiltinLayoutDef,
  setSlideTransition as engineSetSlideTransition,
  getSlideTransition as engineGetSlideTransition,
  insertBlankSlide as engineInsertBlankSlide,
  addElement as engineAddElement,
  setSlideLayout as engineSetSlideLayout,
  resetSlideLayout as engineResetSlideLayout,
  listSlideLayouts as engineListSlideLayouts,
} from '../engines/pptx-engine/index'
import type { NewElementOptions } from '../engines/pptx-engine/index'
import type { SlideTransitionKind } from '../engines/pptx-engine/generate'

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
  /** Rotation in degrees (0-360). Applied around the shape center. */
  rotation?: number
  /** v0.7.104 — group id. Shapes sharing the same non-empty groupId are
   *  treated as a single group in the editor (multi-select, joint resize,
   *  unified bbox). Persisted in memory; PPTX round-trip ignores the field
   *  (genoffice writes a real <p:grpSp> when groupElements() is called,
   *  which is the persistence path for v0.7.107+). */
  groupId?: string
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

/** Insert a valid blank slide after the requested slide. */
export function insertBlankSlideOnDeck(deck: PptxShapeDeck, sourceIndex: number): Slide | null {
  if (!deck.opened) return null
  return engineInsertBlankSlide(deck.opened, sourceIndex)
}

/** Add a shape through the engine so its original OOXML anchor is saveable. */
export function addShapeOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  shapeType: PptxShape['type'],
  offset: { x: number; y: number; cx: number; cy: number },
): PptxShape | null {
  const slide = deck.opened?.deck.slides[slideIndex]
  if (!slide) return null
  const kindByType: Partial<Record<PptxShape['type'], string>> = {
    text: 'textbox',
    rect: 'rect',
    roundRect: 'roundRect',
    ellipse: 'ellipse',
    line: 'line',
    arrow: 'rightArrow',
    triangle: 'triangle',
    star: 'star5',
    hexagon: 'hexagon',
    callout: 'callout1',
  }
  const kind = kindByType[shapeType]
  if (!kind) return null
  const options: NewElementOptions = {
    kind,
    offset,
    paragraphs: shapeType === 'text' ? [{ runs: [{ text: '双击编辑文本' }] }] : [{ runs: [{ text: '' }] }],
    ...(shapeType === 'rect' ? { fillColor: '#3B82F6' } : {}),
    ...(shapeType === 'ellipse' ? { fillColor: '#10B981' } : {}),
    ...(shapeType === 'line' ? { stroke: { color: '#111827', widthEmu: 12700 } } : {}),
  }
  const element = engineAddElement(slide, options)
  const shape = shapeFromTextElement(element)
  if (!shape) return null
  return { ...shape, type: shapeType, rotation: 0 }
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
    engineSetSlideNotes(deck.opened, slideIndex, notes)
    const idx = deck.slides.findIndex((s) => s.index === slideIndex)
    if (idx >= 0) deck.slides[idx].notes = notes
    return true
  } catch {
    return false
  }
}

// ----------------------------------------------------------------------------
// v0.7.32 — Slide comments (OOXML <p:cmLst>) — file-level review comments
// that survive a round-trip through PowerPoint.
// ----------------------------------------------------------------------------

export interface SlideCommentRecord {
  /** OOXML author id (per ppt/commentAuthors.xml). */
  authorId: number
  /** Display name shown in PowerPoint. */
  author: string
  initials: string
  /** 1-based per-author comment number. */
  idx: number
  /** Comment body text. */
  text: string
  /** ISO timestamp (PowerPoint stores it as the dt attribute). */
  date: string
}

/** Return the file-level comments for a slide (OOXML <p:cmLst>). */
export function getSlideCommentsOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
): SlideCommentRecord[] {
  if (!deck.opened) return []
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return []
  const raw = engineGetSlideComments(deck.opened.archive, slide.path)
  return raw.map((c) => ({
    authorId: c.authorId,
    author: c.author,
    initials: c.initials,
    idx: c.idx,
    text: c.text,
    date: c.dt,
  }))
}

/** Add a new file-level comment to a slide. Returns the new comment, or
 *  null on failure / engine-handle-missing. */
export function addSlideCommentOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  opts: { author: string; initials?: string; text: string },
): SlideCommentRecord | null {
  if (!deck.opened) return null
  if (!opts.text.trim()) return null
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return null
  try {
    const added = engineAddSlideComment(deck.opened, slideIndex, opts)
    if (!added) return null
    return {
      authorId: added.authorId,
      author: added.author,
      initials: added.initials,
      idx: added.idx,
      text: added.text,
      date: added.dt,
    }
  } catch {
    return null
  }
}

/** Delete a file-level comment by (authorId, idx). Returns true on success. */
export function deleteSlideCommentOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  ref: { authorId: number; idx: number },
): boolean {
  if (!deck.opened) return false
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return false
  try {
    return engineDeleteSlideComment(deck.opened, slideIndex, ref)
  } catch {
    return false
  }
}

// ----------------------------------------------------------------------------
// v0.7.32 — PPT charts (engine.addChart). The engine handles part writes,
// content-type overrides, slide rels, and graphicFrame emission.
// ----------------------------------------------------------------------------

export interface NewChartOptions {
  kind: NewChartKind
  title?: string
  categories: string[]
  series: Array<{ name: string; values: number[] }>
  /** Top-left + size in EMU. */
  offset: { x: number; y: number; cx: number; cy: number }
  legendPos?: 'b' | 't' | 'r' | 'l' | 'none'
  dataLabels?: boolean
  gridlines?: boolean
  catAxisTitle?: string
  valAxisTitle?: string
}

/** Insert a chart into the given slide. Returns the new shape id (matches
 *  the engine's elementId) so the caller can track selection. */
export function addChartToSlide(
  deck: PptxShapeDeck,
  slideIndex: number,
  opts: NewChartOptions,
): string | null {
  if (!deck.opened) return null
  if (!opts.categories.length || !opts.series.length) return null
  const out = engineAddChart(deck.opened, slideIndex, opts)
  if (!out) return null
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return null
  if (deck.slides[slideIndex]) {
    deck.slides[slideIndex].shapes.push({
      id: out.elementId,
      type: 'rect',
      x: opts.offset.x,
      y: opts.offset.y,
      w: opts.offset.cx,
      h: opts.offset.cy,
      fill: '#ffffff',
      stroke: '#7d8590',
      text: opts.title || `${opts.kind} chart`,
      spIndex: slide.elements.length - 1,
      sourceType: 'graphicFrame',
      preset: 'chart',
    })
  }
  return out.elementId
}

// ----------------------------------------------------------------------------
// v0.7.32 — PPT slide animations (entrance / emphasis / exit effects).
// ----------------------------------------------------------------------------

export type AnimEffectKind =
  | 'fade' | 'flyIn' | 'zoom' | 'spin' | 'bounce' | 'appear'
  | 'disappear' | 'pulse' | 'colorPulse' | 'teeter' | 'growShrink'

export type AnimTrigger = 'onClick' | 'withPrevious' | 'afterPrevious'

export interface SlideAnimationRecord {
  spId: number
  effect: AnimEffectKind
  trigger: AnimTrigger
  durationMs: number
  delayMs: number
}

const EFFECT_TO_ENGINE: Record<string, string> = {
  fade: 'fade',
  flyIn: 'flyIn',
  zoom: 'zoom',
  spin: 'spin',
  bounce: 'bounce',
  appear: 'appear',
  disappear: 'disappear',
  pulse: 'pulse',
  colorPulse: 'colorPulse',
  teeter: 'teeter',
  growShrink: 'growShrink',
}

/** Read the animations currently set on a slide. */
export function getSlideAnimationsOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
): SlideAnimationRecord[] {
  if (!deck.opened) return []
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return []
  const raw = engineGetSlideAnimations(slide)
  return raw
    .filter((a) => typeof (a as any).spid === 'number')
    .map((a) => {
      const effectRaw = ((a as any).effect ?? 'appear') as string
      const triggerRaw = ((a as any).trigger ?? 'onClick') as string
      return {
        spId: (a as any).spid as number,
        effect: (EFFECT_TO_ENGINE[effectRaw] ?? effectRaw) as AnimEffectKind,
        trigger: (triggerRaw === 'withPrev' ? 'withPrevious'
                : triggerRaw === 'afterPrev' ? 'afterPrevious'
                : 'onClick') as AnimTrigger,
        durationMs: (a as any).durationMs ?? 1000,
        delayMs: (a as any).delayMs ?? 0,
      }
    })
}

/** Replace the animation list on a slide. Returns true on success. */
export function getSlideTransitionOnDeck(deck: PptxShapeDeck, slideIndex: number): SlideTransitionKind {
  const opened = deck.opened
  if (!opened) return 'none'
  const slide = opened.deck.slides[slideIndex]
  if (!slide) return 'none'
  return engineGetSlideTransition(slide)
}

export function setSlideTransitionOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  kind: SlideTransitionKind,
): void {
  const opened = deck.opened
  if (!opened) return
  const slide = opened.deck.slides[slideIndex]
  if (!slide) return
  engineSetSlideTransition(slide, kind)
}

export function setSlideAnimationsOnDeck(
  deck: PptxShapeDeck,
  slideIndex: number,
  anims: SlideAnimationRecord[],
): boolean {
  if (!deck.opened) return false
  const slide = deck.opened.deck.slides[slideIndex]
  if (!slide) return false
  const engineAnims = anims.map((a) => ({
    spid: a.spId,
    effect: a.effect,
    trigger: a.trigger === 'withPrevious' ? 'withPrev'
            : a.trigger === 'afterPrevious' ? 'afterPrev'
            : 'onClick',
    durationMs: a.durationMs,
    delayMs: a.delayMs,
    paragraph: undefined as number | undefined,
    autoRev: undefined as boolean | undefined,
    nodeIdx: 0,
  }))
  try {
    engineSetSlideAnimations(slide, engineAnims as unknown as any)
    return true
  } catch {
    return false
  }
}

// ----------------------------------------------------------------------------
// v0.7.32 — PPT master / theme / layouts
// ----------------------------------------------------------------------------

export interface MasterPartInfo {
  partPath: string
  name: string
  kind: 'master' | 'layout'
}

/** List the slide masters + layouts visible in the deck (name + part path). */
export function listMasterPartsOnDeck(deck: PptxShapeDeck): MasterPartInfo[] {
  if (!deck.opened) return []
  const raw = engineListMasterParts(deck.opened.archive) as EngineMasterPartInfo[]
  return raw.map((m) => ({
    partPath: m.partPath,
    name: m.name,
    kind: m.kind,
  }))
}

/** Parse one master part into a Slide model. */
export function parseMasterOnDeck(
  deck: PptxShapeDeck,
  partPath: string,
): { partPath: string; layoutNames: string[] } | null {
  if (!deck.opened) return null
  const slide = engineParseMasterPart(deck.opened.archive, partPath)
  if (!slide) return null
  return { partPath, layoutNames: (slide as any).layoutNames ?? [] }
}

/** Replace theme colors + fonts in every theme part. */
export function applyThemeToDeck(deck: PptxShapeDeck, spec: EngineThemeSpec): number {
  if (!deck.opened) return 0
  try {
    return engineApplyThemeToArchive(deck.opened, spec) as number
  } catch {
    return 0
  }
}

/** Remap explicit srgbClr values across the deck based on a ThemeSpec. */
export function recolorDeck(deck: PptxShapeDeck, spec: EngineThemeSpec): number {
  if (!deck.opened) return 0
  try {
    return engineRemapDeckColors(deck.opened, spec) as number
  } catch {
    return 0
  }
}

/** Show the list of available built-in layouts (filtered by slide size +
 *  existing layout names so the user doesn't see dupes). */
export function listBuiltinLayouts(
  sizeW: number,
  sizeH: number,
  existingNames: string[],
): Array<{ name: string; layoutType: string; key: string }> {
  const set = new Set(existingNames)
  const infos = engineBuiltinLayoutInfos(
    { cx: sizeW, cy: sizeH } as EngineSlideSize,
    set,
  )
  return infos.map((i: EngineSlideLayoutInfo) => ({
    name: i.name,
    layoutType: i.layoutType,
    key: (i.path ?? '').replace(/^builtin:/, ''),
  }))
}

/** Try to materialize one of the built-in layouts onto the deck. Returns
 *  the layout name on success, null on failure. */
export function ensureBuiltinLayout(
  deck: PptxShapeDeck,
  sizeW: number,
  sizeH: number,
  key: string,
): string | null {
  if (!deck.opened) return null
  try {
    return engineEnsureBuiltinLayout(
      deck.opened.archive,
      { cx: sizeW, cy: sizeH } as EngineSlideSize,
      key,
    ) as string | null
  } catch {
    return null
  }
}

/** Enumerate slide layouts available in the deck (path / name / placeholders). */
export function listSlideLayouts(deck: PptxShapeDeck): Array<{ path: string; name: string; placeholders: number }> {
  if (!deck.opened) return []
  try {
    return engineListSlideLayouts(deck.opened.archive).map((l) => ({
      path: l.path,
      name: l.name,
      placeholders: l.placeholders?.length ?? 0,
    }))
  } catch {
    return []
  }
}

/** Switch the active slide to a different layout (path from listSlideLayouts). */
export function setSlideLayout(deck: PptxShapeDeck, slideIndex: number, layoutPath: string): boolean {
  if (!deck.opened) return false
  try {
    const slide = engineSetSlideLayout(deck.opened, slideIndex, layoutPath)
    return slide !== null
  } catch {
    return false
  }
}

/** Reset the active slide's layout (drop explicit placeholder xfrm, fall back to layout/master). */
export function resetSlideLayout(deck: PptxShapeDeck, slideIndex: number): boolean {
  if (!deck.opened) return false
  try {
    const slide = engineResetSlideLayout(deck.opened, slideIndex)
    return slide !== null
  } catch {
    return false
  }
}

/** The static catalog of built-in layouts shipped with the engine. */
export function getBuiltinLayoutCatalog(): BuiltinLayoutDef[] {
  return BUILTIN_LAYOUTS
}

/** Convenience: should the layout gallery show built-in layouts at all?
 *  Caller passes the deck's existing layouts (those without functional
 *  placeholders are candidates for being replaced by a built-in). */
export function shouldOfferBuiltinLayoutsOnDeck(
  existingLayouts: Array<{ name: string; placeholders: unknown[] }>,
): boolean {
  return engineShouldOfferBuiltinLayouts(existingLayouts)
}
