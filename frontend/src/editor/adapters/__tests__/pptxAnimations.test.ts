/**
 * v0.7.32 — PPT slide animations smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  addTableToSlide,
  getSlideAnimationsOnDeck,
  setSlideAnimationsOnDeck,
} from '../pptxShapeAdapter'

test('setSlideAnimationsOnDeck writes fade-in animations that round-trip', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened)
  const shape = addTableToSlide(deck, 0, 2, 2, {
    x: 914400, y: 914400, w: 914400 * 2, h: 914400 * 1.5,
  })
  assert.ok(shape)
  const ok = setSlideAnimationsOnDeck(deck, 0, [
    { spId: 2, effect: 'fade', trigger: 'onClick', durationMs: 800, delayMs: 0 },
    { spId: 2, effect: 'pulse', trigger: 'withPrevious', durationMs: 400, delayMs: 200 },
  ])
  assert.equal(ok, true)
  const got = getSlideAnimationsOnDeck(deck, 0)
  assert.equal(got.length, 2)
  assert.equal(got[0].effect, 'fade')
  assert.equal(got[1].effect, 'pulse')
})

test('setSlideAnimationsOnDeck returns false for invalid slide index', async () => {
  const deck = await newPptxShapeDeck()
  const ok = setSlideAnimationsOnDeck(deck, 99, [])
  assert.equal(ok, false)
})

test('getSlideAnimationsOnDeck returns [] for an empty slide', async () => {
  const deck = await newPptxShapeDeck()
  const got = getSlideAnimationsOnDeck(deck, 0)
  assert.deepEqual(got, [])
})
