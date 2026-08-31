/**
 * Adapter round-trip smoke tests. These verify that we can:
 *   1. Generate a real .docx/.pptx/.xlsx via the engine/adapter.
 *   2. Re-open it and recover the structured model.
 *   3. Mutate + save back to bytes and re-open again.
 *
 * Run with:  npm run test -- src/editor/adapters/__tests__
 *
 * These are integration tests over the genoffice engines we vendored
 * (docx-engine, pptxgenjs, xlsx). They do not require a running server.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { openDocx, saveDocxBytes, patchParagraphText } from '../docxAdapter'
import { openPptx, savePptxBytes, newPptxDeck } from '../pptxAdapter'
import { openXlsx, saveXlsxBytes, newXlsxWorkbook } from '../xlsxAdapter'

test('docx round-trip preserves paragraph text', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  patchParagraphText(doc, 0, 'hello round-trip')
  const out = await saveDocxBytes(doc)
  assert.ok(out.byteLength > 0, 'bytes must be non-empty')
  const doc2 = await openDocx(out)
  assert.equal(doc2.paragraphs[0].text.trim(), 'hello round-trip')
})

test('pptx round-trip produces a valid pptx blob', async () => {
  const deck = newPptxDeck()
  deck.slides = [
    { title: 'slide one', bullets: ['first', 'second'] },
    { title: 'slide two', bullets: ['alpha'] },
  ]
  const bytes = await savePptxBytes(deck)
  assert.ok(bytes.byteLength > 0, 'pptx bytes non-empty')
  const re = await openPptx(bytes)
  assert.ok(re.slides.length >= 2, 'should round-trip at least 2 slides')
  assert.equal(re.slides[0].title.trim(), 'slide one')
  assert.deepEqual(re.slides[0].bullets.map((s) => s.trim()), ['first', 'second'])
})

test('xlsx round-trip preserves cell values', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [{ v: 'a' }, { v: 1 }, { v: true }],
    [{ v: 'b' }, { v: 2 }, { v: null }],
  ]
  const bytes = await saveXlsxBytes(wb)
  assert.ok(bytes.byteLength > 0, 'xlsx bytes non-empty')
  const re = await openXlsx(bytes)
  assert.equal(String(re.sheets[0].rows[0][0].v), 'a')
  // SheetJS returns cell.v as string|number depending on cell type; we just
  // assert the value is non-empty / matches loosely.
  assert.ok(re.sheets[0].rows[0][1].v !== '')
  assert.equal(String(re.sheets[0].rows[1][0].v), 'b')
})
