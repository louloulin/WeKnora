// v0.7.74 — DOC move blocks up/down (Word Alt+Shift+Up / Alt+Shift+Down).
//
// genoffice's moveBlocks uses editor.chain().focus().command(...).run();
// the ProseMirror state underneath is plain, so we mock the chain to drive
// the command callback against an in-memory state and assert the result.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Schema, type NodeSpec } from '@tiptap/pm/model'
import { EditorState, TextSelection, type Transaction } from '@tiptap/pm/state'
import { moveBlocks } from '../docMoveBlock'

const paraSpec: NodeSpec = {
  group: 'block',
  content: 'inline*',
  toDOM: () => ['p', 0],
  parseDOM: [{ tag: 'p' }],
}
const textSpec: NodeSpec = {
  group: 'inline',
  toDOM: () => ['span', 0],
}
const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: paraSpec,
    text: textSpec,
  },
})

function makeMockEditor(texts: string[]) {
  const doc = schema.nodes.doc.create(
    null,
    texts.map((t) => schema.nodes.paragraph.create(null, schema.text(t))),
  )
  let state = EditorState.create({ doc })
  const fakeEditor: any = {
    get state() {
      return state
    },
    set state(v: any) {
      state = v
    },
    view: {
      dispatch(tr: Transaction) {
        state = state.apply(tr)
      },
    },
    chain() {
      let pending: ((args: any) => boolean)[] = []
      const api: any = {
        focus() {
          return api
        },
        command(cb: any) {
          pending.push(cb)
          return api
        },
        run() {
          let tr = state.tr
          for (const cb of pending) {
            const result = cb({ state, tr, dispatch: (t: any) => (tr = t) })
            if (result === false) {
              // command returned false → no-op
              return false
            }
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

function readTexts(editor: any): string[] {
  const out: string[] = []
  editor.state.doc.forEach((n: any) => out.push(n.textContent))
  return out
}

function selectBlock(editor: any, childIndex: number) {
  const block = editor.state.doc.child(childIndex)
  let from = 0
  for (let i = 0; i < childIndex; i++) from += editor.state.doc.child(i).nodeSize
  editor.state.selection = TextSelection.create(editor.state.doc, from)
  return block
}

function selectRange(editor: any, fromChild: number, toChild: number) {
  // Position selection so it lands inside the toChild paragraph (not at its end),
  // which matches how a user would drag-select across blocks: the cursor sits
  // somewhere inside the last block rather than at its trailing boundary.
  let from = 0
  for (let i = 0; i < fromChild; i++) from += editor.state.doc.child(i).nodeSize
  let to = 0
  for (let i = 0; i <= toChild; i++) to += editor.state.doc.child(i).nodeSize
  // back off by 1 so to lands inside the toChild block (parentOffset > 0)
  to -= 1
  editor.state.selection = TextSelection.create(editor.state.doc, from, to)
}

test('moveBlocks: down — neighbor goes below selection', () => {
  const editor = makeMockEditor(['A', 'B', 'C', 'D'])
  selectBlock(editor, 1)
  const ok = moveBlocks(editor, 1)
  assert.equal(ok, true)
  // B and C swap
  assert.deepEqual(readTexts(editor), ['A', 'C', 'B', 'D'])
})

test('moveBlocks: up — neighbor goes above selection', () => {
  const editor = makeMockEditor(['A', 'B', 'C', 'D'])
  selectBlock(editor, 1)
  moveBlocks(editor, -1)
  // A and B swap
  assert.deepEqual(readTexts(editor), ['B', 'A', 'C', 'D'])
})

test('moveBlocks: no neighbor above (first block) — returns false', () => {
  const editor = makeMockEditor(['A', 'B'])
  selectBlock(editor, 0)
  const ok = moveBlocks(editor, -1)
  assert.equal(ok, false)
  assert.deepEqual(readTexts(editor), ['A', 'B'], 'no change')
})

test('moveBlocks: no neighbor below (last block) — returns false', () => {
  const editor = makeMockEditor(['A', 'B'])
  selectBlock(editor, 1)
  const ok = moveBlocks(editor, 1)
  assert.equal(ok, false)
  assert.deepEqual(readTexts(editor), ['A', 'B'], 'no change')
})

test('moveBlocks: range selection down — moves neighbor to top', () => {
  const editor = makeMockEditor(['A', 'B', 'C', 'D'])
  selectRange(editor, 1, 2) // B..C with cursor inside C
  moveBlocks(editor, 1)
  // D should now be between A and B
  assert.deepEqual(readTexts(editor), ['A', 'D', 'B', 'C'])
})

test('moveBlocks: range selection up — moves neighbor below range', () => {
  const editor = makeMockEditor(['A', 'B', 'C', 'D'])
  selectRange(editor, 1, 2) // B..C with cursor inside C
  moveBlocks(editor, -1)
  // A should now be between C and D
  assert.deepEqual(readTexts(editor), ['B', 'C', 'A', 'D'])
})

test('moveBlocks: two consecutive downs move B past C then past D', () => {
  const editor = makeMockEditor(['A', 'B', 'C', 'D'])
  selectBlock(editor, 1)
  moveBlocks(editor, 1)
  // Reselect B (now at index 2)
  selectBlock(editor, 2)
  moveBlocks(editor, 1)
  assert.deepEqual(readTexts(editor), ['A', 'C', 'D', 'B'])
})
