// v0.7.56 — comment persistence: mark collection + comments.xml round-trip.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  openDocx,
  pmDocToSavePlan,
  saveDocxBytes,
  collectCommentIdsFromNode,
  pmNodeToGeneratedBlock,
} from '../docxAdapter'

test('collectCommentIdsFromNode merges comment mark ids across text runs', () => {
  const node = {
    type: 'paragraph',
    content: [
      { type: 'text', text: 'a', marks: [{ type: 'comment', attrs: { ids: '1' } }] },
      { type: 'text', text: 'b', marks: [{ type: 'comment', attrs: { ids: '2 1' } }] },
      { type: 'text', text: 'c' },
    ],
  }
  assert.deepEqual(collectCommentIdsFromNode(node as never), ['1', '2'])
})

test('collectCommentIdsFromNode returns undefined without comment marks', () => {
  const node = { type: 'paragraph', content: [{ type: 'text', text: 'plain' }] }
  assert.equal(collectCommentIdsFromNode(node as never), undefined)
})

test('pmNodeToGeneratedBlock carries commentStarts from marks', () => {
  const node = {
    type: 'paragraph',
    content: [
      { type: 'text', text: 'hi', marks: [{ type: 'comment', attrs: { ids: '7' } }] },
    ],
  }
  const block = pmNodeToGeneratedBlock(node as never)
  assert.deepEqual(block.commentStarts, ['7'])
})

test('saveDocxBytes writes comments.xml from options.comments', async () => {
  const seed = await (await import('../../engines/docx-engine/index')).buildBlankDocx()
  const doc = await openDocx(seed)
  const plan = pmDocToSavePlan(
    {
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          attrs: { 'data-docx-index': 0 },
          content: [
            { type: 'text', text: 'edited', marks: [{ type: 'comment', attrs: { ids: '1' } }] },
          ],
        },
      ],
    } as never,
    doc,
  )
  const bytes = await saveDocxBytes(doc, plan.blocks, {
    comments: [
      { id: '1', author: 'Alice', text: 'please fix', date: '2026-09-01T00:00:00Z' },
    ],
  })
  const zip = await JSZip.loadAsync(bytes)
  const commentsXml = await zip.file('word/comments.xml')?.async('string')
  assert.ok(commentsXml, 'comments.xml must exist')
  assert.match(commentsXml, /Alice/, 'author')
  assert.match(commentsXml, /please fix/, 'body')
  const documentXml = await zip.file('word/document.xml')?.async('string')
  assert.ok(documentXml, 'document.xml must exist')
  assert.match(documentXml, /<w:commentRangeStart w:id="1"\/>/, 'range start marker')
})
