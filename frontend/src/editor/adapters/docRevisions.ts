/**
 * docRevisions — DOC track changes support (v0.7.68).
 *
 * Vendored from genoffice apps/docs/src/renderer/editor/revisions.ts.
 * Adapted: drop the table-row/cell track markers (WeKnora's TipTap schema is
 * simpler), keep the pure helper functions that operate on ins/del text marks.
 *
 * The functions here are stateless utilities the editor calls from its own
 * "Accept All" / "Reject All" buttons and from the revision-goto hotkey.
 */
import { type Editor } from '@tiptap/core'
import type { Node as PmNode } from '@tiptap/pm/model'
import { TextSelection } from '@tiptap/pm/state'

/** types of track-changes ranges genoffice collects. WeKnora only emits ins/del on text + block. */
export type RevisionKind =
  | 'ins'
  | 'del'
  | 'both'
  | 'pPrChange'
  | 'moveFrom'
  | 'moveTo'
  | 'rPrChange'
  | 'rowIns'
  | 'rowDel'
  | 'cellIns'
  | 'cellDel'
  | 'blockIns'
  | 'blockDel'

export interface RevisionRange {
  from: number
  to: number
  kind: RevisionKind
  author: string
  date?: string
}

/** transactions carrying this meta are never recorded as revisions */
export const TRACK_IGNORE = 'trackIgnore'

/** contiguous revision ranges in document order (adjacent same-kind ranges merged) */
export function collectRevisions(doc: PmNode): RevisionRange[] {
  const out: RevisionRange[] = []

  // block-level (paragraph) revisions: node attrs.blockRevision
  doc.forEach((node, pos) => {
    const revision = node.attrs?.blockRevision as {
      kind?: 'ins' | 'del'
      author?: string
      date?: string
    } | null
    if (!revision?.kind) return
    out.push({
      from: pos,
      to: pos + node.nodeSize,
      kind: revision.kind === 'ins' ? 'blockIns' : 'blockDel',
      author: revision.author ?? '',
      ...(revision.date ? { date: revision.date } : {}),
    })
  })

  // text-level ins / del marks; merge adjacent ranges of same kind+author
  const textRanges: RevisionRange[] = []
  doc.descendants((node, pos) => {
    if (!node.isText) return
    const ins = node.marks.find((m) => m.type.name === 'ins')?.attrs as
      | { author?: string; date?: string }
      | undefined
    const del = node.marks.find((m) => m.type.name === 'del')?.attrs as
      | { author?: string; date?: string }
      | undefined
    if (!ins && !del) return
    const start = pos
    const end = pos + node.nodeSize
    const last = textRanges[textRanges.length - 1]
    const kind: RevisionKind = ins && del ? 'both' : ins ? 'ins' : 'del'
    const author = ins?.author ?? del?.author ?? ''
    if (
      last &&
      last.to === start &&
      last.kind === kind &&
      last.author === author
    ) {
      last.to = end
    } else {
      textRanges.push({
        from: start,
        to: end,
        kind,
        author,
        ...((ins?.date ?? del?.date) ? { date: ins?.date ?? del?.date } : {}),
      })
    }
  })
  out.push(...textRanges)

  // sort by start position
  out.sort((a, b) => a.from - b.from)
  return out
}

