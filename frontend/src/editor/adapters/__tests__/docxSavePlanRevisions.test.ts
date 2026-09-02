// v0.7.106 — DOC track-changes save plan: ins / del marks on text runs must
// propagate into Run.ins / Run.del so the engine emits <w:ins> / <w:del>.

import { test } from 'node:test'
import { extractDocxDocumentXml } from './_testZipExtract'
import assert from 'node:assert/strict'
import { pmNodeToGeneratedBlock, pmDocToSavePlan } from '../docxAdapter'
import { buildBlankDocx } from '../../engines/docx-engine/index'
import { openDocx } from '../docxAdapter'

function paraWithInsDel() {
  return {
    type: 'paragraph',
    attrs: { 'data-docx-index': 0 },
    content: [
      {
        type: 'text',
        text: 'kept',
      },
      {
        type: 'text',
        text: 'inserted',
        marks: [
          {
            type: 'ins',
            attrs: { author: 'alice', date: '2026-09-02T10:00:00.000Z', id: '1001' },
          },
        ],
      },
      {
        type: 'text',
        text: 'removed',
        marks: [
          {
            type: 'del',
            attrs: { author: 'bob', date: '2026-09-02T10:05:00.000Z', id: '1002' },
          },
        ],
      },
    ],
  }
}

test('pmNodeToGeneratedBlock propagates ins/del marks into Run.ins / Run.del', () => {
  const node = paraWithInsDel()
  const block = pmNodeToGeneratedBlock(node as never)
  assert.equal(block.runs.length, 3)
  // "kept" — no revision
  assert.equal(block.runs[0].text, 'kept')
  assert.equal(block.runs[0].ins, undefined)
  assert.equal(block.runs[0].del, undefined)
  // "inserted" — wrapped in w:ins
  assert.equal(block.runs[1].text, 'inserted')
  assert.equal(block.runs[1].ins?.author, 'alice')
  assert.equal(block.runs[1].ins?.date, '2026-09-02T10:00:00.000Z')
  assert.equal(block.runs[1].ins?.id, '1001')
  assert.equal(block.runs[1].del, undefined)
  // "removed" — wrapped in w:del
  assert.equal(block.runs[2].text, 'removed')
  assert.equal(block.runs[2].del?.author, 'bob')
  assert.equal(block.runs[2].del?.date, '2026-09-02T10:05:00.000Z')
  assert.equal(block.runs[2].del?.id, '1002')
  assert.equal(block.runs[2].ins, undefined)
})

test('pmNodeToGeneratedBlock coalesces consecutive ins runs sharing (author,date,id)', () => {
  const node = {
    type: 'paragraph',
    attrs: { 'data-docx-index': 0 },
    content: [
      { type: 'text', text: 'foo', marks: [{ type: 'ins', attrs: { author: 'a', date: 'd1', id: '1' } }] },
      { type: 'text', text: 'bar', marks: [{ type: 'ins', attrs: { author: 'a', date: 'd1', id: '1' } }] },
    ],
  }
  const block = pmNodeToGeneratedBlock(node as never)
  // Different signature (text content) => two runs; both still carry ins.
  assert.ok(block.runs.length >= 1)
  assert.equal(block.runs[0].ins?.author, 'a')
})

