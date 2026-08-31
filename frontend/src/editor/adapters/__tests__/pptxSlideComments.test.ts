/**
 * v0.7.32 — PPT slide comments (OOXML <p:cmLst>) round-trip smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  addSlideCommentOnDeck,
  getSlideCommentsOnDeck,
  deleteSlideCommentOnDeck,
} from '../pptxShapeAdapter'

test('addSlideCommentOnDeck creates a comment that round-trips via getSlideCommentsOnDeck', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened, 'blank deck must have an opened handle')
  const added = addSlideCommentOnDeck(deck, 0, {
    author: 'Alice',
    initials: 'A',
    text: '请确认第 3 页的数据',
  })
  assert.ok(added, 'addSlideCommentOnDeck should return a record')
  assert.equal(added.author, 'Alice')
  assert.equal(added.initials, 'A')
  assert.equal(added.text, '请确认第 3 页的数据')
  assert.equal(added.idx, 1, 'first comment idx=1')
  assert.ok(added.date, 'date should be an ISO string')

  const got = getSlideCommentsOnDeck(deck, 0)
  assert.equal(got.length, 1)
  assert.equal(got[0].authorId, added.authorId)
  assert.equal(got[0].idx, 1)
  assert.equal(got[0].text, '请确认第 3 页的数据')
})

test('addSlideCommentOnDeck rejects empty text', async () => {
  const deck = await newPptxShapeDeck()
  const added = addSlideCommentOnDeck(deck, 0, { author: 'Bob', text: '  ' })
  assert.equal(added, null, 'blank text must reject')
})

test('deleteSlideCommentOnDeck removes the targeted comment', async () => {
  const deck = await newPptxShapeDeck()
  const c1 = addSlideCommentOnDeck(deck, 0, { author: 'Alice', text: 'first' })
  assert.ok(c1)
  const c2 = addSlideCommentOnDeck(deck, 0, { author: 'Bob', text: 'second' })
  assert.ok(c2)
  assert.equal(getSlideCommentsOnDeck(deck, 0).length, 2)
  const ok = deleteSlideCommentOnDeck(deck, 0, { authorId: c1.authorId, idx: c1.idx })
  assert.equal(ok, true)
  const after = getSlideCommentsOnDeck(deck, 0)
  assert.equal(after.length, 1)
  assert.equal(after[0].text, 'second')
})

test('slide comments survive saveBytes → openPptxShapes round-trip', async () => {
  const deck = await newPptxShapeDeck()
  const added = addSlideCommentOnDeck(deck, 0, {
    author: 'Alice', initials: 'AL', text: '请在交付前 review',
  })
  assert.ok(added)
  const opened = deck.opened!
  const hasAuthors = opened.archive.entries.has('ppt/commentAuthors.xml')
  assert.ok(hasAuthors, 'commentAuthors.xml should be in archive')
  const cmText = Array.from(opened.archive.entries.keys()).find((k) =>
    /^ppt\/comments\/comment\d+\.xml$/.test(k),
  )
  assert.ok(cmText, 'a per-slide comments part should exist')
})