/** apply accept/reject to a list of ranges */
function applyRevisions(
  editor: Editor,
  ranges: RevisionRange[],
  mode: 'accept' | 'reject',
): void {
  if (ranges.length === 0) return
  const state = editor.state
  // process from end → start to keep positions stable
  const sorted = [...ranges].sort((a, b) => b.from - a.from)
  const tr = state.tr
  for (const r of sorted) {
    if (r.kind === 'blockIns' || r.kind === 'blockDel') {
      const node = tr.doc.nodeAt(r.from)
      if (!node) continue
      const newAttrs = {
        ...node.attrs,
        blockRevision: null,
      }
      if (mode === 'accept') {
        tr.setNodeMarkup(r.from, undefined, newAttrs)
      } else {
        const $from = tr.doc.resolve(r.from)
        const $to = tr.doc.resolve(r.to)
        if (
          $from.parent.isTextblock &&
          $from.sameParent($to) &&
          $from.depth === 1 &&
          tr.doc.childCount > 1
        ) {
          tr.delete($from.before(), $to.after())
        } else {
          tr.delete(r.from, r.to)
        }
      }
      continue
    }
    const removeText = mode === 'accept' ? r.kind !== 'ins' : r.kind !== 'del'
    if (removeText) {
      const $from = tr.doc.resolve(r.from)
      const $to = tr.doc.resolve(r.to)
      if (
        $from.parent.isTextblock &&
        $from.sameParent($to) &&
        r.from === $from.start() &&
        r.to === $to.end() &&
        $from.depth === 1 &&
        tr.doc.childCount > 1
      ) {
        tr.delete($from.before(), $to.after())
      } else {
        tr.delete(r.from, r.to)
      }
    } else if (mode === 'accept') {
      tr.removeMark(r.from, r.to, state.schema.marks['ins']!)
    } else {
      tr.removeMark(r.from, r.to, state.schema.marks['del']!)
    }
  }
  tr.setMeta(TRACK_IGNORE, true)
  editor.view.dispatch(tr)
}

export function acceptAllRevisions(editor: Editor): void {
  applyRevisions(editor, collectRevisions(editor.state.doc), 'accept')
}

export function rejectAllRevisions(editor: Editor): void {
  applyRevisions(editor, collectRevisions(editor.state.doc), 'reject')
}

/** accept/reject only the revisions of one author (e.g. the AI assistant) */
export function applyRevisionsBy(
  editor: Editor,
  author: string,
  mode: 'accept' | 'reject',
): void {
  applyRevisions(
    editor,
    collectRevisions(editor.state.doc).filter((r) => r.author === author),
    mode,
  )
}

/** the revision containing the selection head, else the next one, else the first */
function currentRevision(editor: Editor): RevisionRange | null {
  const ranges = collectRevisions(editor.state.doc)
  if (ranges.length === 0) return null
  const head = editor.state.selection.head
  return (
    ranges.find((r) => head >= r.from && head <= r.to) ??
    ranges.find((r) => r.from > head) ??
    ranges[0] ??
    null
  )
}

export function acceptCurrentRevision(editor: Editor): boolean {
  const r = currentRevision(editor)
  if (!r) return false
  applyRevisions(editor, [r], 'accept')
  return true
}

export function rejectCurrentRevision(editor: Editor): boolean {
  const r = currentRevision(editor)
  if (!r) return false
  applyRevisions(editor, [r], 'reject')
  return true
}

/** move the selection to the next / previous revision (wraps around) */
export function gotoRevision(editor: Editor, dir: 1 | -1): boolean {
  const ranges = collectRevisions(editor.state.doc)
  if (ranges.length === 0) return false
  const anchor = editor.state.selection.from
  const target =
    dir === 1
      ? (ranges.find((r) => r.from > anchor) ?? ranges[0]!)
      : ([...ranges].reverse().find((r) => r.from < anchor) ?? ranges[ranges.length - 1]!)
  const tr = editor.state.tr
  tr.setSelection(
    TextSelection.between(
      tr.doc.resolve(target.from),
      tr.doc.resolve(target.to),
    ),
  )
  tr.scrollIntoView()
  tr.setMeta(TRACK_IGNORE, true)
  editor.view.dispatch(tr)
  return true
}

/** human-readable label for a revision kind */
export function revisionLabel(kind: RevisionKind): string {
  switch (kind) {
    case 'ins':
    case 'blockIns':
      return '插入'
    case 'del':
    case 'blockDel':
      return '删除'
    case 'both':
      return '同时存在插入和删除'
    case 'pPrChange':
      return '段落属性变更'
    case 'rPrChange':
      return '文本属性变更'
    case 'moveFrom':
      return '移动（来源）'
    case 'moveTo':
      return '移动（目标）'
    case 'rowIns':
      return '行插入'
    case 'rowDel':
      return '行删除'
    case 'cellIns':
      return '单元格插入'
    case 'cellDel':
      return '单元格删除'
    default:
      return '修订'
  }
}
