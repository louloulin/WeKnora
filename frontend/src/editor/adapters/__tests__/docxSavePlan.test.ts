// v0.7.51 — full SavePlan: docProtected formula blocks + docxIndex anchoring.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { openDocx, pmDocToSavePlan, saveDocxBytes, parseDocxIndexFromNode } from '../docxAdapter'
import { buildBlankDocx } from '../../engines/docx-engine/index'

async function blankDoc(): Promise<Awaited<ReturnType<typeof openDocx>>> {
  const seed = await buildBlankDocx()
  return openDocx(seed)
}

test('parseDocxIndexFromNode reads data-docx-index and docxIndex attrs', () => {
  assert.equal(parseDocxIndexFromNode({ type: 'paragraph', attrs: { 'data-docx-index': 3 } }), 3)
  assert.equal(parseDocxIndexFromNode({ type: 'docProtected', attrs: { docxIndex: 5 } }), 5)
  assert.equal(parseDocxIndexFromNode({ type: 'paragraph' }), null)
  assert.equal(parseDocxIndexFromNode({ type: 'paragraph', attrs: { 'data-docx-index': '2' } }), 2)
})

test('pmDocToSavePlan emits genXml for docProtected formula blocks', async () => {
  const doc = await blankDoc()
  const pmDoc = {
    type: 'doc',
    content: [
      { type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [{ type: 'text', text: 'before' }] },
      {
        type: 'docProtected',
        attrs: {
          docxIndex: 1,
          blockType: 'passthrough',
          previewText: 'x^2',
          genXml: '<w:p><w:r><m:oMathPara/></w:r></w:p>',
          formulaDisplay: { latex: 'x^2', mathml: '<math/>', omml: '<m:oMath/>', tokens: ['x'] },
        },
      },
      { type: 'paragraph', attrs: { 'data-docx-index': 2 }, content: [{ type: 'text', text: 'after' }] },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  const formula = plan.blocks.find((b) => b.kind === 'xml' && b.docxIndex === 1)
  assert.ok(formula, 'formula block must be emitted')
  assert.equal(formula.kind, 'xml')
  assert.equal((formula as { xml: string }).xml, '<w:p><w:r><m:oMathPara/></w:r></w:p>')
  assert.equal(plan.textByIndex.get(1), 'x^2')
})

test('pmDocToSavePlan emits generated for a trailing new paragraph', async () => {
  const doc = await blankDoc()
  // Original doc has 1 paragraph (index 0). The user edited it and appended
  // a brand-new paragraph with no anchor: the new one must emit 'generated'.
  const pmDoc = {
    type: 'doc',
    content: [
      { type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [{ type: 'text', text: 'original' }] },
      { type: 'paragraph', content: [{ type: 'text', text: 'inserted' }] },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  assert.equal(plan.blocks[0].kind, 'xml')
  assert.equal((plan.blocks[0] as { docxIndex: number }).docxIndex, 0)
  assert.equal(plan.blocks[1].kind, 'generated')
})

test('saveDocxBytes accepts a full SaveBlock[] plan', async () => {
  const doc = await blankDoc()
  const plan = pmDocToSavePlan(
    {
      type: 'doc',
      content: [
        { type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [{ type: 'text', text: 'edited' }] },
      ],
    } as never,
    doc,
  )
  const bytes = await saveDocxBytes(doc, plan.blocks)
  assert.ok(bytes.length > 0, 'must produce bytes')
  // Re-open and confirm the edit survived.
  const reopened = await openDocx(bytes)
  assert.equal(reopened.paragraphs[0].text, 'edited')
})
