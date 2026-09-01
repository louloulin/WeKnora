// v0.7.59 — DOC comment round-trip with replies (parentId) + resolved (done).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  parseDocx,
  saveDocx,
  buildBlankDocx,
  type CommentInfo,
} from '../../engines/docx-engine/index'

test('comments: reply + resolved round-trip through commentsExtended.xml', async () => {
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const comments: CommentInfo[] = [
    { id: '1', author: 'Ada', date: '2026-09-01T10:00:00Z', text: 'Top-level comment', paraId: '00000001' },
    { id: '2', author: 'Bob', date: '2026-09-01T10:01:00Z', text: 'Reply to Ada', parentId: '1', paraId: '00000002' },
    { id: '3', author: 'Carol', date: '2026-09-01T10:02:00Z', text: 'Another top-level', done: true, paraId: '00000003' },
  ]
  const finalBlocks = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, finalBlocks, { comments })
  const reopened = await parseDocx(saved)
  assert.equal(reopened.comments.length, 3)
  const byId = new Map(reopened.comments.map((c) => [c.id, c]))
  assert.equal(byId.get('1')?.text, 'Top-level comment')
  assert.equal(byId.get('2')?.parentId, '1')
  assert.equal(byId.get('2')?.text, 'Reply to Ada')
  assert.equal(byId.get('3')?.done, true)
  assert.equal(byId.get('3')?.text, 'Another top-level')
})

test('comments: paraId auto-assigned when omitted (new comments)', async () => {
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const comments: CommentInfo[] = [
    { id: '5', author: 'Ada', text: 'New without paraId', parentId: '4' },
    { id: '4', author: 'Ada', text: 'Parent without paraId' },
  ]
  const finalBlocks = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, finalBlocks, { comments })
  const reopened = await parseDocx(saved)
  // Both comments should be preserved with the reply link intact.
  const byId = new Map(reopened.comments.map((c) => [c.id, c]))
  assert.equal(byId.get('5')?.parentId, '4')
  assert.equal(byId.get('4')?.text, 'Parent without paraId')
  assert.equal(byId.get('5')?.text, 'New without paraId')
})
