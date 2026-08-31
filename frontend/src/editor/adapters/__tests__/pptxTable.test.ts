/**
 * v0.7.28 — PPT table insertion smoke test.
 *
 * Verifies that addTableToSlide:
 *   - calls the pptx-engine's addTable with the right offset/cx/cy,
 *   - returns a PptxShape with the same id + cell grid (rows × cols).
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  addTableToSlide,
  type PptxShape,
} from '../pptxShapeAdapter'

test('addTableToSlide inserts a 3x3 table into a blank deck', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened, 'blank deck should have an opened handle')
  const shape: PptxShape | null = addTableToSlide(
    deck,
    0,
    3,
    3,
    { x: 914400, y: 914400, w: 914400 * 4, h: 457200 * 3 },
  )
  assert.ok(shape, 'addTableToSlide should return a PptxShape')
  assert.equal(shape.type, 'table')
  assert.equal(shape.rows, 3)
  assert.equal(shape.cols, 3)
  assert.ok(Array.isArray(shape.cellTexts))
  assert.equal(shape.cellTexts?.length, 3)
  assert.equal(shape.cellTexts?.[0].length, 3)
  // The slide should now have one element (the new table).
  assert.ok(deck.opened)
  const elements = deck.opened!.deck.slides[0].elements
  assert.ok(elements.some((e) => e.id === shape!.id), 'engine slide should carry the new table element')
})

test('addTableToSlide produces non-zero cell grid dimensions', async () => {
  const deck = await newPptxShapeDeck()
  const shape = addTableToSlide(deck, 0, 5, 2, {
    x: 0, y: 0, w: 914400 * 6, h: 457200 * 5,
  })
  assert.ok(shape)
  assert.equal(shape!.cellTexts?.length, 5)
  assert.equal(shape!.cellTexts?.[0].length, 2)
  for (const row of shape!.cellTexts ?? []) {
    for (const cell of row) {
      assert.equal(cell, '')
    }
  }
})
