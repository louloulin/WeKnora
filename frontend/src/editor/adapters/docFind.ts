/**
 * docFind — DOC in-document find / replace utilities (v0.7.70).
 *
 * Vendored from genoffice apps/docs/src/renderer/components/FindPanel.tsx
 * (Word "Home > Find" / "Ctrl+F"). Pure helpers only:
 *
 *  - foldCase: length-preserving lowercase (so match offsets stay stable
 *    for the Turkish dotted I and friends).
 *  - findMatches: collect every match range inside editable textblocks
 *    (protected blocks excluded — DocProtected nodes are skipped here so
 *    formulas/images don't get searched).
 *  - replaceMatch: apply a single replacement inside the editor.
 *  - replaceAllMatches: replace every match in one transaction.
 *
 * The Vue 3 UI uses the same plain arrays of { from, to } so the panel renders
 * match counts without needing the live Editor instance.
 */
import { type Editor } from '@tiptap/core'
import type { Node as PmNode } from '@tiptap/pm/model'

export interface FindRange {
  from: number
  to: number
}

export interface FindOptions {
  matchCase: boolean
  wholeWord: boolean
}

/** length-preserving lowercase: chars whose lowercase grows ('İ' → 'i̇') stay as-is so match offsets never shift */
export function foldCase(s: string): string {
  let out = ''
  for (const ch of s) {
    const lower = ch.toLowerCase()
    out += lower.length === ch.length ? lower : ch
  }
  return out
}

const isWordChar = (ch: string | undefined): boolean => !!ch && /[\p{L}\p{N}_]/u.test(ch)

/** node names whose descendants we don't search (formulas, images, protected blocks) */
const PROTECTED_NODE_NAMES = new Set([
  'docInlineMath',
  'docProtected',
  'image',
  'tableCellContent', // genoffice-specific; harmless if absent
  'protected_thing', // test-only schema
])

/** check whether a node is editable text (text or textblock), and not inside a protected block. */
function shouldDescend(node: PmNode): boolean {
  if (node.type.spec.group?.includes('block') && node.type.name !== 'paragraph') {
    // skip nested block-level non-textblock nodes (e.g. table cells)
    if (!node.type.spec.content?.includes('inline')) return false
  }
  if (PROTECTED_NODE_NAMES.has(node.type.name)) return false
  return true
}

/** collect matches inside editable textblocks (protected blocks excluded) */
export function findMatches(editor: Editor, query: string, opts: FindOptions): FindRange[] {
  const found: FindRange[] = []
  if (!query) return found
  const needle = opts.matchCase ? query : foldCase(query)
  editor.state.doc.descendants((node, pos) => {
    // skip protected blocks (formulas, images, etc.) before deciding whether to descend
    if (PROTECTED_NODE_NAMES.has(node.type.name)) return false
    if (!node.isTextblock) return shouldDescend(node)
    // flatten the block's inline content so matches spanning marks are found
    let text = ''
    const posAt: number[] = []
    node.forEach((child, offset) => {
      if (child.isText && child.text) {
        for (let k = 0; k < child.text.length; k++) posAt.push(pos + 1 + offset + k)
        text += child.text
      } else {
        posAt.push(pos + 1 + offset)
        text += '\u0000' // leaf placeholder (hard break) never matches
      }
    })
    const haystack = opts.matchCase ? text : foldCase(text)
    let i = 0
    while ((i = haystack.indexOf(needle, i)) !== -1) {
      const isWhole =
        !opts.wholeWord ||
        (!isWordChar(text[i - 1]) && !isWordChar(text[i + query.length]))
      if (isWhole) {
        found.push({ from: posAt[i], to: posAt[i + query.length - 1] + 1 })
        i += query.length
      } else {
        i += 1
      }
    }
    return false
  })
  return found
}

/** apply a single replacement inside the editor */
export function replaceMatch(editor: Editor, range: FindRange, replacement: string): void {
  const tr = editor.state.tr
    .insertText(replacement, range.from, range.to)
  tr.setMeta('findReplace', true)
  editor.view.dispatch(tr)
}

/** replace every match in one transaction (from end → start to keep positions stable) */
export function replaceAllMatches(
  editor: Editor,
  matches: FindRange[],
  replacement: string,
): number {
  if (matches.length === 0) return 0
  const tr = editor.state.tr
  const sorted = [...matches].sort((a, b) => b.from - a.from)
  for (const r of sorted) {
    tr.insertText(replacement, r.from, r.to)
  }
  tr.setMeta('findReplace', true)
  editor.view.dispatch(tr)
  return matches.length
}

/**
 * DocStats — fields of Word's Word Count dialog. WeKnora computes these from
 * the editor document so the numbers reflect what the user sees.
 */
export interface DocStats {
  pages: number
  words: number
  asianChars: number
  nonAsianWords: number
  charsNoSpace: number
  charsWithSpace: number
  paragraphs: number
  lines: number
}

const CJK_RE = /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\u3040-\u309f\u30a0-\u30ff\uff00-\uffef]/

/** Compute Word-count-style statistics from an editor's document. */
export function computeDocStats(editor: Editor): DocStats {
  const doc = editor.state.doc
  let charsWithSpace = 0
  let charsNoSpace = 0
  let asianChars = 0
  let nonAsianWords = 0
  let paragraphs = 0
  let lines = 0
  doc.descendants((node) => {
    if (node.type.name === 'paragraph' || node.type.name === 'heading') {
      paragraphs++
      lines++ // baseline
      node.descendants((child) => {
        if (!child.isText) return true
        for (const ch of (child.text ?? "")) {
          charsWithSpace++
          if (!/\s/.test(ch)) charsNoSpace++
          if (CJK_RE.test(ch)) {
            asianChars++
          }
        }
        // count non-Asian word breaks: split on whitespace + word boundaries
        const tokens = (child.text ?? "").split(/[\s\u3000]+/).filter(Boolean)
        for (const tok of tokens) {
          if (!CJK_RE.test(tok)) nonAsianWords++
        }
        return true
      })
      // count newlines (line breaks) inside the block
      node.descendants((child) => {
        if (!child.isText) return true
        const breaks = ((child.text ?? "").match(/\n/g) || []).length
        lines += breaks
        return true
      })
    }
    return true
  })
  const words = nonAsianWords + asianChars
  return {
    pages: 0, // unknown without pagination
    words,
    asianChars,
    nonAsianWords,
    charsNoSpace,
    charsWithSpace,
    paragraphs,
    lines,
  }
}
