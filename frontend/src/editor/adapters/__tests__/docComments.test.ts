// v0.7.49 — DOC comment mark ops (copy from genoffice apps/docs/tests/comment-ops.test.ts,
// adapted to run without a DOM: EditorState + mock view instead of TipTap Editor).
//
// Note: TipTap 2.x only folds addAttributes() into the ProseMirror schema when
// the extension is resolved through an Editor instance, so the test schema uses
// the equivalent raw MarkSpec (same attrs/inclusive shape as CommentMark).
// ProseMirror merges adjacent text nodes that share identical marks, so the
// per-node id lists below reflect merged runs, not per-character values.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EditorState, TextSelection } from '@tiptap/pm/state'
import type { Transaction } from '@tiptap/pm/state'
import { Schema } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/core'
import {
  CommentMark,
  addCommentToSelection,
  addReplyToCommentRange,
  nextCommentId,
  removeCommentFromDoc,
} from '../docComments'

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    text: { group: 'inline' },
  },
  marks: {
    comment: {
      inclusive: false,
      attrs: { ids: { default: '' } },
    },
  },
})

function makeEditor(content: string): { editor: Editor; state: () => EditorState } {
  let state = EditorState.create({
    schema,
    doc: schema.node('doc', null, [schema.node('paragraph', null, [schema.text(content)])]),
  })
  const view = {
    dispatch: (tr: Transaction) => {
      state = state.apply(tr)
    },
  }
  // editor.state must be a live getter so selection transactions dispatched
  // through the mock view are visible to the comment ops.
  const editor = {
    get state() {
      return state
    },
    view,
  } as unknown as Editor
  return { editor, state: () => state }
}

function select(editor: Editor, from: number, to: number): void {
  const tr = editor.state.tr.setSelection(TextSelection.create(editor.state.doc, from, to))
  ;(editor.view as { dispatch: (t: Transaction) => void }).dispatch(tr)
}

/** ids attr of the comment mark covering each text node, in doc order */
function commentIdsPerNode(state: EditorState): Array<string | null> {
  const out: Array<string | null> = []
  state.doc.descendants((node) => {
    if (!node.isText) return
    const mark = node.marks.find((m) => m.type.name === 'comment')
    out.push(mark ? String(mark.attrs.ids) : null)
  })
  return out
}

test('CommentMark config matches the Word comment range shape', () => {
  assert.equal(CommentMark.name, 'comment')
  assert.equal(CommentMark.config.inclusive, false)
  const attrs = CommentMark.config.addAttributes?.() ?? {}
  assert.deepEqual(Object.keys(attrs), ['ids'])
  assert.equal((attrs.ids as { default: string }).default, '')
})

test('nextCommentId allocates one past the max numeric id', () => {
  assert.equal(nextCommentId([]), '1')
  assert.equal(
    nextCommentId([
      { id: '3', author: 'a', text: '' },
      { id: '7', author: 'b', text: '' },
    ]),
    '8',
  )
})

test('addCommentToSelection marks the selected text with the comment id', () => {
  const { editor, state } = makeEditor('abcd')
  select(editor, 2, 4) // 'bc'
  assert.equal(addCommentToSelection(editor, '1'), true)
  assert.deepEqual(commentIdsPerNode(state()), [null, '1', null])
})

test('addCommentToSelection returns false on empty selection', () => {
  const { editor } = makeEditor('abcd')
  select(editor, 2, 2)
  assert.equal(addCommentToSelection(editor, '1'), false)
})

test('addCommentToSelection merges overlapping comment ids on one mark', () => {
  const { editor, state } = makeEditor('abcdef')
  select(editor, 1, 5) // 'abcd'
  addCommentToSelection(editor, '1')
  select(editor, 3, 6) // 'cdef'
  addCommentToSelection(editor, '2')
  const ids = commentIdsPerNode(state())
  // merged runs: ab(1) | cd(1 2) | e(2) | f(-)
  assert.deepEqual(ids, ['1', '1 2', '2', null])
})

test('removeCommentFromDoc strips the id and drops the mark when empty', () => {
  const { editor, state } = makeEditor('abcd')
  select(editor, 1, 4) // 'abc'
  addCommentToSelection(editor, '1')
  removeCommentFromDoc(editor, '1')
  assert.deepEqual(commentIdsPerNode(state()), [null])
})

test('removeCommentFromDoc keeps other ids on the same mark', () => {
  const { editor, state } = makeEditor('abcd')
  select(editor, 1, 4) // 'abc'
  addCommentToSelection(editor, '1')
  addCommentToSelection(editor, '2')
  removeCommentFromDoc(editor, '1')
  assert.deepEqual(commentIdsPerNode(state()), ['2', null])
})

test('addReplyToCommentRange appends the reply id to parent-anchored ranges', () => {
  const { editor, state } = makeEditor('abcd')
  select(editor, 1, 3) // 'ab'
  addCommentToSelection(editor, '1')
  assert.equal(addReplyToCommentRange(editor, '1', '2'), true)
  assert.deepEqual(commentIdsPerNode(state()), ['1 2', null])
})

test('addReplyToCommentRange returns false when parent id is absent', () => {
  const { editor } = makeEditor('abcd')
  select(editor, 1, 3)
  addCommentToSelection(editor, '1')
  assert.equal(addReplyToCommentRange(editor, '99', '2'), false)
})
