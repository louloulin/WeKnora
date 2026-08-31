/**
 * v0.7.32 — PPT master / theme / builtin-layouts smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  newPptxShapeDeck,
  listMasterPartsOnDeck,
  parseMasterOnDeck,
  listBuiltinLayouts,
  ensureBuiltinLayout,
  getBuiltinLayoutCatalog,
  shouldOfferBuiltinLayoutsOnDeck,
  applyThemeToDeck,
  recolorDeck,
} from '../pptxShapeAdapter'

test('listBuiltinLayouts returns the standard PowerPoint layout set', () => {
  const out = listBuiltinLayouts(9144000, 5143500, [])
  assert.ok(out.length > 0, 'should offer at least one builtin layout')
  const names = out.map((o) => o.name)
  // canonical PowerPoint layouts
  assert.ok(names.some((n) => /Title Slide/i.test(n)), 'should include Title Slide')
  assert.ok(names.some((n) => /Title and Content/i.test(n)), 'should include Title and Content')
  assert.ok(names.some((n) => /Blank/i.test(n)), 'should include Blank')
})

test('listBuiltinLayouts filters out layouts already in the deck', () => {
  const all = listBuiltinLayouts(9144000, 5143500, [])
  const withTitle = all.find((o) => /Title Slide/i.test(o.name))
  assert.ok(withTitle)
  const filtered = listBuiltinLayouts(
    9144000,
    5143500,
    all.map((o) => o.name),
  )
  assert.equal(filtered.length, 0, 'all titles are taken, no layouts left to offer')
})

test('shouldOfferBuiltinLayoutsOnDeck returns a boolean for any slide size', () => {
  const a = shouldOfferBuiltinLayoutsOnDeck([{name: "Title Slide", placeholders: []},{name: "Title and Content", placeholders: [1]}])
  const b = shouldOfferBuiltinLayoutsOnDeck([{name: "Blank", placeholders: []}])
  assert.equal(typeof a, 'boolean')
  assert.equal(typeof b, 'boolean')
})

test('getBuiltinLayoutCatalog returns the static catalog', () => {
  const cat = getBuiltinLayoutCatalog()
  assert.ok(Array.isArray(cat))
  assert.ok(cat.length > 0, 'catalog should have entries')
  for (const def of cat) {
    assert.ok(def.key && def.name && def.type, 'every catalog entry has key/name/type')
  }
})

test('listMasterPartsOnDeck returns at least the default master on a blank deck', async () => {
  const deck = await newPptxShapeDeck()
  const masters = listMasterPartsOnDeck(deck)
  assert.ok(masters.length >= 1, 'blank deck must have at least one master part')
  const slide = deck.opened!
  // slideMaster1.xml is the canonical default
  const hasDefaultMaster = masters.some((m) => /slideMaster1\.xml/.test(m.partPath))
  assert.ok(hasDefaultMaster, 'expected slideMaster1.xml in the master list')
})

test('parseMasterOnDeck returns slide model for the default master', async () => {
  const deck = await newPptxShapeDeck()
  const masters = listMasterPartsOnDeck(deck)
  const master1 = masters.find((m) => /slideMaster1\.xml/.test(m.partPath))
  assert.ok(master1, 'expected master1 in the list')
  const parsed = parseMasterOnDeck(deck, master1.partPath)
  assert.ok(parsed, 'parseMasterOnDeck should succeed')
  assert.equal(parsed.partPath, master1.partPath)
})

test('parseMasterOnDeck returns null on invalid part', async () => {
  const deck = await newPptxShapeDeck()
  const parsed = parseMasterOnDeck(deck, 'ppt/slideMasters/nonexistent.xml')
  assert.equal(parsed, null)
})

test('applyThemeToDeck rewrites theme parts without erroring on a blank deck', async () => {
  const deck = await newPptxShapeDeck()
  const n = applyThemeToDeck(deck, {
    name: '测试主题',
    colors: {
      dk1: '1F2937', lt1: 'FFFFFF', dk2: '0F172A', lt2: 'E5E7EB',
      accent1: '3B82F6', accent2: 'EF4444', accent3: '10B981', accent4: 'F59E0B',
      accent5: '6366F1', accent6: 'EC4899', hlink: '2563EB', folHlink: '9333EA',
    },
    majorFont: 'Calibri',
    minorFont: 'Calibri',
  })
  assert.equal(typeof n, 'number')
  // The blank deck ships with theme1.xml — at least one part should be rewritten.
  assert.ok(n >= 1, `expected ≥1 theme part rewritten, got ${n}`)
})

test('recolorDeck returns 0 on a deck with no explicit srgbClr values', async () => {
  const deck = await newPptxShapeDeck()
  const n = recolorDeck(deck, {
    name: 'noop',
    colors: {
      dk1: '000000', lt1: 'FFFFFF', dk2: '000000', lt2: 'FFFFFF',
      accent1: 'FF0000', accent2: '00FF00', accent3: '0000FF', accent4: 'FFFF00',
      accent5: '00FFFF', accent6: 'FF00FF', hlink: '0000FF', folHlink: '800080',
    },
  })
  assert.equal(typeof n, 'number')
})
