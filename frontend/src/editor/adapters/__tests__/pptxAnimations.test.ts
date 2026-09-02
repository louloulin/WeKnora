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

// v0.7.112 — patch + reorder convenience helpers for per-animation edits.
test('patchSlideAnimationOnDeck mutates a single animation by index', async () => {
  const { patchSlideAnimationOnDeck } = await import('../pptxShapeAdapter')
  const deck = await newPptxShapeDeck()
  const shape = addTableToSlide(deck, 0, 2, 2, {
    x: 914400, y: 914400, w: 914400 * 2, h: 914400 * 1.5,
  })
  assert.ok(shape)
  setSlideAnimationsOnDeck(deck, 0, [
    { spId: 2, effect: 'fade', trigger: 'onClick', durationMs: 800, delayMs: 0 },
    { spId: 2, effect: 'pulse', trigger: 'withPrevious', durationMs: 400, delayMs: 200 },
  ])
  // patch #1: effect + durationMs + delayMs
  const ok = patchSlideAnimationOnDeck(deck, 0, 1, {
    effect: 'spin',
    durationMs: 1200,
    delayMs: 500,
  })
  assert.equal(ok, true)
  const got = getSlideAnimationsOnDeck(deck, 0)
  assert.equal(got.length, 2)
  assert.equal(got[0].effect, 'fade')
  assert.equal(got[0].durationMs, 800)
  assert.equal(got[1].effect, 'spin')
  assert.equal(got[1].durationMs, 1200)
  assert.equal(got[1].delayMs, 500)
})

test('patchSlideAnimationOnDeck rejects out-of-range index', async () => {
  const { patchSlideAnimationOnDeck } = await import('../pptxShapeAdapter')
  const deck = await newPptxShapeDeck()
  setSlideAnimationsOnDeck(deck, 0, [
    { spId: 2, effect: 'fade', trigger: 'onClick', durationMs: 800, delayMs: 0 },
  ])
  assert.equal(patchSlideAnimationOnDeck(deck, 0, 99, { effect: 'spin' }), false)
  assert.equal(patchSlideAnimationOnDeck(deck, 0, -1, { effect: 'spin' }), false)
})

test('reorderSlideAnimationOnDeck swaps neighbours up / down', async () => {
  const { reorderSlideAnimationOnDeck } = await import('../pptxShapeAdapter')
  const deck = await newPptxShapeDeck()
  setSlideAnimationsOnDeck(deck, 0, [
    { spId: 2, effect: 'fade',  trigger: 'onClick',       durationMs: 800, delayMs: 0 },
    { spId: 2, effect: 'pulse', trigger: 'withPrevious',  durationMs: 400, delayMs: 200 },
    { spId: 2, effect: 'spin',  trigger: 'afterPrevious', durationMs: 600, delayMs: 100 },
  ])
  // move 0 -> down (1)
  assert.equal(reorderSlideAnimationOnDeck(deck, 0, 0, 1), true)
  let got = getSlideAnimationsOnDeck(deck, 0)
  assert.deepEqual(got.map((a) => a.effect), ['pulse', 'fade', 'spin'])
  // move 2 -> up (-1)
  assert.equal(reorderSlideAnimationOnDeck(deck, 0, 2, -1), true)
  got = getSlideAnimationsOnDeck(deck, 0)
  assert.deepEqual(got.map((a) => a.effect), ['pulse', 'spin', 'fade'])
})

test('reorderSlideAnimationOnDeck rejects edges', async () => {
  const { reorderSlideAnimationOnDeck } = await import('../pptxShapeAdapter')
  const deck = await newPptxShapeDeck()
  setSlideAnimationsOnDeck(deck, 0, [
    { spId: 2, effect: 'fade', trigger: 'onClick', durationMs: 800, delayMs: 0 },
    { spId: 2, effect: 'pulse', trigger: 'withPrevious', durationMs: 400, delayMs: 200 },
  ])
  assert.equal(reorderSlideAnimationOnDeck(deck, 0, 0, -1), false)
  assert.equal(reorderSlideAnimationOnDeck(deck, 0, 1, 1), false)
})
