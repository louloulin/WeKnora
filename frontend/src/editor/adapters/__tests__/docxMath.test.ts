/**
 * v0.7.32 — DOC OMML math formula smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  latexToDocxMath,
  mathDisplayParagraph,
  readMathFragmentsInParagraph,
  readMathTokensInParagraph,
} from '../docxAdapter'

test('latexToDocxMath converts a simple formula into OMML', () => {
  const omml = latexToDocxMath('E = mc^2')
  assert.ok(omml)
  assert.match(omml!, /<m:/, 'OMML fragment should contain m:* tags')
})

test('mathDisplayParagraph wraps OMML in a <w:p> paragraph', () => {
  const para = mathDisplayParagraph('\\sqrt{x+1}', 'center')
  assert.ok(para)
  assert.match(para!, /^<w:p>/)
  assert.match(para!, /<m:oMathPara>/)
  assert.match(para!, /<\/w:p>$/)
})

test('mathDisplayParagraph returns null on blank latex', () => {
  const para = mathDisplayParagraph('')
  assert.equal(para, null)
})

test('readMathFragmentsInParagraph finds OMML inside a wrapped paragraph', () => {
  const para = mathDisplayParagraph('a^2 + b^2 = c^2', 'left')
  assert.ok(para)
  const frags = readMathFragmentsInParagraph(para!)
  assert.ok(Array.isArray(frags))
  assert.ok(frags.length >= 1, 'expected ≥1 OMML fragment in the paragraph')
})

test('readMathTokensInParagraph extracts plain-text math content', () => {
  const para = mathDisplayParagraph('1 + 2', 'center')
  assert.ok(para)
  const tokens = readMathTokensInParagraph(para!)
  // At least the numbers should appear as tokens; the spaces may or may not.
  const joined = tokens.join('')
  assert.match(joined, /[12]/)
})

test('latexToDocxMath supports superscript via ^', () => {
  const omml = latexToDocxMath('x^2')
  assert.ok(omml)
  assert.match(omml!, /<m:sSup>/, 'expected sSup element for x^2')
})
