/**
 * docTocRefresh — TOC (Table of Contents) page number backfill (v0.7.71).
 *
 * Vendored from genoffice apps/docs/src/renderer/editor/toc-refresh.ts.
 *
 * Pure helper: caller supplies freshly measured heading pages; we walk the
 * doc for docProtected "tocLine" fields and replace their right-side text
 * with the matching page number. The caller formats via the owning section's
 * pgNumType, so Roman/letter/dashed numbers survive the refresh.
 *
 * Adapted for WeKnora: the field "tocLine" lives on `docProtected` blocks;
 * WeKnora also uses the same schema (see docNodes.ts DocProtected), so the
 * shape lines up. If the protected kind is ever renamed, only this file
 * changes.
 */
import type { Node as PmNode } from '@tiptap/pm/model'
import type { Transaction } from '@tiptap/pm/state'
import type { HeadingRef } from './docHeadings'

interface TocLineFieldDisplay {
  kind?: string
  left?: string
  right?: string
}

/**
 * `displays` is aligned with `headings` (document order): one formatted page
 * string per heading, or undefined when the heading was not measured.
 *
 * Returns true when `tr` gained at least one node update.
 */
export function applyTocPageDisplays(
  doc: PmNode,
  tr: Transaction,
  headings: HeadingRef[],
  displays: Array<string | undefined>,
): boolean {
  const pagesOfTitle = new Map<string, Array<string | undefined>>()
  headings.forEach((h, i) => {
    const key = h.text.trim()
    const list = pagesOfTitle.get(key)
    if (list) list.push(displays[i])
    else pagesOfTitle.set(key, [displays[i]])
  })
  if (pagesOfTitle.size === 0) return false
  let changed = false
  const seen = new Map<string, number>()
  doc.forEach((node, offset) => {
    if (node.type.name !== 'docProtected') return
    const field = node.attrs.fieldDisplay as TocLineFieldDisplay | null
    if (field?.kind !== 'tocLine') return
    const key = (field.left ?? '').trim()
    const list = pagesOfTitle.get(key)
    if (!list) return
    const nth = seen.get(key) ?? 0
    seen.set(key, nth + 1)
    const right = list[Math.min(nth, list.length - 1)]
    if (right === undefined || (field.right ?? '') === right) return
    tr.setNodeMarkup(offset, undefined, {
      ...node.attrs,
      fieldDisplay: { ...field, right },
    })
    changed = true
  })
  return changed
}
