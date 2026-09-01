// v0.7.64 — PPT slide transition round-trip: setSlideTransition → savePptx → getSlideTransition.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  openPptx,
  savePptx,
  setSlideTransition,
  getSlideTransition,
} from '../../engines/pptx-engine/index'

test('PPT transition round-trip: fade → save → reopen → getSlideTransition', async () => {
  const { createBlankPptx } = await import('../../engines/pptx-engine/blank')
  const bytes = await createBlankPptx()
  const opened = await openPptx(bytes)
  assert.equal(getSlideTransition(opened.deck.slides[0] as any), 'none')
  setSlideTransition(opened.deck.slides[0] as any, 'fade')
  assert.equal(getSlideTransition(opened.deck.slides[0] as any), 'fade')
  const saved = await savePptx(opened)
  const reopened = await openPptx(saved)
  assert.equal(getSlideTransition(reopened.deck.slides[0] as any), 'fade')
})

test('PPT transition: <p:fade/> element present in saved slide XML', async () => {
  const { createBlankPptx } = await import('../../engines/pptx-engine/blank')
  const bytes = await createBlankPptx()
  const opened = await openPptx(bytes)
  setSlideTransition(opened.deck.slides[0] as any, 'fade')
  const saved = await savePptx(opened)
  const zip = await JSZip.loadAsync(saved)
  const slidePaths: string[] = []
  zip.forEach((p) => { if (/ppt\/slides\/slide\d+\.xml$/.test(p)) slidePaths.push(p) })
  const xmls = await Promise.all(slidePaths.map((p) => zip.file(p)?.async('string')))
  const hasFade = xmls.some((x) => x && /<p:transition>/.test(x) && /<p:fade\/>/.test(x))
  assert.ok(hasFade, 'slide XML contains <p:transition><p:fade/></p:transition>')
})

test('PPT transition: clearing (none) removes the transition element', async () => {
  const { createBlankPptx } = await import('../../engines/pptx-engine/blank')
  const bytes = await createBlankPptx()
  const opened = await openPptx(bytes)
  setSlideTransition(opened.deck.slides[0] as any, 'wipe')
  assert.equal(getSlideTransition(opened.deck.slides[0] as any), 'wipe')
  setSlideTransition(opened.deck.slides[0] as any, 'none')
  assert.equal(getSlideTransition(opened.deck.slides[0] as any), 'none')
})
