// v0.7.74 — DOC markdown plain-text paste detection + HTML conversion.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  looksLikeMarkdown,
  markdownPasteHtml,
} from '../docMarkdownPaste'

test('looksLikeMarkdown: ATX heading → true', () => {
  assert.equal(looksLikeMarkdown('# Hello world'), true)
  assert.equal(looksLikeMarkdown('## Subhead'), true)
  assert.equal(looksLikeMarkdown('###### tiny'), true)
})

test('looksLikeMarkdown: fenced code block → true', () => {
  assert.equal(looksLikeMarkdown('```\nfoo\n```'), true)
  assert.equal(looksLikeMarkdown('```js\nfoo()\n```'), true)
})

test('looksLikeMarkdown: link → true', () => {
  assert.equal(looksLikeMarkdown('See [docs](https://example.com)'), true)
})

test('looksLikeMarkdown: bold (**) → true', () => {
  assert.equal(looksLikeMarkdown('**important**'), true)
  // __bold__ is NOT triggered (collides with __init__ dunder names)
  assert.equal(looksLikeMarkdown('__init__'), false)
})

test('looksLikeMarkdown: table → true', () => {
  assert.equal(
    looksLikeMarkdown('| a | b |\n|---|---|\n| 1 | 2 |\n'),
    true,
  )
})

test('looksLikeMarkdown: repeated list lines → true', () => {
  assert.equal(looksLikeMarkdown('- one\n- two\n- three\n'), true)
  assert.equal(looksLikeMarkdown('1. first\n2. second\n'), true)
})

test('looksLikeMarkdown: repeated quote lines → true', () => {
  assert.equal(looksLikeMarkdown('> quoted\n> again\n'), true)
})

test('looksLikeMarkdown: single list line — false (avoid accidental conversions)', () => {
  assert.equal(looksLikeMarkdown('- only one line'), false)
  assert.equal(looksLikeMarkdown('> single quote'), false)
})

test('looksLikeMarkdown: plain prose — false', () => {
  assert.equal(looksLikeMarkdown('Hello world. Just a paragraph.'), false)
  assert.equal(looksLikeMarkdown('Numbers like 1.5 and codes like #123 are fine.'), false)
})

test('looksLikeMarkdown: lone # — false', () => {
  // Pound in prose should not trigger
  assert.equal(looksLikeMarkdown('Issue #123 is fixed'), false)
})

test('markdownPasteHtml: returns null for non-markdown', () => {
  assert.equal(markdownPasteHtml('Just plain text.'), null)
})

test('markdownPasteHtml: ATX heading → <h1>', () => {
  const html = markdownPasteHtml('# Title')
  assert.ok(html)
  assert.match(html ?? '', /<h1[^>]*>Title<\/h1>/)
})

test('markdownPasteHtml: link → <a>', () => {
  const html = markdownPasteHtml('[label](https://example.com)')
  assert.ok(html)
  assert.match(html ?? '', /<a href="https:\/\/example\.com">label<\/a>/)
})

test('markdownPasteHtml: fenced code → <pre><code>', () => {
  const html = markdownPasteHtml('```\nfoo\nbar\n```')
  assert.ok(html)
  assert.match(html ?? '', /<pre[^>]*><code[^>]*>foo[\s\S]*<\/code><\/pre>/)
})

test('markdownPasteHtml: bold (**) → <strong>', () => {
  const html = markdownPasteHtml('**hi**')
  assert.ok(html)
  assert.match(html ?? '', /<strong>hi<\/strong>/)
})

test('markdownPasteHtml: handles CRLF input', () => {
  const html = markdownPasteHtml('# Win\r\n\r\nLine')
  assert.ok(html)
  assert.match(html ?? '', /<h1[^>]*>Win<\/h1>/)
})
