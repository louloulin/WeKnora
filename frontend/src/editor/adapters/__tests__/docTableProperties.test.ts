// v0.7.54 — table property save path: fill / borders / repeatHeader.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { pmTableToTableXml } from '../docxAdapter'

const border = { style: 'single', szEighths: 4, color: '8EAADB' }

test('pmTableToTableXml emits cell fill and borders from preset attrs', () => {
  const table = {
    type: 'table',
    content: [
      {
        type: 'tableRow',
        content: [
          {
            type: 'tableHeader',
            attrs: { colwidth: [100], fill: 'D9E2F3', borders: { top: border, left: border, bottom: border, right: border } },
            content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }],
          },
        ],
      },
    ],
  }
  const xml = pmTableToTableXml(table as never)
  assert.match(xml, /<w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"\/>/, 'fill')
  assert.match(xml, /<w:tcBorders>/, 'borders wrapper')
  assert.match(xml, /<w:top w:val="single" w:sz="4" w:space="0" w:color="8EAADB"\/>/, 'top border')
  assert.match(xml, /<w:right w:val="single" w:sz="4" w:space="0" w:color="8EAADB"\/>/, 'right border')
})

test('pmTableToTableXml emits tblHeader for repeatHeader rows', () => {
  const table = {
    type: 'table',
    content: [
      {
        type: 'tableRow',
        attrs: { repeatHeader: true },
        content: [
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }] },
        ],
      },
      {
        type: 'tableRow',
        content: [
          { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: '1' }] }] },
        ],
      },
    ],
  }
  const xml = pmTableToTableXml(table as never)
  assert.match(xml, /<w:tr><w:trPr><w:tblHeader\/><\/w:trPr>/, 'first row repeats')
  const second = xml.split('</w:tr>')[1] ?? ''
  assert.doesNotMatch(second, /<w:tblHeader\/>/, 'second row does not repeat')
})

test('pmTableToTableXml omits shd/borders when attrs are absent', () => {
  const table = {
    type: 'table',
    content: [
      {
        type: 'tableRow',
        content: [
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }] },
        ],
      },
    ],
  }
  const xml = pmTableToTableXml(table as never)
  assert.doesNotMatch(xml, /<w:shd/, 'no fill')
  assert.doesNotMatch(xml, /<w:tcBorders>/, 'no borders')
  assert.doesNotMatch(xml, /<w:tblHeader\/>/, 'no repeat header')
})
