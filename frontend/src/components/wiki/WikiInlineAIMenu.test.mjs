// Unit test for WikiInlineAIMenu component logic.
//
// Mirrors the .vue file's visibility, selection-validation, and
// positioning math so we can run it under Node without the Vue runtime.
// Locks in:
//   - menu is hidden when no selection
//   - menu is hidden for whitespace-only or too-short selection
//   - menu shows when selection is inside the container
//   - selection text is truncated to MAX_SELECTION_LENGTH
//   - menuTop is offset upward (above the selection) by default
//   - menuLeft is horizontally centered on the selection
//   - menuLeft is clamped to viewport (no off-screen overflow)
//   - if above the viewport, menuTop falls back to below the selection

import assert from 'node:assert/strict'
import test from 'node:test'

const MIN_SELECTION_LENGTH = 1
const MAX_SELECTION_LENGTH = 4000
const MENU_HEIGHT_ESTIMATE = 36
const MENU_WIDTH_ESTIMATE = 360
const VIEWPORT_PADDING = 8

function shouldShow({ containerRef, selection, containerRect }) {
  if (!containerRef) return { show: false, reason: 'no-container' }
  if (!selection || selection.isCollapsed) return { show: false, reason: 'collapsed' }
  if (!containerRect.contains(selection.rangeRect)) return { show: false, reason: 'outside' }
  const text = selection.text.trim()
  if (text.length < MIN_SELECTION_LENGTH) return { show: false, reason: 'too-short' }
  return { show: true, text: text.slice(0, MAX_SELECTION_LENGTH) }
}

function computePosition({ selectionRect, viewportWidth, viewportHeight, scrollX, scrollY }) {
  let top = selectionRect.top - MENU_HEIGHT_ESTIMATE - 6
  if (top < VIEWPORT_PADDING) {
    top = selectionRect.bottom + 6
  }
  let left = selectionRect.left + selectionRect.width / 2 - MENU_WIDTH_ESTIMATE / 2
  if (left < VIEWPORT_PADDING) left = VIEWPORT_PADDING
  if (left + MENU_WIDTH_ESTIMATE > viewportWidth - VIEWPORT_PADDING) {
    left = viewportWidth - MENU_WIDTH_ESTIMATE - VIEWPORT_PADDING
  }
  return { top: top + scrollY, left: left + scrollX }
}

test('menu is hidden when there is no container ref', () => {
  const r = shouldShow({ containerRef: null, selection: { isCollapsed: false, text: 'hi', rangeRect: {} } })
  assert.equal(r.show, false)
})

test('menu is hidden when selection is collapsed', () => {
  const r = shouldShow({
    containerRef: {},
    selection: { isCollapsed: true, text: '', rangeRect: {} },
    containerRect: { contains: () => true },
  })
  assert.equal(r.show, false)
})

test('menu is hidden when selection is outside container', () => {
  const r = shouldShow({
    containerRef: {},
    selection: { isCollapsed: false, text: 'hi', rangeRect: { x: 0, y: 0 } },
    containerRect: { contains: () => false },
  })
  assert.equal(r.show, false)
})

test('menu is hidden for whitespace-only selection', () => {
  const r = shouldShow({
    containerRef: {},
    selection: { isCollapsed: false, text: '   \n\t  ', rangeRect: {} },
    containerRect: { contains: () => true },
  })
  assert.equal(r.show, false)
})

test('menu shows when valid selection is inside container', () => {
  const r = shouldShow({
    containerRef: {},
    selection: { isCollapsed: false, text: 'Hello world', rangeRect: {} },
    containerRect: { contains: () => true },
  })
  assert.equal(r.show, true)
  assert.equal(r.text, 'Hello world')
})

test('long selection is truncated to MAX_SELECTION_LENGTH', () => {
  const longText = 'a'.repeat(5000)
  const r = shouldShow({
    containerRef: {},
    selection: { isCollapsed: false, text: longText, rangeRect: {} },
    containerRect: { contains: () => true },
  })
  assert.equal(r.show, true)
  assert.equal(r.text.length, MAX_SELECTION_LENGTH)
})

test('default position is above the selection, centered', () => {
  const r = computePosition({
    selectionRect: { top: 200, bottom: 220, left: 100, width: 200 },
    viewportWidth: 1280,
    viewportHeight: 800,
    scrollX: 0,
    scrollY: 0,
  })
  // Top should be selectionRect.top - MENU_HEIGHT_ESTIMATE - 6 = 200 - 36 - 6 = 158
  assert.equal(r.top, 158)
  // Left should be 100 + 200/2 - 360/2 = 200 - 180 = 20
  assert.equal(r.left, 20)
})

test('position falls back below the selection when above is off-screen', () => {
  const r = computePosition({
    selectionRect: { top: 10, bottom: 30, left: 100, width: 200 },
    viewportWidth: 1280,
    viewportHeight: 800,
    scrollX: 0,
    scrollY: 0,
  })
  // Above would be -32 which is < VIEWPORT_PADDING, so use bottom + 6
  assert.equal(r.top, 36)
})

test('position is clamped on the left edge', () => {
  const r = computePosition({
    selectionRect: { top: 200, bottom: 220, left: 0, width: 50 },
    viewportWidth: 1280,
    viewportHeight: 800,
    scrollX: 0,
    scrollY: 0,
  })
  assert.equal(r.left, VIEWPORT_PADDING)
})

test('position is clamped on the right edge', () => {
  const r = computePosition({
    selectionRect: { top: 200, bottom: 220, left: 1200, width: 50 },
    viewportWidth: 1280,
    viewportHeight: 800,
    scrollX: 0,
    scrollY: 0,
  })
  assert.equal(r.left, 1280 - MENU_WIDTH_ESTIMATE - VIEWPORT_PADDING)
})

test('scroll offsets are added to final position', () => {
  const r = computePosition({
    selectionRect: { top: 200, bottom: 220, left: 100, width: 200 },
    viewportWidth: 1280,
    viewportHeight: 800,
    scrollX: 50,
    scrollY: 100,
  })
  assert.equal(r.top, 158 + 100)
  assert.equal(r.left, 20 + 50)
})
