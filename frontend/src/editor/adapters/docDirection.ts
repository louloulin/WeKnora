/**
 * docDirection — DOC writing-direction helpers (v0.7.80).
 *
 * Vendored from genoffice apps/docs/src/renderer/editor/direction.ts, but
 * adapted to expose only the genoffice-schema-agnostic helpers:
 *
 *  - firstStrongDir: UAX#9 bidi (Hebrew / Arabic / Syriac / Thaana / NKo /
 *    Samaritan / Mandaic → 'rtl'; other letters → 'ltr'; no letters → null)
 *  - effectiveBidi: reads both explicit `bidi` and the render-only
 *    `bidiInferred` flag set by AutoDirection
 *  - alignAttrFor: resolves a logical align ('left' / 'right' / 'center' /
 *    'justify') into the stored visual attr, swapping start/end in RTL
 *    paragraphs (Word semantics)
 *
 * The TipTap chain commands (setSelectionAlign / setParagraphDirection /
 * AutoDirectionExtension) are intentionally NOT vendored — they depend on
 * genoffice's `docParagraph / docHeading / docListItem / docProtected` schema
 * which WeKnora names differently (paragraph / heading / taskList / docProtected).
 * Adapting those needs a small node-name map and is tracked in v0.7.80.b.
 */

const LETTER = /\p{L}/u

/** strong RTL scripts per UAX#9 (R/AL classes) */
export const RTL_CHAR =
  /[\p{Script=Hebrew}\p{Script=Arabic}\p{Script=Syriac}\p{Script=Thaana}\p{Script=Nko}\p{Script=Samaritan}\p{Script=Mandaic}]/u

/**
 * Direction of the first strong directional character (dir="auto" semantics),
 * or null if none. Only letters decide: digits, combining marks, format
 * controls and punctuation (including script-Arabic marks like the Arabic
 * comma, which are bidi-neutral despite their script) are all weak.
 */
export function firstStrongDir(text: string): 'ltr' | 'rtl' | null {
  for (const ch of text) {
    if (!LETTER.test(ch)) continue
    return RTL_CHAR.test(ch) ? 'rtl' : 'ltr'
  }
  return null
}

/** effective direction: explicit w:bidi or the render-only inferred flag */
export function effectiveBidi(attrs: Record<string, unknown>): boolean {
  return attrs.bidi === true || attrs.bidiInferred === true
}

/**
 * Resolve a logical align into the stored visual attr. In an LTR paragraph,
 * null means "start" (left); in an RTL paragraph, null means "start" (right).
 * Word semantics: aligning to start clears the attr, aligning to end sets it
 * explicitly (start = visual begin of the writing direction).
 */
export function alignAttrFor(
  align: 'left' | 'center' | 'right' | 'justify',
  bidi: boolean,
): string | null {
  if (align === 'left') return bidi ? 'left' : null
  if (align === 'right') return bidi ? null : 'right'
  return align
}

/** Flip a paragraph's align + bidi to swap writing direction (start-aligned keeps visual value). */
export function dirFlipAttrs(
  attrs: Record<string, unknown>,
  bidi: boolean,
): Record<string, unknown> {
  let align = attrs.align as string | null
  if (align === 'left') align = 'right'
  else if (align === 'right') align = 'left'
  // an explicit direction choice supersedes the render-only inference
  return { ...attrs, bidi, align, bidiInferred: false }
}

/** Returns the dir ('ltr' / 'rtl') implied by a paragraph's bidi attrs. */
export function paragraphDir(attrs: Record<string, unknown>): 'ltr' | 'rtl' {
  return effectiveBidi(attrs) ? 'rtl' : 'ltr'
}

// ---------------------------------------------------------------------------
// v0.7.81 — WeKnora-adapted layer (schema-aware). These mirror genoffice's
// `setSelectionAlign / setParagraphDirection / AutoDirectionExtension` but
// use WeKnora's node names: paragraph / heading / taskItem / docProtected.
// ---------------------------------------------------------------------------

import type { Editor } from '@tiptap/core'

/** WeKnora-equivalent of genoffice's DIR_BLOCKS. */
export const WK_DIR_BLOCKS = new Set(['paragraph', 'heading', 'taskItem'])

/**
 * bidi attr of the paragraph-like node at the cursor (false in textbox
 * sub-editors, which have no bidi). Returns false if no paragraph-like
 * ancestor is active.
 */
export function activeBidiWK(editor: Editor): boolean {
  for (const name of ['heading', 'taskItem', 'paragraph']) {
    if (editor.isActive(name)) return effectiveBidi(editor.getAttributes(name) as Record<string, unknown>)
  }
  return false
}

/** Apply alignment to every paragraph-like block in the selection. */
export function setSelectionAlignWK(
  editor: Editor,
  align: 'left' | 'center' | 'right' | 'justify',
): boolean {
  return editor
    .chain()
    .focus()
    .command(({ state, tr, dispatch }) => {
      const { from, to } = state.selection
      let changed = false
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (WK_DIR_BLOCKS.has(node.type.name)) {
          const value = alignAttrFor(align, effectiveBidi(node.attrs as Record<string, unknown>))
          tr.setNodeMarkup(pos, undefined, { ...node.attrs, align: value })
          changed = true
        } else if (node.type.name === 'docProtected') {
          tr.setNodeMarkup(pos, undefined, {
            ...node.attrs,
            imageAlign: alignAttrFor(align, false),
          })
          changed = true
        }
      })
      if (changed && dispatch) dispatch(tr)
      return changed
    })
    .run()
}

/** Set the writing direction of every paragraph-like block in the selection. */
export function setParagraphDirectionWK(editor: Editor, dir: 'ltr' | 'rtl'): boolean {
  const bidi = dir === 'rtl'
  return editor
    .chain()
    .focus()
    .command(({ state, tr, dispatch }) => {
      const { from, to } = state.selection
      let changed = false
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!WK_DIR_BLOCKS.has(node.type.name)) return
        if (effectiveBidi(node.attrs as Record<string, unknown>) === bidi) return
        tr.setNodeMarkup(pos, undefined, dirFlipAttrs(node.attrs as Record<string, unknown>, bidi))
        changed = true
      })
      if (changed && dispatch) dispatch(tr)
      return changed
    })
    .run()
}
