// v0.7.50 — comment mark restoration: collectCommentIds merges run-level
// commentIds with cross-paragraph commentStarts/commentEnds.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { collectCommentIds } from '../docxAdapter'
import type { Block } from '../../engines/docx-engine'

const base: Block = {
  id: 'b1',
  type: 'paragraph',
  docxIndex: 0,
  originalXml: '<w:p/>',
}

test('collectCommentIds merges run-level commentIds', () => {
  const block: Block = {
    ...base,
    runs: [
      { text: 'a', commentIds: ['1'] },
      { text: 'b', commentIds: ['2', '1'] },
      { text: 'c' },
    ],
  }
  assert.deepEqual(collectCommentIds(block), ['1', '2'])
})

test('collectCommentIds includes cross-paragraph starts and ends', () => {
  const block: Block = {
    ...base,
    runs: [{ text: 'x', commentIds: ['3'] }],
    commentStarts: ['4'],
    commentEnds: ['5'],
  }
  assert.deepEqual(collectCommentIds(block), ['3', '4', '5'])
})

test('collectCommentIds returns undefined when no comments touch the block', () => {
  const block: Block = { ...base, runs: [{ text: 'plain' }] }
  assert.equal(collectCommentIds(block), undefined)
})

test('collectCommentIds reads textbox paragraphs too', () => {
  const block: Block = {
    ...base,
    textboxes: [
      {
        paras: [{ runs: [{ text: 'box', commentIds: ['7'] }] }],
      },
    ],
  }
  assert.deepEqual(collectCommentIds(block), ['7'])
})
