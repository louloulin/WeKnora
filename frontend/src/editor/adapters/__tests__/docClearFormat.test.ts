// v0.7.78 — DOC clear formatting helpers (Ctrl+Space equivalent).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Schema, type MarkSpec } from '@tiptap/pm/model'
import { EditorState, TextSelection, type Transaction } from '@tiptap/pm/state'
import {
  clearFormatting,
  hasFormatting,
  countFormatting,
} from '../docClearFormat'

const boldSpec: MarkSpec = { toDOM: () => ['b', 0], parseDOM: [{ tag: 'b' }] }
const italicSpec: MarkSpec = { toDOM: () => ['i', 0], parseDOM: [{ tag: 'i' }] }
const underlineSpec: MarkSpec = { toDOM: () => ['u', 0], parseDOM: [{ tag: 'u' }] }
const colorSpec: MarkSpec = {
  attrs: { color: { default: null } },
  toDOM: (m) => ['span', { style: `color:${m.attrs.color}` }, 0],
  parseDOM: [{ tag: 'span', getAttrs: (n: any) => ({ color: n.getAttribute?.('style')?.match(/color:(.+)/)?.[1] }) }],
}

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*', marks: '_', toDOM: () => ['p', 0] },
    text: { group: 'inline', toDOM: () => ['span', 0] },
  },
  marks: {
    bold: boldSpec,
    italic: italicSpec,
    underline: underlineSpec,
    color: colorSpec,
  },
})

function makeMockEditor(initialText: string, ranges: Array<[number, number, string]>) {
  // Build doc with given marks
  const inline: any[] = []
  let plain = initialText
  // simple range-based mark application (assumes ranges are sorted, non-overlapping)
  let cursor = 0
  const sortedRanges = [...ranges].sort((a, b) => a[0] - b[0])
  for (const [from, to, markName] of sortedRanges) {
    if (from > cursor) inline.push(schema.text(plain.slice(cursor, from)))
    const text = plain.slice(from, to)
    const mark = schema.marks[markName]
    if (!mark) throw new Error(`unknown mark ${markName}`)
    inline.push(text ? schema.text(text, [mark.create()]) : schema.text(''))
    cursor = to
  }
  if (cursor < plain.length) inline.push(schema.text(plain.slice(cursor)))
  const doc = schema.nodes.doc.create(null, [schema.nodes.paragraph.create(null, inline)])

  let state = EditorState.create({ doc })
  const selFrom = 1
  const selTo = 1 + plain.length
  state = state.apply(state.tr.setSelection(TextSelection.create(state.doc, selFrom, selTo)))

  const fakeEditor: any = {
    get state() { return state },
    set state(v: any) { state = v },
    view: { dispatch(tr: Transaction) { state = state.apply(tr) } },
    chain() {
      let pending: Array<(args: any) => any> = []
      const api: any = {
        focus() { return api },
        unsetMark(name: string) {
          pending.push(({ tr, state, dispatch }: any) => {
            const { from, to } = state.selection
            if (from === to) return false
            const markType = state.schema.marks[name]
            if (!markType) return false
            // Apply to the accumulated tr (not a fresh one) so subsequent
            // unsetMark calls in the same chain see the previous removal.
            tr.removeMark(from, to, markType)
            return true
          })
          return api
        },
        run() {
          let tr = state.tr
          for (const cb of pending) {
            const result = cb({ state, tr, dispatch: (t: any) => (tr = t) })
            if (result === false) break
          }
          if (tr.docChanged || tr.steps.length > 0) state = state.apply(tr)
          return true
        },
      }
      return api
    },
  }
  return fakeEditor
}

function textAndMarks(editor: any): Array<{ text: string; marks: string[] }> {
  const out: Array<{ text: string; marks: string[] }> = []
  editor.state.doc.descendants((node: any) => {
    if (!node.isText) return
    out.push({ text: node.text, marks: node.marks.map((m: any) => m.type.name) })
  })
  return out
}

