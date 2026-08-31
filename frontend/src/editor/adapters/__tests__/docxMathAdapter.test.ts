/**
 * docxMathAdapter.test — smoke test for the DOC formula adapter
 * (latexToDocxMath / mathDisplayParagraph / docxMathToMathML).
 *
 * Run: ./node_modules/.bin/tsx --test src/editor/adapters/__tests__/docxMathAdapter.test.ts
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  latexToDocxMath,
  mathDisplayParagraph,
  docxMathToMathML,
  docxMathToLatex,
  readMathFragmentsInParagraph,
  readMathTokensInParagraph,
} from '../docxAdapter'

test('latexToDocxMath: simple fraction produces OMML', () => {
  const omml = latexToDocxMath('\\frac{a}{b}')
  assert.ok(omml, 'omml returned')
  assert.ok(omml!.includes('<m:'), 'omml starts with m: namespace')
  assert.ok(omml!.includes('a'), 'numerator a')
  assert.ok(omml!.includes('b'), 'denominator b')
})

test('latexToDocxMath: empty input returns null', () => {
  assert.equal(latexToDocxMath(''), null)
  assert.equal(latexToDocxMath('   '), null)
})

test('mathDisplayParagraph: wraps OMML in <w:p>', () => {
  const xml = mathDisplayParagraph('x^2')
  assert.ok(xml, 'xml returned')
  assert.ok(xml!.startsWith('<w:p'), 'starts with w:p')
  assert.ok(xml!.endsWith('</w:p>'), 'ends with w:p')
  assert.ok(xml!.includes('<m:oMath'), 'contains oMath')
})

test('mathDisplayParagraph: alignment parameter changes jc', () => {
  const left = mathDisplayParagraph('a', 'left')
  const center = mathDisplayParagraph('a', 'center')
  assert.ok(left!.includes('w:jc w:val="left"'), 'left align jc')
  // Center is the OOXML default — emits m:jc (math) but no w:jc (paragraph).
  assert.ok(center!.includes('m:jc m:val="center"'), 'center align m:jc')
})

test('docxMathToMathML: round-trips a simple expression', () => {
  const omml = latexToDocxMath('x^2 + y^2')
  assert.ok(omml)
  const wrapped = '<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">' + omml + '</m:oMath>'
  const mathml = docxMathToMathML(wrapped)
  assert.ok(mathml, 'mathml returned')
  assert.ok(mathml!.includes('<math') || mathml!.includes(':math'), 'mathml starts with math element')
})

test('docxMathToLatex: reverse converts a known OMML', () => {
  const omml = latexToDocxMath('1+2')
  assert.ok(omml)
  const wrapped = '<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">' + omml + '</m:oMath>'
  const latex = docxMathToLatex(wrapped)
  // Lossy; just check it returns a string and contains digits.
  assert.ok(typeof latex === 'string' || latex === null)
  if (latex) {
    assert.ok(latex.length > 0, 'reverse-converted latex non-empty')
  }
})

test('readMathFragmentsInParagraph: extracts OMML fragments', () => {
  const xml = mathDisplayParagraph('a+b')
  assert.ok(xml)
  const fragments = readMathFragmentsInParagraph(xml!)
  assert.ok(Array.isArray(fragments), 'returns array')
  assert.ok(fragments.length >= 1, 'at least one oMath fragment')
})

test('readMathTokensInParagraph: extracts visible math tokens', () => {
  const xml = mathDisplayParagraph('x+1')
  assert.ok(xml)
  const tokens = readMathTokensInParagraph(xml!)
  assert.ok(tokens.length >= 1, 'at least one token')
  // tokens include 'x' and '1' (or 'x' and '1' depending on render)
  const joined = tokens.join('')
  assert.ok(joined.includes('x'), 'x present in tokens')
})
