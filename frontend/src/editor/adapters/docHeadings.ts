/**
 * docHeadings — DOC heading outline (v0.7.71).
 *
 * Vendored from genoffice apps/docs/src/renderer/editor/headings.ts (the
 * NavPane and TOC page-refresh both consume this).
 *
 * Adapted: genoffice uses a custom "docHeading" node; WeKnora uses the
 * stock TipTap "heading" node (StarterKit). We detect both so existing
 * WeKnora docs work AND future schema migration can keep the helper
 * single-source.
 */
import type { Node as PmNode } from '@tiptap/pm/model'

export interface HeadingRef {
  text: string
  level: number
  /** top-level position of the heading node */
  pos: number
}

/** Single heading predicate shared by the TOC, the nav pane, and TOC page backfill (document order). */
export function collectHeadings(doc: PmNode): HeadingRef[] {
  const out: HeadingRef[] = []
  doc.forEach((node, offset) => {
    const name = node.type.name
    if (name !== 'heading' && name !== 'docHeading') return
    const text = node.textContent.trim()
    if (!text) return
    out.push({ text, level: Number(node.attrs.level) || 1, pos: offset })
  })
  return out
}

/** Convenience: outline tree (nested) for sidebar rendering. */
export interface HeadingTree extends HeadingRef {
  children: HeadingTree[]
}

export function buildHeadingTree(headings: HeadingRef[]): HeadingTree[] {
  const roots: HeadingTree[] = []
  const stack: HeadingTree[] = []
  for (const h of headings) {
    const node: HeadingTree = { ...h, children: [] }
    while (stack.length > 0 && stack[stack.length - 1]!.level >= h.level) {
      stack.pop()
    }
    if (stack.length === 0) {
      roots.push(node)
    } else {
      stack[stack.length - 1]!.children.push(node)
    }
    stack.push(node)
  }
  return roots
}

/** Flatten a heading tree back to a depth-first list with depths for jump-nav rendering. */
export function flattenHeadingTree(tree: HeadingTree[], depth = 0): Array<HeadingRef & { depth: number }> {
  const out: Array<HeadingRef & { depth: number }> = []
  for (const n of tree) {
    out.push({ ...n, depth })
    if (n.children.length > 0) {
      out.push(...flattenHeadingTree(n.children, depth + 1))
    }
  }
  return out
}
