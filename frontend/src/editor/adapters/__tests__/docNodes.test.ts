// v0.7.49 — lightweight DOC node config tests (copy from genoffice extensions.ts).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DocInlineMath, DocProtected } from '../docNodes'

test('DocInlineMath is an atomic inline node with omml/mathml/latex/text attrs', () => {
  assert.equal(DocInlineMath.name, 'docInlineMath')
  assert.equal(DocInlineMath.config.inline, true)
  assert.equal(DocInlineMath.config.group, 'inline')
  assert.equal(DocInlineMath.config.atom, true)
  const attrs = DocInlineMath.config.addAttributes?.() ?? {}
  assert.deepEqual(Object.keys(attrs).sort(), ['latex', 'mathml', 'omml', 'text'])
  assert.equal((attrs.omml as { default: string }).default, '')
  assert.equal((attrs.text as { default: string }).default, '')
})

test('DocInlineMath renderHTML emits span[data-inline-math] with token text', () => {
  const node = {
    attrs: { omml: '<m:oMath/>', mathml: '<math/>', latex: 'E=mc^2', text: 'E=mc2' },
  } as never
  const spec = DocInlineMath.config.renderHTML?.({ node } as never) as unknown as [
    string,
    Record<string, string>,
    string,
  ]
  assert.equal(spec[0], 'span')
  assert.equal(spec[1]['data-inline-math'], '1')
  assert.equal(spec[1].class, 'doc-inline-math')
  assert.equal(spec[2], 'E=mc2')
})

test('DocProtected is a block atom with formula subset attrs', () => {
  assert.equal(DocProtected.name, 'docProtected')
  assert.equal(DocProtected.config.group, 'block')
  assert.equal(DocProtected.config.atom, true)
  const attrs = DocProtected.config.addAttributes?.() ?? {}
  assert.deepEqual(
    Object.keys(attrs).sort(),
    ['blockType', 'docxIndex', 'formulaDisplay', 'genXml', 'label', 'previewText'],
  )
  assert.equal((attrs.blockType as { default: string }).default, 'passthrough')
})

test('DocProtected renderHTML emits div[data-doc-protected] with latex payload', () => {
  const node = {
    attrs: {
      docxIndex: 3,
      blockType: 'passthrough',
      label: '公式',
      previewText: 'x^2 + y^2 = z^2',
      genXml: '<w:p/>',
      formulaDisplay: { latex: 'x^2 + y^2 = z^2', mathml: '<math/>', omml: '<m:oMath/>', tokens: [] },
    },
  } as never
  const spec = DocProtected.config.renderHTML?.({ node } as never) as unknown as [
    string,
    Record<string, string>,
    string,
  ]
  assert.equal(spec[0], 'div')
  assert.equal(spec[1]['data-doc-protected'], '1')
  assert.equal(spec[1].class, 'doc-protected')
  assert.equal(spec[1]['data-latex'], 'x^2 + y^2 = z^2')
  assert.equal(spec[2], 'x^2 + y^2 = z^2')
})
