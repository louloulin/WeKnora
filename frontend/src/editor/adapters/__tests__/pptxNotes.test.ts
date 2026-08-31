/**
 * v0.7.30 — PPT speaker notes smoke test.
 *
 * Verifies that setSlideNotesOnDeck:
 *   - writes text into the slide's notesSlide part,
 *   - updates the in-memory deck.slides[i].notes mirror,
 *   - invalid slide index returns false.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { newPptxShapeDeck, setSlideNotesOnDeck } from '../pptxShapeAdapter'

test('setSlideNotesOnDeck writes speaker notes and updates mirror', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened, 'blank deck must have an opened handle')
  const text = 'Hello\nLine two'
  const ok = setSlideNotesOnDeck(deck, 0, text)
  assert.equal(ok, true, 'setSlideNotesOnDeck should succeed')
  assert.equal(deck.slides[0].notes, text, 'deck.slides[0].notes must mirror the writes')
  const opened = deck.opened!
  const keys = Array.from(opened.archive.entries.keys())
  const hasNotesPart = keys.some((k) => /notesSlide\d+\.xml$/.test(k))
  assert.ok(hasNotesPart, 'package should contain a notesSlide part after setSlideNotes')
})

test('setSlideNotesOnDeck returns false for invalid slide index', async () => {
  const deck = await newPptxShapeDeck()
  const ok = setSlideNotesOnDeck(deck, 99, 'noop')
  assert.equal(ok, false, 'invalid index must fail gracefully')
})
