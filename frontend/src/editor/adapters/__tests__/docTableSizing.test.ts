// v0.7.53 — DOC table column sizing (copy from genoffice table-sizing.ts).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { fitColumnWidths, MIN_TABLE_COLUMN_PX } from '../docTableSizing'
import { pmTableToTableXml } from '../docxAdapter'

test('fitColumnWidths keeps requested columns and shrinks untouched ones', () => {
  const current = [200, 200, 200]
  const requested = new Map<number, number>([[1, 300]])
  const out = fitColumnWidths(current, requested, 500)
  assert.equal(out.length, 3)
  assert.ok(Math.abs(out[1] - 300) < 0.01, 'requested column keeps its size')
  const total = out.reduce((s, w) => s + w, 0)
  assert.ok(total <= 500.01, 'total fits the content box')
})

test('fitColumnWidths clamps to the minimum column width when the box allows it', () => {
  const out = fitColumnWidths([100, 100], new Map(), 200)
  assert.ok(out.every((w) => w >= MIN_TABLE_COLUMN_PX - 0.01))
  // A box narrower than 2*min still fits: floor = min(min, limit/columns).
  const tight = fitColumnWidths([100, 100], new Map(), 50)
  assert.ok(tight.every((w) => w >= 25 - 0.01))
})

test('fitColumnWidths returns empty for an empty grid', () => {
  assert.deepEqual(fitColumnWidths([], new Map(), 500), [])
})

test('pmTableToTableXml honors TipTap colwidth attrs', () => {
  const table = {
    type: 'table',
    content: [
      {
        type: 'tableRow',
        content: [
          { type: 'tableHeader', attrs: { colwidth: [100] }, content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }] },
          { type: 'tableHeader', attrs: { colwidth: [300] }, content: [{ type: 'paragraph', content: [{ type: 'text', text: 'B' }] }] },
        ],
      },
      {
        type: 'tableRow',
        content: [
          { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: '1' }] }] },
          { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: '2' }] }] },
        ],
      },
    ],
  }
  const xml = pmTableToTableXml(table as never)
  assert.match(xml, /<w:gridCol w:w="1500"\/>/, '100px -> 1500 dxa')
  assert.match(xml, /<w:gridCol w:w="4500"\/>/, '300px -> 4500 dxa')
  assert.match(xml, /<w:tcW w:w="1500" w:type="dxa"\/>/, 'header cell width')
  assert.match(xml, /<w:tblW w:w="6000" w:type="dxa"\/>/, 'total width')
})

test('pmTableToTableXml falls back to 2000 dxa without colwidth', () => {
  const table = {
    type: 'table',
    content: [
      {
        type: 'tableRow',
        content: [
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }] },
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'B' }] }] },
        ],
      },
    ],
  }
  const xml = pmTableToTableXml(table as never)
  assert.match(xml, /<w:gridCol w:w="2000"\/>/)
  assert.match(xml, /<w:tblW w:w="4000" w:type="dxa"\/>/)
})