test('pmDocToSavePlan emits a generated block (not original) when a paragraph gains an ins mark', async () => {
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  const pmDoc = {
    type: 'doc',
    content: [paraWithInsDel()],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  // The first (and only) block must be 'generated' (we added ins/del marks),
  // never 'original' (text-only fast path) — otherwise saveDocxBytes would
  // re-emit the original paragraph bytes and silently drop the w:ins/w:del.
  assert.ok(plan.blocks.length >= 1)
  const gen = plan.blocks.find((b) => b.kind === 'generated')
  assert.ok(gen, 'paragraph with ins/del must produce a generated SaveBlock')
})

test('pmDocToSavePlan keeps ins/del inside the generated block (no marks dropped)', async () => {
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  const pmDoc = {
    type: 'doc',
    content: [paraWithInsDel()],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  const gen = plan.blocks.find((b) => b.kind === 'generated') as
    | { kind: 'generated'; block: { runs: Array<{ ins?: { author: string }; del?: { author: string } }> } }
    | undefined
  assert.ok(gen, 'generated block must exist')
  const insRun = gen.block.runs.find((r) => r.ins)
  const delRun = gen.block.runs.find((r) => r.del)
  assert.ok(insRun, 'ins run survives into the save plan')
  assert.ok(delRun, 'del run survives into the save plan')
  assert.equal(insRun?.ins?.author, 'alice')
  assert.equal(delRun?.del?.author, 'bob')
})


test('saveDocxBytes round-trip writes w:ins / w:del wrappers to word/document.xml', async () => {
  const { saveDocxBytes } = await import('../docxAdapter')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  const pmDoc = {
    type: 'doc',
    content: [paraWithInsDel()],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  const bytes = await saveDocxBytes(doc, plan.blocks as never)
  // Extract word/document.xml from the .docx (which is a zip).
  const { extractDocxDocumentXml } = await import('./_testZipExtract')
  const xml = extractDocxDocumentXml(bytes)
  assert.ok(/<w:ins\b/.test(xml), 'document.xml must contain <w:ins>')
  assert.ok(/<w:del\b/.test(xml), 'document.xml must contain <w:del>')
  assert.ok(/w:author="alice"/.test(xml), 'must carry author=alice on the ins')
  assert.ok(/w:author="bob"/.test(xml), 'must carry author=bob on the del')
  assert.ok(/w:date="2026-09-02T10:00:00\.000Z"/.test(xml), 'must carry the ins date')
  assert.ok(/<w:delText[^>]*>removed<\/w:delText>/.test(xml), 'deleted text uses w:delText')
})



test('del mark on text node round-trips through saveDocxBytes with w:delText', async () => {
  const { saveDocxBytes } = await import('../docxAdapter')
  const { extractDocxDocumentXml } = await import('./_testZipExtract')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  const pmDoc = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        attrs: { 'data-docx-index': 0 },
        content: [
          {
            type: 'text',
            text: 'kept',
          },
          {
            type: 'text',
            text: 'deleted_by_user',
            marks: [
              {
                type: 'del',
                attrs: { author: 'carol', date: '2026-09-02T11:00:00.000Z', id: '2002' },
              },
            ],
          },
        ],
      },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  const bytes = await saveDocxBytes(doc, plan.blocks as never)
  const xml = extractDocxDocumentXml(bytes)
  assert.ok(/<w:del\b/.test(xml), 'must contain <w:del>')
  assert.ok(/w:author="carol"/.test(xml), 'must carry author=carol on the del')
  assert.ok(/w:date="2026-09-02T11:00:00\.000Z"/.test(xml), 'must carry the del date')
  assert.ok(/<w:delText[^>]*>deleted_by_user<\/w:delText>/.test(xml), 'deleted text must use w:delText')
})

test('paragraph carrying both ins and del marks in different runs', async () => {
  const { saveDocxBytes } = await import('../docxAdapter')
  const { extractDocxDocumentXml } = await import('./_testZipExtract')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  const pmDoc = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        attrs: { 'data-docx-index': 0 },
        content: [
          {
            type: 'text',
            text: 'A',
            marks: [{ type: 'ins', attrs: { author: 'a', date: 'd1', id: 'i1' } }],
          },
          {
            type: 'text',
            text: 'B',
          },
          {
            type: 'text',
            text: 'C',
            marks: [{ type: 'del', attrs: { author: 'b', date: 'd2', id: 'd2' } }],
          },
        ],
      },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc as never, doc)
  const bytes = await saveDocxBytes(doc, plan.blocks as never)
  const xml = extractDocxDocumentXml(bytes)
  // Both wrappers present
  assert.ok(/<w:ins\b/.test(xml), 'must contain <w:ins>')
  assert.ok(/<w:del\b/.test(xml), 'must contain <w:del>')
  // Use w:delText for the deleted run, plain w:t for the inserted + kept runs.
  assert.ok(/<w:delText[^>]*>C<\/w:delText>/.test(xml), 'must emit <w:delText>C</w:delText>')
  assert.ok(/<w:t[^>]*>A<\/w:t>/.test(xml), 'must emit <w:t>A</w:t>')
  assert.ok(/<w:t[^>]*>B<\/w:t>/.test(xml), 'must emit <w:t>B</w:t>')
})
