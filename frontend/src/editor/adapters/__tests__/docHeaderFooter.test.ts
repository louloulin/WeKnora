/**
 * docHeaderFooter.test — v0.7.109 DOC header/footer UI helpers
 *
 * 验证：
 *  - hfSegmentsOf 解析 PAGE_TOKEN ('#'), 'PAGES#', 'DATE#'
 *  - hfInlineHtml 输出可被 Vue 渲染的 chip spans
 *  - isEmptyHf 区分空值与占位
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { hfSegmentsOf, hfInlineHtml, isEmptyHf, PAGE_TOKEN, defaultHeader, defaultFooter } from '../docHeaderFooter'

test('hfSegmentsOf — plain text', () => {
  const segs = hfSegmentsOf('Hello world')
  assert.equal(segs.length, 1)
  assert.equal(segs[0].kind, 'text')
  assert.equal(segs[0].value, 'Hello world')
})

test('hfSegmentsOf — page token', () => {
  const segs = hfSegmentsOf('Page # of 10')
  assert.equal(segs.length, 3)
  assert.equal(segs[0].kind, 'text')
  assert.equal(segs[0].value, 'Page ')
  assert.equal(segs[1].kind, 'page')
  assert.equal(segs[1].value, '#')
  assert.equal(segs[2].kind, 'text')
  assert.equal(segs[2].value, ' of 10')
})

test('hfSegmentsOf — NUMPAGES token (PAGES#)', () => {
  const segs = hfSegmentsOf('Page # of #PAGES#')
  assert.equal(segs.length, 4)
  assert.equal(segs[0].kind, 'text')
  assert.equal(segs[1].kind, 'page')
  assert.equal(segs[2].kind, 'text')
  assert.equal(segs[2].value, ' of ')
  assert.equal(segs[3].kind, 'pages')
  assert.equal(segs[3].value, 'PAGES')
})

test('hfSegmentsOf — DATE token (#DATE#)', () => {
  const segs = hfSegmentsOf('Created on #DATE#')
  assert.equal(segs.length, 2)
  assert.equal(segs[0].kind, 'text')
  assert.equal(segs[0].value, 'Created on ')
  assert.equal(segs[1].kind, 'date')
  assert.equal(segs[1].value, 'DATE')
})

test('hfSegmentsOf — empty string returns []', () => {
  assert.deepEqual(hfSegmentsOf(''), [])
})

test('hfInlineHtml — text-only', () => {
  const html = hfInlineHtml('Hello')
  assert.equal(html, 'Hello')
})

test('hfInlineHtml — text + chip', () => {
  const html = hfInlineHtml('Page # of 10')
  assert.ok(html.includes('Page '))
  assert.ok(html.includes('<span class="doc-hf-chip doc-hf-chip--page">#'))
  assert.ok(html.includes('</span>'))
})

test('hfInlineHtml — escapes < / > / &', () => {
  const html = hfInlineHtml('A < B & C')
  assert.ok(html.includes('A &lt; B &amp; C'))
})

test('isEmptyHf — null / undefined / empty text / with PAGE_TOKEN', () => {
  assert.equal(isEmptyHf(null), true)
  assert.equal(isEmptyHf(undefined), true)
  assert.equal(isEmptyHf({ text: '' }), true)
  assert.equal(isEmptyHf({ text: PAGE_TOKEN, pageNumber: true }), false)
  assert.equal(isEmptyHf({ text: 'Header' }), false)
})

test('defaultHeader — empty text, no page number', () => {
  const h = defaultHeader()
  assert.equal(h.text, '')
  assert.equal(h.pageNumber, undefined)
})

test('defaultFooter(pageNumber=true) — page token', () => {
  const f = defaultFooter(true)
  assert.equal(f.text, PAGE_TOKEN)
  assert.equal(f.pageNumber, true)
})

test('defaultFooter(pageNumber=false) — empty text', () => {
  const f = defaultFooter(false)
  assert.equal(f.text, '')
})
