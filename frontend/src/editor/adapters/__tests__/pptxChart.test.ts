/**
 * v0.7.32 — PPT chart insertion smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  addChartToSlide,
} from '../pptxShapeAdapter'

test('addChartToSlide inserts a bar chart into a blank deck', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened, 'blank deck must have an opened handle')
  const id = addChartToSlide(deck, 0, {
    kind: 'bar',
    title: '示例柱状图',
    categories: ['一月', '二月', '三月', '四月'],
    series: [
      { name: '本部门', values: [12, 18, 9, 22] },
      { name: '行业均值', values: [10, 14, 11, 17] },
    ],
    offset: { x: 914400, y: 914400, cx: 914400 * 5, cy: 914400 * 3 },
  })
  assert.ok(id, 'addChartToSlide should return the new chart elementId')
  const keys = Array.from(deck.opened!.archive.entries.keys())
  const hasChartPart = keys.some((k) => /ppt\/charts\/chart\d+\.xml$/.test(k))
  assert.ok(hasChartPart, 'archive should contain a chart part')
  const slide = deck.opened!.deck.slides[0]
  const lastEl = slide.elements[slide.elements.length - 1]
  assert.ok(lastEl, 'slide should have a new element appended')
  const mirrored = deck.slides[0].shapes.find((s) => s.id === id)
  assert.ok(mirrored, 'PptxShapeSlide should mirror the new chart shape')
  assert.equal(mirrored!.type, 'rect')
  assert.equal(mirrored!.preset, 'chart')
})

test('addChartToSlide rejects empty categories', async () => {
  const deck = await newPptxShapeDeck()
  const id = addChartToSlide(deck, 0, {
    kind: 'pie',
    categories: [],
    series: [{ name: 'x', values: [1] }],
    offset: { x: 0, y: 0, cx: 914400, cy: 914400 },
  })
  assert.equal(id, null)
})

test('addChartToSlide supports line + pie + doughnut chart kinds', async () => {
  const deck = await newPptxShapeDeck()
  const line = addChartToSlide(deck, 0, {
    kind: 'line',
    categories: ['Jan', 'Feb', 'Mar'],
    series: [{ name: 'trend', values: [5, 7, 6] }],
    offset: { x: 914400, y: 914400, cx: 914400 * 4, cy: 914400 * 2 },
  })
  assert.ok(line)
  const pie = addChartToSlide(deck, 0, {
    kind: 'pie',
    categories: ['Apple', 'Banana', 'Cherry'],
    series: [{ name: 'share', values: [40, 35, 25] }],
    offset: { x: 914400, y: 914400 * 3, cx: 914400 * 3, cy: 914400 * 3 },
  })
  assert.ok(pie)
  const doughnut = addChartToSlide(deck, 0, {
    kind: 'doughnut',
    categories: ['2023', '2024', '2025'],
    series: [{ name: 'arr', values: [100, 130, 200] }],
    offset: { x: 914400, y: 914400 * 6, cx: 914400 * 3, cy: 914400 * 3 },
  })
  assert.ok(doughnut)
  const keys = Array.from(deck.opened!.archive.entries.keys())
  const chartParts = keys.filter((k) => /ppt\/charts\/chart\d+\.xml$/.test(k))
  assert.ok(chartParts.length >= 3, `expected ≥3 chart parts, got ${chartParts.length}`)
})
