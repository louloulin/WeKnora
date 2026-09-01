// v0.7.70 — DOC find/replace + word count helpers

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { EditorState, TextSelection } from '@tiptap/pm/state'
import type { Transaction } from '@tiptap/pm/state'
import { Schema } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/core'
import {
  foldCase,
  findMatches,
  replaceMatch,
  replaceAllMatches,
  computeDocStats,
} from '../docFind'

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*', marks: '_' },
    heading: { group: 'block', content: 'inline*', marks: '_' },
    text: { group: 'inline' },
    hard_break: { inline: true, group: 'inline' },
    protected_thing: {
      group: 'block',
      atom: true,
      content: 'text*', // leaf node; matches our "protected" shape
      toDOM: () => ['div', { 'data-protected': '1' }],
      parseDOM: [{ tag: 'div[data-protected]' }],
    },
  },
  marks: {},
})

function makeEditor(content: { type: 'paragraph' | 'heading' | 'protected'; text?: string }[]) {
  const blocks = content.map((b) => {
    if (b.type === 'protected') {
      return schema.nodes.protected_thing!.create(null, schema.text(b.text ?? ''))
    }
    const node = schema.nodes[b.type]!.create(null, b.text ? schema.text(b.text) : [])
    return node
  })
  let state = EditorState.create({ schema, doc: schema.node('doc', null, blocks) })
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

test('foldCase: ASCII lowercase', () => {
  assert.equal(foldCase('Hello World'), 'hello world')
})

test('foldCase: preserves length for Turkish dotted I (no offset shift)', () => {
  const out = foldCase('İSTANBUL')
  assert.equal(out.length, 'İSTANBUL'.length)
})

test('findMatches: empty query returns no matches', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'hello world' }])
  assert.equal(findMatches(editor, '', { matchCase: false, wholeWord: false }).length, 0)
})

test('findMatches: simple substring finds all occurrences', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'hello hello hello' }])
  const m = findMatches(editor, 'hello', { matchCase: false, wholeWord: false })
  assert.equal(m.length, 3)
})

test('findMatches: case-insensitive default', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'Hello HELLO hello' }])
  const m = findMatches(editor, 'hello', { matchCase: false, wholeWord: false })
  assert.equal(m.length, 3)
})

test('findMatches: case-sensitive respects casing', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'Hello HELLO hello' }])
  const m = findMatches(editor, 'hello', { matchCase: true, wholeWord: false })
  assert.equal(m.length, 1)
})

test('findMatches: whole-word boundary filter', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'cat scatter category' }])
  const m = findMatches(editor, 'cat', { matchCase: false, wholeWord: true })
  assert.equal(m.length, 1, 'only standalone "cat" matches')
})

test('findMatches: skips protected blocks', () => {
  const { editor } = makeEditor([
    { type: 'paragraph', text: 'visible secret' },
    { type: 'protected', text: 'secret' },
  ])
  const m = findMatches(editor, 'secret', { matchCase: false, wholeWord: false })
  // protected block has node name 'protected_thing' → skipped
  assert.equal(m.length, 1)
})

test('replaceMatch: replaces a single range with new text', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'hello world' }])
  // locate 'world' (positions 7..12 in the doc)
  const ranges = findMatches(editor, 'world', { matchCase: false, wholeWord: false })
  assert.equal(ranges.length, 1)
  replaceMatch(editor, ranges[0]!, 'everyone')
  const after = editor.state.doc.textBetween(0, editor.state.doc.content.size, ' ', ' ')
  assert.equal(after.trim(), 'hello everyone')
})

test('replaceAllMatches: replaces every match', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'cat cat cat dog' }])
  const ranges = findMatches(editor, 'cat', { matchCase: false, wholeWord: false })
  const count = replaceAllMatches(editor, ranges, 'kitten')
  assert.equal(count, 3)
  const after = editor.state.doc.textBetween(0, editor.state.doc.content.size, ' ', ' ')
  assert.equal(after.trim(), 'kitten kitten kitten dog')
})

test('replaceAllMatches: empty matches returns 0', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: 'no match' }])
  const count = replaceAllMatches(editor, [], 'x')
  assert.equal(count, 0)
})

test('computeDocStats: counts words, paragraphs, lines, chars', () => {
  const { editor } = makeEditor([
    { type: 'paragraph', text: 'Hello world' },
    { type: 'paragraph', text: 'foo bar baz' },
    { type: 'heading', text: 'Title' },
  ])
  const stats = computeDocStats(editor)
  assert.equal(stats.paragraphs, 3)
  assert.equal(stats.nonAsianWords, 6) // 2 + 3 + 1
  assert.equal(stats.asianChars, 0)
  // "Hello world" = 11 chars (10 + 1 space) → charsNoSpace = 10
  // "foo bar baz" = 11 chars (8 + 2 spaces) → charsNoSpace = 9
  // "Title" = 5 chars, no spaces → charsNoSpace = 5
  assert.equal(stats.charsNoSpace, 24)
  assert.equal(stats.charsWithSpace, 27) // 11 + 11 + 5
  assert.equal(stats.lines, 3)
})

test('computeDocStats: counts CJK characters as 1 each', () => {
  const { editor } = makeEditor([{ type: 'paragraph', text: '你好，世界' }])
  const stats = computeDocStats(editor)
  // 你/好/世/界 are CJK ideographs (4) + ，is fullwidth comma (CJK punctuation)
  assert.equal(stats.asianChars, 5)
  assert.equal(stats.words, 5)
  assert.equal(stats.nonAsianWords, 0)
})
