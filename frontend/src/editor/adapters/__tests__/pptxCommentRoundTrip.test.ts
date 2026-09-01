// v0.7.61 — PPT comment round-trip: addSlideComment → savePptx → openPptx → getSlideComments.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { openPptx, savePptx, addSlideComment, getSlideComments } from '../../engines/pptx-engine/index'
import { createBlankPptx } from '../../engines/pptx-engine/blank'

test('PPT comment round-trip: add → save → reopen → read', async () => {
  const blank = await createBlankPptx()
  const opened = await openPptx(blank)
  const added = addSlideComment(opened, 0, { author: 'Ada', text: 'Slide 1 note' })
  assert.ok(added, 'addSlideComment returned a comment')
  assert.equal(added.author, 'Ada')
  assert.equal(added.text, 'Slide 1 note')
  const added2 = addSlideComment(opened, 0, { author: 'Bob', text: 'Second note' })
  assert.ok(added2)
  const saved = await savePptx(opened)
  const reopened = await openPptx(saved)
  const slide = reopened.deck.slides[0]
  assert.ok(slide)
  const comments = getSlideComments(reopened.archive, slide.path)
  assert.equal(comments.length, 2)
  const texts = comments.map((c) => c.text).sort()
  assert.deepEqual(texts, ['Second note', 'Slide 1 note'])
  const authors = comments.map((c) => c.author).sort()
  assert.deepEqual(authors, ['Ada', 'Bob'])
})

test('PPT comment: ensureAuthor idempotent (same author gets same authorId)', async () => {
  const blank = await createBlankPptx()
  const opened = await openPptx(blank)
  const a1 = addSlideComment(opened, 0, { author: 'Alice', text: 'first' })
  const a2 = addSlideComment(opened, 0, { author: 'Alice', text: 'second' })
  assert.ok(a1 && a2)
  assert.equal(a1.authorId, a2.authorId, 'same author → same authorId')
  const saved = await savePptx(opened)
  const reopened = await openPptx(saved)
  const slide = reopened.deck.slides[0]
  assert.ok(slide)
  const comments = getSlideComments(reopened.archive, slide.path)
  assert.equal(comments.length, 2)
  const ids = new Set(comments.map((c) => c.authorId))
  assert.equal(ids.size, 1, 'both comments share one authorId')
})
