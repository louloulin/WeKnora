// v0.7.68 — DOC track-changes helpers, runs without a DOM via EditorState + mock view.
// Mirrors the docComments.test.ts pattern.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EditorState, TextSelection } from '@tiptap/pm/state'
import type { Transaction } from '@tiptap/pm/state'
import { Schema, type MarkSpec } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/core'
import {
  collectRevisions,
  acceptAllRevisions,
  rejectAllRevisions,
  applyRevisionsBy,
  acceptCurrentRevision,
  rejectCurrentRevision,
  gotoRevision,
  revisionLabel,
} from '../docRevisions'

const insMark: MarkSpec = {
  inclusive: false,
  attrs: { author: { default: '' }, date: { default: null } },
}
const delMark: MarkSpec = {
  inclusive: false,
  attrs: { author: { default: '' }, date: { default: null } },
}

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    text: { group: 'inline' },
  },
  marks: { ins: insMark, del: delMark },
})

function makeEditor(content: string): { editor: Editor; state: () => EditorState } {
  let state = EditorState.create({
    schema,
    doc: schema.node('doc', null, [
      schema.node('paragraph', null, content ? [schema.text(content)] : []),
    ]),
  })
  const view = {
    dispatch: (tr: Transaction) => {
      state = state.apply(tr)
    },
  }
  const editor = {
    get state() {
      return state
    },
    view,
  } as unknown as Editor
  return { editor, state: () => state }
}

/** insert text at the current selection */
function insertText(editor: Editor, text: string): void {
  const tr = editor.state.tr.insertText(text)
  tr.setMeta('trackIgnore', true)
  editor.view.dispatch(tr)
}

/** set selection range */
function select(editor: Editor, from: number, to: number): void {
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, from, to),
  )
  tr.setMeta('trackIgnore', true)
  editor.view.dispatch(tr)
}

/** add an ins/del mark to the given range */
function addMark(
  editor: Editor,
  markName: 'ins' | 'del',
  from: number,
  to: number,
  author: string,
): void {
  const mark = editor.state.schema.marks[markName]!.create({ author })
  const tr = editor.state.tr.addMark(from, to, mark)
  tr.setMeta('trackIgnore', true)
  editor.view.dispatch(tr)
}

const docText = (ed: Editor): string =>
  ed.state.doc.textBetween(0, ed.state.doc.content.size, '', '')

test('collectRevisions: empty doc returns []', () => {
  const { editor } = makeEditor('')
  assert.deepEqual(collectRevisions(editor.state.doc), [])
})

test('collectRevisions: detects ins range', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 1, 6, 'alice')
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 1)
  assert.equal(revs[0]!.kind, 'ins')
  assert.equal(revs[0]!.author, 'alice')
  assert.equal(revs[0]!.from, 1)
  assert.equal(revs[0]!.to, 6)
})

test('collectRevisions: detects del range', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'del', 7, 12, 'bob')
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 1)
  assert.equal(revs[0]!.kind, 'del')
  assert.equal(revs[0]!.author, 'bob')
})

test('collectRevisions: merges adjacent same-author same-kind ranges', () => {
  const { editor } = makeEditor('abcdef')
  addMark(editor, 'ins', 1, 4, 'alice')
  addMark(editor, 'ins', 4, 7, 'alice')
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 1)
  assert.equal(revs[0]!.from, 1)
  assert.equal(revs[0]!.to, 7)
})

test('acceptAllRevisions: removes ins marks but keeps text', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 5, 'alice')
  acceptAllRevisions(editor)
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 0)
  assert.equal(docText(editor), 'hello world')
})

test('rejectAllRevisions: keeps del-marked text (just removes the del mark)', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'del', 6, 12, 'alice')
  rejectAllRevisions(editor)
  assert.equal(docText(editor), 'hello world')
  assert.equal(collectRevisions(editor.state.doc).length, 0)
})

test('rejectAllRevisions: removes inserted text', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  rejectAllRevisions(editor)
  assert.equal(docText(editor), ' world')
})

test('applyRevisionsBy: only touches one author', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  addMark(editor, 'ins', 7, 12, 'bob')
  applyRevisionsBy(editor, 'alice', 'reject')
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 1)
  assert.equal(revs[0]!.author, 'bob')
  assert.equal(docText(editor), ' world')
})

test('acceptCurrentRevision: accepts the revision at the cursor', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  addMark(editor, 'ins', 7, 12, 'bob')
  select(editor, 0, 0)
  acceptCurrentRevision(editor)
  const revs = collectRevisions(editor.state.doc)
  assert.equal(revs.length, 1)
  assert.equal(revs[0]!.author, 'bob')
})

test('rejectCurrentRevision: rejects the first revision when cursor is past it', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  select(editor, 11, 11)
  rejectCurrentRevision(editor)
  assert.equal(docText(editor), ' world')
})

test('gotoRevision: forward wraps to first range', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  addMark(editor, 'ins', 7, 12, 'bob')
  select(editor, 11, 11)
  assert.equal(gotoRevision(editor, 1), true)
  // wraps to first range (alice 0..5)
  assert.ok(editor.state.selection.from <= 5)
})

test('gotoRevision: backward wraps to last range', () => {
  const { editor } = makeEditor('hello world')
  addMark(editor, 'ins', 0, 6, 'alice')
  addMark(editor, 'ins', 7, 12, 'bob')
  select(editor, 0, 0)
  assert.equal(gotoRevision(editor, -1), true)
})

test('gotoRevision: empty doc returns false', () => {
  const { editor } = makeEditor('')
  assert.equal(gotoRevision(editor, 1), false)
})

test('revisionLabel: returns Chinese labels', () => {
  assert.equal(revisionLabel('ins'), '插入')
  assert.equal(revisionLabel('del'), '删除')
  assert.equal(revisionLabel('blockIns'), '插入')
  assert.equal(revisionLabel('moveTo'), '移动（目标）')
})

test('TRACK_IGNORE constant matches genoffice value', async () => {
  const { TRACK_IGNORE } = await import('../docRevisions')
  assert.equal(TRACK_IGNORE, 'trackIgnore')
})
