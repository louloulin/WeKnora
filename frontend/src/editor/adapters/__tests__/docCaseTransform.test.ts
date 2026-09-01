// v0.7.75 — DOC case transform helpers (Word Shift+F3).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Schema } from '@tiptap/pm/model'
import { EditorState, TextSelection } from '@tiptap/pm/state'
import {
  transformCase,
  nextCaseMode,
  applyCase,
  selectionText,
  type CaseMode,
} from '../docCaseTransform'

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*', marks: '_', toDOM: () => ['p', 0] },
    text: { group: 'inline', toDOM: () => ['span', 0] },
  },
  marks: {
    bold: { toDOM: () => ['b', 0], parseDOM: [{ tag: 'b' }] },
  },
})

function makeMockEditor(text: string, selectionFrom?: number, selectionTo?: number) {
  const doc = schema.nodes.doc.create(null, [schema.nodes.paragraph.create(null, schema.text(text))])
  let state = EditorState.create({ doc })
  const selFrom = selectionFrom ?? 1
  const selTo = selectionTo ?? 1 + text.length
  state = state.apply(state.tr.setSelection(TextSelection.create(state.doc, selFrom, selTo)))
  const fakeEditor: any = {
    get state() { return state },
    set state(v: any) { state = v },
    view: { dispatch(tr: any) { state = state.apply(tr) } },
    chain() {
      let pending: ((args: any) => boolean | void)[] = []
      const api: any = {
        focus() { return api },
        command(cb: any) { pending.push(cb); return api },
        run() {
          let tr = state.tr
          for (const cb of pending) {
            cb({ state, tr, dispatch: (t: any) => (tr = t) })
          }
          if (tr.docChanged) state = state.apply(tr)
          return true
        },
      }
      return api
    },
  }
  return fakeEditor
}

test('transformCase: upper', () => {
  assert.equal(transformCase('Hello', 'upper'), 'HELLO')
  assert.equal(transformCase('hello world', 'upper'), 'HELLO WORLD')
})

test('transformCase: lower', () => {
  assert.equal(transformCase('HELLO', 'lower'), 'hello')
  assert.equal(transformCase('Mixed Case', 'lower'), 'mixed case')
})

test('transformCase: title (capitalize each word)', () => {
  assert.equal(transformCase('hello world', 'title'), 'Hello World')
  assert.equal(transformCase('HELLO WORLD', 'title'), 'Hello World')
  assert.equal(transformCase('hello-world', 'title'), 'Hello-world')
})

test('transformCase: sentence (capitalize first letter of each sentence)', () => {
  assert.equal(transformCase('hello world', 'sentence'), 'Hello world')
  assert.equal(transformCase('hello. goodbye.', 'sentence'), 'Hello. Goodbye.')
})

test('nextCaseMode: empty → lower', () => {
  assert.equal(nextCaseMode(''), 'lower')
  assert.equal(nextCaseMode('123 !@#'), 'lower', 'no letters → lower')
})

test('nextCaseMode: lowercase → upper', () => {
  assert.equal(nextCaseMode('hello'), 'upper')
})

test('nextCaseMode: UPPERCASE → title', () => {
  assert.equal(nextCaseMode('HELLO'), 'title')
})

test('nextCaseMode: mixed case → lower (Word behavior)', () => {
  assert.equal(nextCaseMode('Hello World'), 'lower')
})

test('applyCase: upper rewrites selection in place', () => {
  const editor = makeMockEditor('Hello world', 1, 12)
  const ok = applyCase(editor, 'upper')
  assert.equal(ok, true)
  const text = editor.state.doc.textContent
  assert.equal(text, 'HELLO WORLD')
})

test('applyCase: lower rewrites selection in place', () => {
  const editor = makeMockEditor('HELLO WORLD', 1, 12)
  applyCase(editor, 'lower')
  assert.equal(editor.state.doc.textContent, 'hello world')
})

test('applyCase: title capitalizes each word', () => {
  const editor = makeMockEditor('hello world', 1, 12)
  applyCase(editor, 'title')
  assert.equal(editor.state.doc.textContent, 'Hello World')
})

test('applyCase: empty selection → false', () => {
  const editor = makeMockEditor('Hello', 6, 6) // collapsed
  const ok = applyCase(editor, 'upper')
  assert.equal(ok, false, 'no selection → no-op')
})

test('applyCase: preserves selection range after rewrite (Word behavior)', () => {
  const editor = makeMockEditor('hello world', 1, 12)
  applyCase(editor, 'upper')
  // After rewriting, the selection should still cover the original range (length preserved here)
  const sel = editor.state.selection
  assert.equal(editor.state.doc.textContent.substring(sel.from - 1, sel.to - 1), 'HELLO WORLD')
})

test('selectionText: returns text between selection', () => {
  const editor = makeMockEditor('hello world', 1, 6)
  assert.equal(selectionText(editor), 'hello')
})
