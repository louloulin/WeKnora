// v0.7.43.c — Tests for the vendored StylesheetEditor (cell color/font/fill
// persistence through xl/styles.xml).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { StylesheetEditor } from '../xlsxStyles'

const SEED_STYLES = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="2">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
  </fills>
  <borders count="1"><border/></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

const parseFonts = (xml: string): string[] => xml.match(/<font>[\s\S]*?<\/font>/g) ?? []
const parseFills = (xml: string): string[] => xml.match(/<fill>[\s\S]*?<\/fill>/g) ?? []
const parseNumFmts = (xml: string): string[] => xml.match(/<numFmt [^/]*\/>/g) ?? []
const parseCellXfs = (xml: string): string => xml.match(/<cellXfs[\s\S]*?<\/cellXfs>/)?.[0] ?? ''

test('StylesheetEditor: new instance preserves seed counts', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  assert.equal(ed.changed, false)
  const xfs = parseCellXfs(ed.serialize())
  assert.ok(xfs.includes('count="1"'), 'cellXfs count must stay 1')
})

test('StylesheetEditor: bold adds a font + a cellXfs entry', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx = ed.resolveStyle(0, { bold: true })
  assert.ok(idx > 0, 'must allocate a new cellXfs entry')
  assert.equal(ed.changed, true)
  const xml = ed.serialize()
  const fonts = parseFonts(xml)
  assert.equal(fonts.length, 2)
  assert.ok(fonts[1].includes('<b/>'))
  const xfs = parseCellXfs(xml)
  assert.ok(xfs.includes('fontId="1"'), 'new xf must reference the new font')
})

test('StylesheetEditor: fillColor adds a fill entry', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx = ed.resolveStyle(0, { fillColor: 'FFFFC7CE' })
  assert.ok(idx > 0)
  const xml = ed.serialize()
  const fills = parseFills(xml)
  assert.ok(fills.length > 2, `expected >2 fills, got ${fills.length}`)
  const newFill = fills[fills.length - 1]
  assert.ok(newFill.includes('solid'), `fill must be solid: ${newFill}`)
  assert.ok(newFill.includes('FFFFC7CE'))
})

test('StylesheetEditor: dedup — same delta + base returns same index', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx1 = ed.resolveStyle(0, { bold: true })
  const idx2 = ed.resolveStyle(0, { bold: true })
  assert.equal(idx1, idx2)
})

test('StylesheetEditor: fontColor adds a color tag', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx = ed.resolveStyle(0, { fontColor: 'FF9C0006' })
  assert.ok(idx > 0)
  const xml = ed.serialize()
  const fonts = parseFonts(xml)
  assert.ok(fonts[1].includes('FF9C0006'), `expected font color: ${fonts[1]}`)
})

test('StylesheetEditor: numberFormat builtin is referenced by id', () => {
  // 0.00% is builtin numFmtId 10, so the editor must reference id 10
  // directly instead of allocating a new <numFmt> entry.
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx = ed.resolveStyle(0, { numberFormat: '0.00%' })
  assert.ok(idx > 0)
  const xml = ed.serialize()
  assert.ok(parseNumFmts(xml).length === 0, 'no <numFmt> entry should be added for builtin 10')
  assert.ok(parseCellXfs(xml).includes('numFmtId="10"'), 'xf must reference builtin 10')
})

test('StylesheetEditor: custom numberFormat allocates a numFmt entry', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  const idx = ed.resolveStyle(0, { numberFormat: '#,##0.00 元' })
  assert.ok(idx > 0)
  const xml = ed.serialize()
  const numFmts = parseNumFmts(xml)
  assert.ok(numFmts.length >= 1)
  assert.ok(numFmts.some((f) => f.includes('#,##0.00 元')))
})

test('StylesheetEditor: serialize round-trip', () => {
  const ed = new StylesheetEditor(SEED_STYLES)
  ed.resolveStyle(0, { bold: true, fillColor: 'FFFFFF00' })
  const xml = ed.serialize()
  assert.ok(xml.includes('<?xml'))
  assert.ok(xml.includes('fonts count="2"'))
  assert.ok(xml.includes('cellXfs count="2"'))
  // Re-instantiate from serialized XML — counts must be preserved.
  const ed2 = new StylesheetEditor(xml)
  const xml2 = ed2.serialize()
  assert.ok(xml2.includes('fonts count="2"'))
  assert.ok(xml2.includes('cellXfs count="2"'))
})

test('StylesheetEditor: reject malformed stylesheet', () => {
  assert.throws(() => new StylesheetEditor('<styleSheet/>'))
})
