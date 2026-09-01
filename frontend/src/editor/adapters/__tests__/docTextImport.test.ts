// v0.7.58 — Tencent Docs compatibility: .txt/.md → DOC paragraphs.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  textToDocParagraphs,
  markdownToDocParagraphs,
  stripInlineMarkdown,
  looksLikeMarkdown,
  importTextToDocParagraphs,
} from '../docTextImport'

test('textToDocParagraphs: one line per paragraph, blank lines dropped', () => {
  const out = textToDocParagraphs('line1\n\nline2\r\nline3')
  assert.deepEqual(out, [
    { text: 'line1', kind: 'paragraph' },
    { text: 'line2', kind: 'paragraph' },
    { text: 'line3', kind: 'paragraph' },
  ])
  assert.deepEqual(textToDocParagraphs(''), [{ text: '', kind: 'paragraph' }])
})

test('markdownToDocParagraphs: headings / lists / quotes / code', () => {
  const out = markdownToDocParagraphs(
    '# Title\n\nSome **bold** text.\n\n- item1\n- item2\n\n> quote\n\n```js\ncode\n```',
  )
  assert.deepEqual(out, [
    { text: 'Title', kind: 'heading', level: 1 },
    { text: 'Some bold text.', kind: 'paragraph' },
    { text: 'item1', kind: 'listItem' },
    { text: 'item2', kind: 'listItem' },
    { text: 'quote', kind: 'paragraph' },
    { text: 'code', kind: 'paragraph' },
  ])
})

test('stripInlineMarkdown: bold / italic / code / link', () => {
  assert.equal(stripInlineMarkdown('**bold** and *it* and `code` and [link](https://x.com)'), 'bold and it and code and link')
  assert.equal(stripInlineMarkdown('plain'), 'plain')
})

test('looksLikeMarkdown: heading / fence / link / repeated lists', () => {
  assert.equal(looksLikeMarkdown('# Title\nbody'), true)
  assert.equal(looksLikeMarkdown('```js\ncode\n```'), true)
  assert.equal(looksLikeMarkdown('see [docs](https://x.com)'), true)
  assert.equal(looksLikeMarkdown('- a\n- b'), true)
  assert.equal(looksLikeMarkdown('just prose with a # hash'), false)
  assert.equal(looksLikeMarkdown('single - dash'), false)
})

test('importTextToDocParagraphs: auto-detects markdown vs plain text', () => {
  const md = importTextToDocParagraphs('# Title\n\n- a\n- b')
  assert.equal(md[0]?.kind, 'heading')
  assert.equal(md[0]?.level, 1)
  const plain = importTextToDocParagraphs('just some\nplain text')
  assert.deepEqual(plain, [
    { text: 'just some', kind: 'paragraph' },
    { text: 'plain text', kind: 'paragraph' },
  ])
})
