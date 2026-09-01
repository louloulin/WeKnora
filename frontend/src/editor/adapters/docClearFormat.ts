/**
 * docClearFormat — DOC clear formatting helpers (v0.7.78).
 *
 * Word's "Clear All Formatting" / Ctrl+Space: strip every character-level mark
 * (bold / italic / underline / strike / code / highlight / color / link / …)
 * from the active selection, leaving the underlying text intact. Paragraph-
 * level attributes (heading level, list kind, alignment) are NOT touched —
 * that's a separate Word command ("Reset Paragraph").
 *
 * Local implementation; genoffice doesn't expose this as a separate file.
 */
import type { Editor } from '@tiptap/core'
import type { Mark } from '@tiptap/pm/model'

/** Marks that should be cleared when the user asks to "clear formatting". */
const REMOVABLE_MARKS = [
  'bold',
  'italic',
  'underline',
  'strike',
  'code',
  'link',
  'highlight',
  'subscript',
  'superscript',
  'textStyle',
  'commentMark',
] as const

/** Strip all character-level marks from the current selection. */
export function clearFormatting(editor: Editor): boolean {
  const { state, view } = editor
  const { from, to } = state.selection
  if (from === to) return false
  // Iterate marks present in the selection and unset each via its own type.
  const seen: string[] = []
  state.doc.nodesBetween(from, to, (node) => {
    if (!node.isText) return
    for (const m of node.marks) {
      if (!seen.includes(m.type.name)) seen.push(m.type.name)
    }
  })
  if (seen.length === 0) return false
  const chain = editor.chain().focus()
  for (const name of seen) {
    chain.unsetMark(name)
  }
  return chain.run()
}

/** Whether the selection currently has any removable formatting. */
export function hasFormatting(editor: Editor): boolean {
  const { state } = editor
  const { from, to } = state.selection
  if (from === to) return false
  let found = false
  state.doc.nodesBetween(from, to, (node) => {
    if (found || !node.isText) return
    for (const m of node.marks as Mark[]) {
      if ((REMOVABLE_MARKS as readonly string[]).includes(m.type.name)) {
        found = true
        return
      }
    }
  })
  return found
}

/** Number of removable marks across the selection (for UI count display). */
export function countFormatting(editor: Editor): number {
  const { state } = editor
  const { from, to } = state.selection
  if (from === to) return 0
  const seen = new Set<string>()
  state.doc.nodesBetween(from, to, (node) => {
    if (!node.isText) return
    for (const m of node.marks as Mark[]) {
      if ((REMOVABLE_MARKS as readonly string[]).includes(m.type.name)) {
        seen.add(m.type.name)
      }
    }
  })
  return seen.size
}
