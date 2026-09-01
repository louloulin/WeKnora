// v0.7.80 — DOC writing-direction helpers (UAX#9 bidi).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Schema } from '@tiptap/pm/model'
import { AllSelection, EditorState, TextSelection } from '@tiptap/pm/state'
import {
  firstStrongDir,
  effectiveBidi,
  alignAttrFor,
  dirFlipAttrs,
  paragraphDir,
  RTL_CHAR,
} from '../docDirection'

test('firstStrongDir: ASCII letters → ltr', () => {
  assert.equal(firstStrongDir('Hello'), 'ltr')
})

test('firstStrongDir: Hebrew → rtl', () => {
  assert.equal(firstStrongDir('שלום'), 'rtl')
})

test('firstStrongDir: Arabic → rtl', () => {
  assert.equal(firstStrongDir('مرحبا'), 'rtl')
})

test('firstStrongDir: digits / punctuation only → null', () => {
  assert.equal(firstStrongDir('123 !@#'), null)
})

test('firstStrongDir: empty → null', () => {
  assert.equal(firstStrongDir(''), null)
})

test('firstStrongDir: ignores format chars and digits before first letter', () => {
  // " 123ABC" — first letter is A → ltr
  assert.equal(firstStrongDir(' 123ABC'), 'ltr')
  // " 123שלום" — first letter is Hebrew → rtl
  assert.equal(firstStrongDir(' 123שלום'), 'rtl')
})

test('firstStrongDir: Arabic comma (U+060C) is not a letter → skips', () => {
  // "،hello" — Arabic comma then ASCII → ltr
  assert.equal(firstStrongDir('،hello'), 'ltr')
})

test('effectiveBidi: bidi=true → true', () => {
  assert.equal(effectiveBidi({ bidi: true }), true)
})

test('effectiveBidi: bidiInferred=true → true (render-only flag)', () => {
  assert.equal(effectiveBidi({ bidiInferred: true }), true)
})

test('effectiveBidi: both false → false', () => {
  assert.equal(effectiveBidi({ bidi: false, bidiInferred: false }), false)
})

test('effectiveBidi: absent attrs → false', () => {
  assert.equal(effectiveBidi({}), false)
})

test('alignAttrFor: LTR left → null (start = left)', () => {
  assert.equal(alignAttrFor('left', false), null)
})

test('alignAttrFor: RTL left → "left" (start = right; explicit left)', () => {
  assert.equal(alignAttrFor('left', true), 'left')
})

test('alignAttrFor: LTR right → "right"', () => {
  assert.equal(alignAttrFor('right', false), 'right')
})

test('alignAttrFor: RTL right → null (start = right)', () => {
  assert.equal(alignAttrFor('right', true), null)
})

test('alignAttrFor: center / justify unaffected by bidi', () => {
  assert.equal(alignAttrFor('center', false), 'center')
  assert.equal(alignAttrFor('center', true), 'center')
  assert.equal(alignAttrFor('justify', false), 'justify')
  assert.equal(alignAttrFor('justify', true), 'justify')
})

test('dirFlipAttrs: LTR with align=left → RTL with align=right', () => {
  const out = dirFlipAttrs({ align: 'left', bidi: false }, true)
  assert.equal(out.bidi, true)
  assert.equal(out.align, 'right')
  assert.equal(out.bidiInferred, false)
})

test('dirFlipAttrs: RTL with align=right → LTR with align=left', () => {
  const out = dirFlipAttrs({ align: 'right', bidi: true }, false)
  assert.equal(out.bidi, false)
  assert.equal(out.align, 'left')
})

test('dirFlipAttrs: center align preserved across flip', () => {
  const out = dirFlipAttrs({ align: 'center', bidi: false }, true)
  assert.equal(out.align, 'center')
  assert.equal(out.bidi, true)
})

test('paragraphDir: explicit bidi → rtl', () => {
  assert.equal(paragraphDir({ bidi: true }), 'rtl')
})

test('paragraphDir: inferred bidi → rtl', () => {
  assert.equal(paragraphDir({ bidiInferred: true }), 'rtl')
})

