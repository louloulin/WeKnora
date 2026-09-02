/**
 * v0.7.107 — SLIDE <p:grpSp> persistence: groupElements() + savePptxShapeBytes()
 * should emit a real <p:grpSp> in the saved slide XML, with grpSpPr + children.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { inflateRawSync } from 'node:zlib'
import {
  newPptxShapeDeck,
  addShapeOnDeck,
  savePptxShapeBytes,
} from '../pptxShapeAdapter'
import { groupElements, ungroupElement } from '../../engines/pptx-engine/index'

/** Extract the first `ppt/slides/slide*.xml` entry from a .pptx (zip). */
function extractSlideXml(bytes: Uint8Array): string {
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  // Walk back from the end for EOCD signature 0x06054b50.
  let eocd = -1
  for (let i = bytes.length - 22; i >= 0 && i >= bytes.length - 65557; i--) {
    if (
      bytes[i] === 0x50 &&
      bytes[i + 1] === 0x4b &&
      bytes[i + 2] === 0x05 &&
      bytes[i + 3] === 0x06
    ) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('EOCD not found')
  const cdSize = dv.getUint32(eocd + 12, true)
  const cdOffset = dv.getUint32(eocd + 16, true)
  let p = cdOffset
  const end = cdOffset + cdSize
  while (p < end) {
    if (
      bytes[p] !== 0x50 ||
      bytes[p + 1] !== 0x4b ||
      bytes[p + 2] !== 0x01 ||
      bytes[p + 3] !== 0x02
    ) {
      throw new Error('bad CD entry')
    }
    const compMethod = dv.getUint16(p + 10, true)
    const compSize = dv.getUint32(p + 20, true)
    const fnameLen = dv.getUint16(p + 28, true)
    const extraLen = dv.getUint16(p + 30, true)
    const commentLen = dv.getUint16(p + 32, true)
    const lhOffset = dv.getUint32(p + 42, true)
    const fname = new TextDecoder().decode(bytes.subarray(p + 46, p + 46 + fnameLen))
    if (/^ppt\/slides\/slide\d+\.xml$/.test(fname)) {
      const lhFnameLen = dv.getUint16(lhOffset + 26, true)
      const lhExtraLen = dv.getUint16(lhOffset + 28, true)
      const dataStart = lhOffset + 30 + lhFnameLen + lhExtraLen
      const raw = bytes.subarray(dataStart, dataStart + compSize)
      if (compMethod === 0) return new TextDecoder('utf-8').decode(raw)
      if (compMethod === 8) return new TextDecoder('utf-8').decode(inflateRawSync(raw))
      throw new Error(`unsupported comp ${compMethod}`)
    }
    p += 46 + fnameLen + extraLen + commentLen
  }
  throw new Error('slide xml not found')
}

test('groupElements() + save emits <p:grpSp> with grpSpPr and 3 child <p:sp>', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened)
  const a = addShapeOnDeck(deck, 0, 'rect', { x: 914400, y: 914400, cx: 914400, cy: 914400 })
  const b = addShapeOnDeck(deck, 0, 'rect', { x: 1828800, y: 914400, cx: 914400, cy: 914400 })
  const c = addShapeOnDeck(deck, 0, 'rect', { x: 914400, y: 1828800, cx: 914400, cy: 914400 })
  assert.ok(a && b && c)
  const result = groupElements(deck.opened, 0, [a.id, b.id, c.id])
  assert.ok(result, 'groupElements must succeed')
  assert.ok(result.groupId, 'groupId must be non-empty')
  // Slide now has 1 element (the grpSp), not 3.
  assert.equal(deck.opened.deck.slides[0].elements.length, 1)
  // Save + extract slide XML.
  const bytes = await savePptxShapeBytes(deck)
  const xml = extractSlideXml(bytes)
  assert.ok(/<p:grpSp\b/.test(xml), 'slide XML must contain <p:grpSp>')
  assert.ok(/<p:grpSpPr\b/.test(xml), 'slide XML must contain <p:grpSpPr>')
  assert.ok(/<p:nvGrpSpPr\b/.test(xml), 'slide XML must contain <p:nvGrpSpPr>')
  // 3 child <p:sp> inside the grpSp
  const spMatches = xml.match(/<p:sp\b/g) ?? []
  assert.ok(spMatches.length >= 3, `expected >=3 <p:sp>, got ${spMatches.length}`)
  // The grpSp has a child <a:xfrm> with both off/ext and chOff/chExt
  assert.ok(/<a:off\b/.test(xml), 'grpSpPr must contain <a:off>')
  assert.ok(/<a:ext\b/.test(xml), 'grpSpPr must contain <a:ext>')
  assert.ok(/<a:chOff\b/.test(xml), 'grpSpPr must contain <a:chOff>')
  assert.ok(/<a:chExt\b/.test(xml), 'grpSpPr must contain <a:chExt>')
})

test('ungroupElement() after group lifts children back to slide top-level', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened)
  const a = addShapeOnDeck(deck, 0, 'rect', { x: 914400, y: 914400, cx: 914400, cy: 914400 })
  const b = addShapeOnDeck(deck, 0, 'ellipse', { x: 1828800, y: 914400, cx: 914400, cy: 914400 })
  const c = addShapeOnDeck(deck, 0, 'triangle', { x: 914400, y: 1828800, cx: 914400, cy: 914400 })
  assert.ok(a && b && c)
  const result = groupElements(deck.opened, 0, [a.id, b.id, c.id])
  assert.ok(result)
  assert.equal(deck.opened.deck.slides[0].elements.length, 1)
  // Now ungroup
  const lifted = ungroupElement(deck.opened, 0, result.groupId)
  assert.ok(lifted, 'ungroupElement must succeed')
  // Children are lifted; slide has 3 elements again (no grpSp).
  assert.equal(deck.opened.deck.slides[0].elements.length, 3)
  const types = deck.opened.deck.slides[0].elements.map((e) => e.type)
  assert.deepEqual(types.sort(), ['shape', 'shape', 'shape'])
})

test('save after ungroup emits 3 independent <p:sp> (no <p:grpSp>)', async () => {
  const deck = await newPptxShapeDeck()
  assert.ok(deck.opened)
  const a = addShapeOnDeck(deck, 0, 'rect', { x: 914400, y: 914400, cx: 914400, cy: 914400 })
  const b = addShapeOnDeck(deck, 0, 'ellipse', { x: 1828800, y: 914400, cx: 914400, cy: 914400 })
  const c = addShapeOnDeck(deck, 0, 'triangle', { x: 914400, y: 1828800, cx: 914400, cy: 914400 })
  assert.ok(a && b && c)
  const result = groupElements(deck.opened, 0, [a.id, b.id, c.id])
  assert.ok(result)
  const lifted = ungroupElement(deck.opened, 0, result.groupId)
  assert.ok(lifted)
  const bytes = await savePptxShapeBytes(deck)
  const xml = extractSlideXml(bytes)
  assert.ok(!/<p:grpSp\b/.test(xml), 'after ungroup the saved XML must NOT contain <p:grpSp>')
  const spMatches = xml.match(/<p:sp\b/g) ?? []
  assert.ok(spMatches.length >= 3, `expected >=3 <p:sp>, got ${spMatches.length}`)
})
