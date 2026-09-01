/// v0.7.47 — self-simplified Drawing module (vendored concept from
/// genoffice xlsx-drawing-edit + xlsx-drawing-add). Designed for browser
/// use without zod schema dependencies — pure TS interfaces, no runtime
/// validators. The vendored applyVisualAdditions (xlsxDrawingAdd) is
/// reused as-is for adding new charts/shapes/images.

import JSZip from 'jszip'
import {
  applyVisualAdditions,
  allocatePartPath,
  appendRelationship,
  registerContentTypeOverride,
  relsPathFor,
  relativeTarget,
  resolveRelTarget,
  VisualAddError,
  type DrawingAnchor,
  type VisualAddition,
  type ImageAdd,
  type ChartAdd,
  type ShapeAdd,
  type MutablePackage as DrawMutablePackage,
} from './xlsxDrawingAdd'

export { VisualAddError }
export type { DrawingAnchor, ImageAdd, ChartAdd, ShapeAdd, VisualAddition }

// ============================================================
// Edit operations — surgical remove / move / resize of an anchor
// that already lives in the file. The vendored xlsxDrawingAdd
// handles additions; this file handles edits + deletions so we
// don't pull in the zod schema chain.
// ============================================================

/// Edit a single existing drawing anchor. Identified by (drawingPath,
/// drawingIndex) — the index counts every anchor element in document
/// order (twoCellAnchor / oneCellAnchor / absoluteAnchor).
export interface DrawingEdit {
  readonly drawingPath: string
  readonly drawingIndex: number
  readonly remove?: true | undefined
  readonly anchor?: DrawingAnchor | undefined
  /// Width / height in EMU (1 inch = 914400 EMU). Applied to the anchor's
  /// `<a:ext>` when present (rotated shapes carry size in frame).
  readonly frameSize?: { readonly width: number; readonly height: number } | undefined
}

const ANCHOR_PATTERN =
  /<((?:xdr:)?(?:twoCellAnchor|oneCellAnchor|absoluteAnchor))\b[\s\S]*?<\/\1>/g

/// Apply edits to existing drawings. Returns true when any drawing file
/// was modified; false when no edits touched the package (identity).
export async function applyDrawingEdits(
  pkg: DrawMutablePackage,
  edits: readonly DrawingEdit[],
  touched: Set<string>,
): Promise<boolean> {
  const byPath = new Map<string, DrawingEdit[]>()
  for (const edit of edits) {
    const group = byPath.get(edit.drawingPath) ?? []
    group.push(edit)
    byPath.set(edit.drawingPath, group)
  }
  let dirty = false
  for (const [drawingPath, group] of byPath) {
    if (!(await pkg.has(drawingPath))) {
      throw new VisualAddError(`Workbook is missing ${drawingPath}.`)
    }
    const xml = await pkg.readText(drawingPath)
    // Walk all anchors; iterate edits in reverse index order so removal
    // doesn't shift the remaining indices.
    const anchors = [...xml.matchAll(ANCHOR_PATTERN)]
    const sorted = [...group].sort((a, b) => b.drawingIndex - a.drawingIndex)
    let next = xml
    for (const edit of sorted) {
      const match = anchors[edit.drawingIndex]
      if (!match) continue
      if (edit.remove === true) {
        next = next.replace(match[0], '')
        dirty = true
      } else if (edit.anchor || edit.frameSize) {
        const replaced = patchAnchor(match[0], edit.anchor, edit.frameSize)
        if (replaced !== match[0]) {
          next = next.replace(match[0], replaced)
          dirty = true
        }
      }
    }
    if (dirty) {
      pkg.write(drawingPath, next)
      touched.add(drawingPath)
    }
  }
  return dirty
}

function patchAnchor(
  anchorXml: string,
  anchor: DrawingAnchor | undefined,
  frameSize: { width: number; height: number } | undefined,
): string {
  let out = anchorXml
  if (anchor) {
    // Replace from / to markers using the vendored DrawingAnchor field names
    // (fromRow / fromColumn / toRow / toColumn).
    const fromXml = `<xdr:from><xdr:col>${anchor.fromColumn}</xdr:col><xdr:colOff>${anchor.fromColumnOffset}</xdr:colOff><xdr:row>${anchor.fromRow}</xdr:row><xdr:rowOff>${anchor.fromRowOffset}</xdr:rowOff></xdr:from>`
    const toXml = `<xdr:to><xdr:col>${anchor.toColumn}</xdr:col><xdr:colOff>${anchor.toColumnOffset}</xdr:colOff><xdr:row>${anchor.toRow}</xdr:row><xdr:rowOff>${anchor.toRowOffset}</xdr:rowOff></xdr:to>`
    if (/<xdr:from>[\s\S]*?<\/xdr:from>/.test(out)) {
      out = out.replace(/<xdr:from>[\s\S]*?<\/xdr:from>/, fromXml)
    }
    if (/<xdr:to>[\s\S]*?<\/xdr:to>/.test(out)) {
      out = out.replace(/<xdr:to>[\s\S]*?<\/xdr:to>/, toXml)
    }
  }
  if (frameSize) {
    const ext = `<a:ext cx="${frameSize.width}" cy="${frameSize.height}"/>`
    if (/<a:ext\b[^>]*\/?>/.test(out)) {
      out = out.replace(/<a:ext\b[^>]*\/?>/, ext)
    } else if (/<a:xfrm\b[^>]*>([\s\S]*?)<\/a:xfrm>/.test(out)) {
      out = out.replace(/<a:xfrm\b[^>]*>([\s\S]*?)<\/a:xfrm>/, `<a:xfrm>${ext}</a:xfrm>`)
    } else {
      out = out.replace(/<xdr:(?:twoCellAnchor|oneCellAnchor|absoluteAnchor)\b[^>]*>/,
        `$&<a:xfrm>${ext}</a:xfrm>`)
    }
  }
  return out
}

// ============================================================
// Composite: add + edit in one shot. The CollabSheetEditor saves
/// everything via transformPackage, then walks the new drawings + rels.
/// ============================================================

/// Run add and edit operations in one package transform. Returns the
/// (possibly new) drawings list keyed by (worksheetPath, drawingIndex).
export async function applyDrawingPipeline(
  pkg: DrawMutablePackage,
  additions: readonly VisualAddition[],
  edits: readonly DrawingEdit[],
  touched: Set<string>,
): Promise<void> {
  if (additions.length > 0) {
    await applyVisualAdditions(pkg, additions, touched)
  }
  if (edits.length > 0) {
    await applyDrawingEdits(pkg, edits, touched)
  }
}

/// Read an image file from the browser File input, return base64 + mediaType.
export async function readImageFile(file: File): Promise<ImageAdd> {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  // base64-encode (browser-safe)
  let binary = ''
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]!)
  }
  const base64 = typeof btoa === 'function' ? btoa(binary) : Buffer.from(bytes).toString('base64')
  const mt = file.type as ImageAdd['mediaType']
  if (mt !== 'image/png' && mt !== 'image/jpeg' && mt !== 'image/gif') {
    throw new VisualAddError(`Unsupported image type: ${file.type || 'unknown'}. Use PNG / JPEG / GIF.`)
  }
  return { mediaType: mt, base64 }
}

// Re-export bridge helpers from xlsxDrawingAdd for convenience
export {
  allocatePartPath,
  appendRelationship,
  registerContentTypeOverride,
  relsPathFor,
  relativeTarget,
  resolveRelTarget,
}
