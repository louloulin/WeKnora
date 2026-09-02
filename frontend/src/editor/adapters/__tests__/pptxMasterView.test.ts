/**
 * pptxMasterView.test.ts — v0.7.113 PPT 母版视图 (genoffice vendor)
 *
 * Tests for the four helpers that back the slide master/layout list view:
 *   - parseMasterToSlideOnDeck
 *   - readMasterPartXmlOnDeck
 *   - writeMasterPartXmlOnDeck
 *   - renameMasterOnDeck
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  openPptxShapes,
  savePptxShapeBytes,
  listMasterPartsOnDeck,
  parseMasterToSlideOnDeck,
  readMasterPartXmlOnDeck,
  writeMasterPartXmlOnDeck,
  renameMasterOnDeck,
} from '../pptxShapeAdapter'

const Buffer = (await import('node:buffer')).Buffer

test('parseMasterToSlideOnDeck returns a parsed slide model for the master', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  assert.ok(parts.length >= 1, 'a fresh deck must have at least one master')
  const master = parts.find((p) => p.kind === 'master')!
  const slide = parseMasterToSlideOnDeck(deck, master.partPath)
  assert.ok(slide, 'parsed slide is non-null')
  assert.ok(slide!.elements.length >= 0, 'elements array exists')
})

test('readMasterPartXmlOnDeck returns the raw cSld XML', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  const master = parts[0]!
  const xml = readMasterPartXmlOnDeck(deck, master.partPath)
  assert.ok(xml, 'xml returned')
  assert.match(xml!, /<p:cSld\b/, 'xml contains cSld element')
})

test('writeMasterPartXmlOnDeck mutates the archive and readback sees the change', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  const master = parts[0]!
  const before = readMasterPartXmlOnDeck(deck, master.partPath)!
  // Append a marker comment to identify our patch
  const marker = '<!-- pptxMasterView.test.ts writeProbe -->'
  const next = before.replace(/(<p:cSld\b[^>]*>)/, '$1' + marker)
  assert.ok(writeMasterPartXmlOnDeck(deck, master.partPath, next))
  const after = readMasterPartXmlOnDeck(deck, master.partPath)!
  assert.ok(after.includes(marker), 'after write, marker is present')
})

test('renameMasterOnDeck adds a name attr when missing', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  const master = parts[0]!
  const before = readMasterPartXmlOnDeck(deck, master.partPath)!
  const stripped = before.replace(/<p:cSld\b[^>]*\sname="[^"]*"([^>]*)>/, (_m, tail) => `<p:cSld${tail}>`)
  if (stripped !== before) {
    writeMasterPartXmlOnDeck(deck, master.partPath, stripped)
  }
  assert.ok(renameMasterOnDeck(deck, master.partPath, 'Custom Test Master'))
  const after = readMasterPartXmlOnDeck(deck, master.partPath)!
  assert.match(after, /<p:cSld\b[^>]*name="Custom Test Master"/, 'name attr inserted')
})

test('renameMasterOnDeck replaces existing name attr', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  const master = parts[0]!
  // Set initial name, then update.
  renameMasterOnDeck(deck, master.partPath, 'Initial')
  const xml1 = readMasterPartXmlOnDeck(deck, master.partPath)!
  assert.match(xml1, /name="Initial"/, 'initial name set')
  renameMasterOnDeck(deck, master.partPath, 'Updated')
  const xml2 = readMasterPartXmlOnDeck(deck, master.partPath)!
  assert.match(xml2, /name="Updated"/, 'updated name visible')
  assert.ok(!/name="Initial"/.test(xml2), 'old name removed')
})

test('renameMasterOnDeck survives round-trip savePptx → openPptx', async () => {
  const deck = await newPptxShapeDeck()
  const parts = listMasterPartsOnDeck(deck)
  const master = parts[0]!
  renameMasterOnDeck(deck, master.partPath, 'RoundTrip Master')
  const bytes = await savePptxShapeBytes(deck)
  const reopened = await openPptxShapes(bytes)
  const reopenMaster = listMasterPartsOnDeck(reopened)[0]!
  assert.equal(reopenMaster.name, 'RoundTrip Master', 'name persisted through save/reopen')
})