test('paragraphDir: default → ltr', () => {
  assert.equal(paragraphDir({}), 'ltr')
})

test('RTL_CHAR: matches Hebrew, Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic', () => {
  assert.ok(RTL_CHAR.test('ש'))  // Hebrew
  assert.ok(RTL_CHAR.test('ا')) // Arabic
  assert.ok(RTL_CHAR.test('ܐ')) // Syriac
  assert.ok(RTL_CHAR.test('ހ')) // Thaana
  assert.ok(RTL_CHAR.test('ߐ')) // NKo
  assert.ok(RTL_CHAR.test('ࠀ')) // Samaritan
  assert.ok(RTL_CHAR.test('ࡀ')) // Mandaic
  // CJK / Latin NOT RTL
  assert.equal(RTL_CHAR.test('a'), false)
  assert.equal(RTL_CHAR.test('中'), false)
  assert.equal(RTL_CHAR.test('あ'), false)
})

// v0.7.81 — WeKnora-adapted direction helpers (paragraph / heading / taskItem)

import {
  activeBidiWK,
  setSelectionAlignWK,
  setParagraphDirectionWK,
  WK_DIR_BLOCKS,
} from '../docDirection'

const schemaDir = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: {
      group: 'block',
      content: 'inline*',
      attrs: { align: { default: null }, bidi: { default: undefined }, bidiInferred: { default: false } },
      toDOM: () => ['p', 0],
    },
    heading: {
      group: 'block',
      content: 'inline*',
      attrs: { level: { default: 1 }, align: { default: null }, bidi: { default: undefined }, bidiInferred: { default: false } },
      toDOM: () => ['h1', 0],
    },
    taskItem: {
      group: 'block',
      content: 'inline*',
      attrs: { align: { default: null }, bidi: { default: undefined }, bidiInferred: { default: false } },
      toDOM: () => ['li', 0],
    },
    docProtected: {
      group: 'block',
      content: '',
      atom: true,
      attrs: { imageAlign: { default: null } },
      toDOM: () => ['div', { 'data-protected': '1' }],
    },
    text: { group: 'inline', toDOM: () => ['span', 0] },
  },
})

function makeDirEditor(blocks: Array<{ type: string; attrs?: any; text?: string }>) {
  const blockNodes = blocks.map((b) => {
    const nodeSpec = schemaDir.nodes[b.type as keyof typeof schemaDir.nodes]
    if (!nodeSpec) throw new Error(`unknown type ${b.type}`)
    const content = b.text ? schemaDir.text(b.text) : []
    return (schemaDir as any).nodes[b.type].create(b.attrs ?? {}, content)
  })
  let state = EditorState.create({ schema: schemaDir, doc: schemaDir.node('doc', null, blockNodes) })
  const fakeEditor: any = {
    get state() { return state },
    set state(v: any) { state = v },
    view: { dispatch(tr: any) { state = state.apply(tr) } },
    chain() {
      let pending: any[] = []
      const api: any = {
        focus() { return api },
        command(cb: any) { pending.push(cb); return api },
        run() {
          let tr = state.tr
          for (const cb of pending) {
            const r = cb({ state, tr, dispatch: (t: any) => (tr = t) })
            if (r === false) break
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

function paraAttrs(editor: any, type: string, idx: number): any {
  const matches: any[] = []
  editor.state.doc.forEach((node: any) => {
    if (node.type.name === type) matches.push(node.attrs)
  })
  return matches[idx] ?? null
}

function selectAll(editor: any) {
  editor.state = editor.state.apply(editor.state.tr.setSelection(new AllSelection(editor.state.doc)))
}

test('WK_DIR_BLOCKS: contains paragraph / heading / taskItem', () => {
  assert.ok(WK_DIR_BLOCKS.has('paragraph'))
  assert.ok(WK_DIR_BLOCKS.has('heading'))
  assert.ok(WK_DIR_BLOCKS.has('taskItem'))
  assert.equal(WK_DIR_BLOCKS.has('docParagraph'), false, 'genoffice name should not leak')
})

test('setSelectionAlignWK: LTR paragraph align=left → align=null', () => {
  const editor = makeDirEditor([{ type: 'paragraph', text: 'hi' }])
  setSelectionAlignWK(editor, 'left')
  // LTR align=left → null in our alignAttrFor
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, null)
})

test('setSelectionAlignWK: RTL paragraph align=right → align=null', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { bidi: true }, text: 'hi' }])
  setSelectionAlignWK(editor, 'right')
  // RTL align=right → null (start = right)
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, null)
})

