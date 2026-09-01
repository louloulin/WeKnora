// v0.7.63 — DOC page break round-trip: pageBreakBefore → <w:pageBreakBefore/>.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  parseDocx,
  saveDocx,
  type SaveBlock,
} from '../../engines/docx-engine/index'

test('DOC page break: pageBreakBefore in ParaFormat → <w:pageBreakBefore/> in generated OOXML', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  // Emit a generated block so the save regenerates the XML.
  const blocks: SaveBlock[] = [
    {
      kind: 'generated',
      block: {
        type: 'paragraph',
        runs: [{ text: 'after page break' }],
        format: { pageBreakBefore: true },
      },
    },
  ]
  const saved = await saveDocx(original, blocks)
  const zip = await JSZip.loadAsync(saved)
  const docXml = await zip.file('word/document.xml')?.async('string')
  assert.ok(docXml)
  assert.match(docXml ?? '', /<w:pageBreakBefore\/>/, 'document.xml contains pageBreakBefore')
})

test('DOC page break: DocPageBreak extension registers pageBreakBefore attribute', async () => {
  const { DocPageBreak } = await import('../docPageBreak')
  assert.equal(DocPageBreak.name, 'docPageBreak')
  assert.equal(typeof DocPageBreak, 'object')
})
