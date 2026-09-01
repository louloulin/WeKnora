// v0.7.49 - DOC equation adapter tests (copy from genoffice docs/editor/equation.ts).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  inlineMathML,
  inlineEquationNodeJson,
  equationBlockJson,
} from '../docEquation'

test('inlineMathML strips block display flag from OMML-derived MathML', () => {
  // Build a real OMML payload via inlineEquationNodeJson, then ask the adapter
  // to wrap it in MathML with display=inline. ommlToMathML emits display="block"
  // by default; inlineMathML must rewrite it.
  const node = inlineEquationNodeJson('x')
  const omml = node.attrs!.omml as string
  const out = inlineMathML(omml)
  assert.equal(typeof out, 'string')
  assert.ok(out.length > 0, 'inlineMathML returned empty string')
  assert.match(out, /display="inline"/)
  assert.doesNotMatch(out, /display="block"/)
})

test('inlineEquationNodeJson returns docInlineMath node with omml/mathml/latex/text', () => {
  const node = inlineEquationNodeJson('E=mc^2')
  assert.equal(node.type, 'docInlineMath')
  assert.ok(node.attrs && typeof node.attrs === 'object')
  const a = node.attrs as Record<string, unknown>
  assert.equal(typeof a.omml, 'string')
  assert.match(a.omml as string, /^<m:oMath>[\s\S]*<\/m:oMath>$/)
  assert.equal(typeof a.mathml, 'string')
  assert.equal(a.latex, 'E=mc^2')
  assert.equal(typeof a.text, 'string')
  assert.match(a.text as string, /E/)
})

test('equationBlockJson returns docProtected node carrying formulaDisplay', () => {
  const node = equationBlockJson('x^2 + y^2 = z^2')
  assert.equal(node.type, 'docProtected')
  const a = node.attrs as Record<string, unknown>
  assert.equal(a.blockType, 'passthrough')
  assert.equal(typeof a.label, 'string')
  assert.equal(a.previewText, 'x^2 + y^2 = z^2')
  assert.equal(typeof a.genXml, 'string')
  assert.match(a.genXml as string, /<m:oMathPara/)
  const fd = a.formulaDisplay as Record<string, unknown>
  assert.ok(fd && typeof fd === 'object')
  assert.equal(typeof fd.omml, 'string')
  assert.equal(typeof fd.mathml, 'string')
  assert.equal(typeof fd.latex, 'string')
  assert.ok(Array.isArray(fd.tokens))
})

test('equationBlockJson trims whitespace in latex preview', () => {
  const node = equationBlockJson('  a + b  ')
  const a = node.attrs as Record<string, unknown>
  assert.equal(a.previewText, 'a + b')
  assert.equal((a.formulaDisplay as Record<string, unknown>).latex, 'a + b')
})