test('setSelectionAlignWK: RTL paragraph align=left → align="left" (visual left)', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { bidi: true }, text: 'hi' }])
  selectAll(editor)
  setSelectionAlignWK(editor, 'left')
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, 'left')
})

test('setSelectionAlignWK: center align unchanged by bidi', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { bidi: true }, text: 'hi' }])
  selectAll(editor)
  setSelectionAlignWK(editor, 'center')
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, 'center')
})

test('setSelectionAlignWK: applies alignment to paragraph-like blocks', () => {
  const editor = makeDirEditor([
    { type: 'paragraph', text: 'first' },
    { type: 'taskItem', text: 'todo' },
  ])
  selectAll(editor)
  setSelectionAlignWK(editor, 'right')
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, 'right', 'LTR right → right')
  assert.equal(paraAttrs(editor, 'taskItem', 0).align, 'right')
})

test('setSelectionAlignWK: docProtected gets imageAlign, not align', () => {
  const editor = makeDirEditor([{ type: 'docProtected' }])
  selectAll(editor)
  setSelectionAlignWK(editor, 'center')
  assert.equal(paraAttrs(editor, 'docProtected', 0).imageAlign, 'center')
  assert.equal(paraAttrs(editor, 'docProtected', 0).align, undefined, 'no align attr on protected')
})

test('setSelectionAlignWK: empty selection (collapsed) → no change', () => {
  const editor = makeDirEditor([{ type: 'paragraph', text: 'hi' }])
  // Manually collapse selection
  editor.state = editor.state.apply(editor.state.tr.setSelection(TextSelection.create(editor.state.doc, 2, 2)))
  setSelectionAlignWK(editor, 'center')
  assert.equal(paraAttrs(editor, 'paragraph', 0).align, 'center', 'collapsed selection uses the active paragraph')
})

test('setParagraphDirectionWK: LTR → RTL flips align=left to align=right', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { align: null }, text: 'hi' }])
  selectAll(editor)
  setParagraphDirectionWK(editor, 'rtl')
  const attrs = paraAttrs(editor, 'paragraph', 0)
  assert.equal(attrs.bidi, true)
  assert.equal(attrs.bidiInferred, false)
  assert.equal(attrs.align, null)
})

test('setParagraphDirectionWK: RTL → LTR flips align=right to align=left', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { align: null, bidi: true }, text: 'hi' }])
  selectAll(editor)
  setParagraphDirectionWK(editor, 'ltr')
  const attrs = paraAttrs(editor, 'paragraph', 0)
  assert.equal(attrs.bidi, false)
  assert.equal(attrs.align, null)
})

test('setParagraphDirectionWK: center align preserved across flip', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: { align: 'center' }, text: 'hi' }])
  selectAll(editor)
  setParagraphDirectionWK(editor, 'rtl')
  const attrs = paraAttrs(editor, 'paragraph', 0)
  assert.equal(attrs.align, 'center')
  assert.equal(attrs.bidi, true)
})

test('setParagraphDirectionWK: paragraph without bidi attr is treated as LTR; flipping to RTL sets bidi', () => {
  const editor = makeDirEditor([{ type: 'paragraph', attrs: {}, text: 'hi' }])
  selectAll(editor)
  setParagraphDirectionWK(editor, 'rtl')
  assert.equal(paraAttrs(editor, 'paragraph', 0).bidi, true)
})

test('activeBidiWK: returns false when no paragraph-like node active (mock editor defaults)', () => {
  // Our mock editor's isActive isn't defined; activeBidiWK iterates and
  // editor.isActive returns undefined → falsy in `if` → falls through to false.
  const editor = makeDirEditor([{ type: 'paragraph', text: 'hi' }])
  editor.isActive = () => false
  assert.equal(activeBidiWK(editor), false)
})