test('clearFormatting: empty selection → no change', () => {
  const editor = makeMockEditor('hello', [])
  editor.state.selection = TextSelection.create(editor.state.doc, 3, 3) // collapsed
  const ok = clearFormatting(editor)
  assert.equal(ok, false, 'collapsed selection → false')
})

test('clearFormatting: selection with no marks → no change', () => {
  const editor = makeMockEditor('hello world', [])
  const ok = clearFormatting(editor)
  assert.equal(ok, false)
})

test('clearFormatting: selection with bold → strips bold', () => {
  const editor = makeMockEditor('hello world', [[0, 5, 'bold']])
  // Before: at least one text node has bold
  const before = textAndMarks(editor)
  assert.ok(before.some((n) => n.marks.includes('bold')))
  // textContent includes 'hello world'
  assert.equal(editor.state.doc.textContent, 'hello world')
  clearFormatting(editor)
  // After: no text node has bold (PM auto-merges adjacent same-mark runs into 'hello world' once bold is gone)
  const after = textAndMarks(editor)
  for (const n of after) {
    assert.equal(n.marks.includes('bold'), false, `text "${n.text}" should not have bold`)
  }
  // textContent unchanged
  assert.equal(editor.state.doc.textContent, 'hello world')
})

test('clearFormatting: multiple marks (bold + italic + underline) → strips all', () => {
  // Use non-overlapping ranges so each mark is on its own text node
  const editor = makeMockEditor('hello world', [[0, 5, 'bold'], [6, 11, 'italic']])
  const before = textAndMarks(editor)
  assert.ok(before.some((n) => n.marks.includes('bold')))
  assert.ok(before.some((n) => n.marks.includes('italic')))
  assert.equal(editor.state.doc.textContent, 'hello world')
  clearFormatting(editor)
  // After: no marks anywhere; textContent unchanged
  const after = textAndMarks(editor)
  for (const n of after) {
    assert.equal(n.marks.length, 0, `text "${n.text}" should have no marks`)
  }
  assert.equal(editor.state.doc.textContent, 'hello world')
})

test('clearFormatting: only clears marks in selection range', () => {
  const editor = makeMockEditor('aaa bbb ccc', [[0, 3, 'bold'], [8, 11, 'bold']])
  // Select only middle "bbb"
  editor.state.selection = TextSelection.create(editor.state.doc, 4, 7)
  clearFormatting(editor)
  // aaa and ccc should still be bold; bbb should not
  const out = textAndMarks(editor)
  assert.ok(out[0]?.marks.includes('bold'), 'aaa still bold')
  assert.equal(out[1]?.marks.length ?? -1, 0, 'bbb no marks')
  assert.ok(out[2]?.marks.includes('bold'), 'ccc still bold')
})

test('hasFormatting: returns true when selection has marks', () => {
  const editor = makeMockEditor('hello', [[0, 5, 'bold']])
  assert.equal(hasFormatting(editor), true)
})

test('hasFormatting: returns false when selection has no marks', () => {
  const editor = makeMockEditor('hello', [])
  assert.equal(hasFormatting(editor), false)
})

test('hasFormatting: collapsed selection → false', () => {
  const editor = makeMockEditor('hello', [[0, 5, 'bold']])
  editor.state.selection = TextSelection.create(editor.state.doc, 3, 3)
  assert.equal(hasFormatting(editor), false)
})

test('countFormatting: returns unique mark count', () => {
  const editor = makeMockEditor('hello world', [[0, 5, 'bold'], [0, 5, 'italic']])
  assert.equal(countFormatting(editor), 2)
})

test('countFormatting: same mark twice on overlapping runs → still 1', () => {
  const editor = makeMockEditor('hello world', [[0, 3, 'bold'], [2, 5, 'bold']])
  assert.equal(countFormatting(editor), 1, 'deduped')
})
