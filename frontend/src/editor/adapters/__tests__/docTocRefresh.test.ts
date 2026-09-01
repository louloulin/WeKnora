// v0.7.71 — DOC TOC page-refresh helper

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EditorState, TextSelection } from '@tiptap/pm/state'
import type { Transaction } from '@tiptap/pm/state'
import { Schema, type NodeSpec, type MarkSpec } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/core'
import { applyTocPageDisplays } from '../docTocRefresh'
import type { HeadingRef } from '../docHeadings'

// docProtected atom carrying a { fieldDisplay } attr (matches docNodes.ts DocProtected).
const protectedNode: NodeSpec = {
  group: 'block',
  atom: true,
  attrs: {
    docxIndex: { default: 0 },
    blockType: { default: 'protected' },
    label: { default: '' },
    previewText: { default: '' },
    genXml: { default: '' },
    formulaDisplay: { default: false },
    fieldDisplay: { default: null },
  },
  toDOM: () => ['div', 0],
  parseDOM: [{ tag: 'div[data-protected]' }],
}

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    heading: {
      group: 'block',
      content: 'inline*',
      attrs: { level: { default: 1 } },
      toDOM: (n) => ['h' + n.attrs.level, 0],
      parseDOM: [
        { tag: 'h1', getAttrs: () => ({ level: 1 }) },
        { tag: 'h2', getAttrs: () => ({ level: 2 }) },
      ],
    },
    docProtected: protectedNode,
    text: { group: 'inline' },
  },
  marks: {} as Record<string, MarkSpec>,
})

function makeEditor(blocks: Array<
  | { type: 'paragraph' | 'heading'; text?: string; level?: number }
  | { type: 'docProtected'; fieldDisplay: { kind: string; left?: string; right?: string } | null }
>) {
  const children = blocks.map((b) => {
    if (b.type === 'docProtected') {
      return schema.nodes.docProtected!.create({ fieldDisplay: b.fieldDisplay })
    }
    if (b.type === 'heading') {
      return schema.nodes.heading!.create({ level: b.level ?? 1 }, b.text ? [schema.text(b.text)] : [])
    }
    return schema.nodes.paragraph!.create(null, b.text ? [schema.text(b.text)] : [])
  })
  let state = EditorState.create({ schema, doc: schema.node('doc', null, children) })
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

test('applyTocPageDisplays: no headings → returns false (no change)', () => {
  const { editor } = makeEditor([
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Intro', right: '' } },
  ])
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, [], [])
  assert.equal(changed, false)
})

test('applyTocPageDisplays: matches tocLine by title text', () => {
  const { editor } = makeEditor([
    { type: 'heading', text: 'Intro', level: 1 },
    { type: 'paragraph', text: 'body' },
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Intro', right: '' } },
  ])
  const headings: HeadingRef[] = [{ text: 'Intro', level: 1, pos: 0 }]
  const displays: Array<string | undefined> = ['3']
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, headings, displays)
  assert.equal(changed, true)
})

test('applyTocPageDisplays: skips non-tocLine fieldDisplay', () => {
  const { editor } = makeEditor([
    { type: 'docProtected', fieldDisplay: { kind: 'pageNumber', left: '', right: '' } },
  ])
  const headings: HeadingRef[] = [{ text: 'Intro', level: 1, pos: 0 }]
  const displays: Array<string | undefined> = ['5']
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, headings, displays)
  assert.equal(changed, false)
})

test('applyTocPageDisplays: no matching title → no change', () => {
  const { editor } = makeEditor([
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Other', right: '' } },
  ])
  const headings: HeadingRef[] = [{ text: 'Intro', level: 1, pos: 0 }]
  const displays: Array<string | undefined> = ['3']
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, headings, displays)
  assert.equal(changed, false)
})

test('applyTocPageDisplays: duplicate titles consume pages in order', () => {
  // First "Intro" heading on page 1, second "Intro" on page 5
  const { editor } = makeEditor([
    { type: 'heading', text: 'Intro', level: 1 },
    { type: 'paragraph', text: 'body 1' },
    { type: 'heading', text: 'Intro', level: 1 },
    { type: 'paragraph', text: 'body 2' },
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Intro', right: '' } },
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Intro', right: '' } },
  ])
  const headings: HeadingRef[] = [
    { text: 'Intro', level: 1, pos: 0 },
    { text: 'Intro', level: 1, pos: 2 },
  ]
  const displays: Array<string | undefined> = ['1', '5']
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, headings, displays)
  assert.equal(changed, true)
  // Commit the transaction so node attrs are visible on the next state read.
  editor.view.dispatch(tr)
  const newState = editor.state
  const tocLines: any[] = []
  newState.doc.forEach((node) => {
    if (node.type.name === 'docProtected' && node.attrs.fieldDisplay?.kind === 'tocLine') {
      tocLines.push(node.attrs.fieldDisplay.right)
    }
  })
  assert.equal(tocLines.length, 2)
  assert.equal(tocLines[0], '1')
  assert.equal(tocLines[1], '5')
})

test('applyTocPageDisplays: same page twice → no change (idempotent)', () => {
  const { editor } = makeEditor([
    { type: 'heading', text: 'Intro', level: 1 },
    { type: 'docProtected', fieldDisplay: { kind: 'tocLine', left: 'Intro', right: '3' } },
  ])
  const headings: HeadingRef[] = [{ text: 'Intro', level: 1, pos: 0 }]
  const displays: Array<string | undefined> = ['3']
  const tr = editor.state.tr.setSelection(
    TextSelection.create(editor.state.doc, 0),
  )
  const changed = applyTocPageDisplays(editor.state.doc, tr, headings, displays)
  assert.equal(changed, false, 'no change when right already matches display')
})
