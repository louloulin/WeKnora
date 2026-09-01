// v0.7.58 — Tencent Docs compatibility layer: CSV import/export.
// Vendored from genoffice csv-import.ts + csv-export.ts (pure parts).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  decodeCsvBuffer,
  sniffDelimiter,
  parseCsv,
  isNumericCell,
  buildWorksheetXml,
  csvToXlsxBuffer,
  blankXlsxBuffer,
  csvField,
  csvFromDisplayRows,
  gridToCsvBytes,
} from '../csvImport'

test('decodeCsvBuffer: BOM + UTF-8 + legacy charsets', () => {
  const utf8 = new TextEncoder().encode('名称,数量\n苹果,3\n')
  assert.equal(decodeCsvBuffer(utf8), '名称,数量\n苹果,3\n')

  const bom = new Uint8Array([0xef, 0xbb, 0xbf, ...utf8])
  assert.equal(decodeCsvBuffer(bom), '名称,数量\n苹果,3\n')

  // GBK bytes for 名称,数量 (Chinese Windows Excel export)
  const gbk = new Uint8Array([0xc3, 0xfb, 0xb3, 0xc6, 0x2c, 0xca, 0xfd, 0xc1, 0xbf, 0x0a])
  const decoded = decodeCsvBuffer(gbk, 'zh-CN')
  assert.equal(decoded, '名称,数量\n')
})

test('sniffDelimiter: comma / semicolon / tab', () => {
  assert.equal(sniffDelimiter('a,b,c\n1,2,3\n'), ',')
  assert.equal(sniffDelimiter('a;b;c\n1;2;3\n'), ';')
  assert.equal(sniffDelimiter('a\tb\tc\n1\t2\t3\n'), '\t')
  // quoted delimiter does not count
  assert.equal(sniffDelimiter('"a,b",c\n1,2\n'), ',')
})

test('parseCsv: quoting, CRLF, trailing empty row', () => {
  const rows = parseCsv('name,note,amount\r\n"Ada, Bob","says ""ok""","1,234.50"\r\n"multi\nline",,-3\r\n')
  assert.deepEqual(rows, [
    ['name', 'note', 'amount'],
    ['Ada, Bob', 'says "ok"', '1,234.50'],
    ['multi\nline', '', '-3'],
  ])
  assert.deepEqual(parseCsv('a,b\n'), [['a', 'b']])
})

test('isNumericCell: plain decimals only, leading zeros stay text', () => {
  assert.equal(isNumericCell('123'), true)
  assert.equal(isNumericCell('-3.5'), true)
  assert.equal(isNumericCell('1e3'), true)
  assert.equal(isNumericCell('007'), false)
  assert.equal(isNumericCell('1,234'), false)
  assert.equal(isNumericCell('abc'), false)
})

test('buildWorksheetXml: numeric vs inlineStr cells + dimension', () => {
  const xml = buildWorksheetXml([
    ['name', 'qty'],
    ['苹果', '3'],
  ])
  assert.match(xml, /<dimension ref="A1:B2"\/>/)
  assert.match(xml, /<c r="A2" t="inlineStr"><is><t xml:space="preserve">苹果<\/t><\/is><\/c>/)
  assert.match(xml, /<c r="B2"><v>3<\/v><\/c>/)
})

test('csvToXlsxBuffer: round-trip through JSZip', async () => {
  const bytes = await csvToXlsxBuffer('name,qty\n苹果,3\n')
  const zip = await JSZip.loadAsync(bytes)
  const sheet = await zip.file('xl/worksheets/sheet1.xml')?.async('string')
  assert.ok(sheet)
  assert.match(sheet ?? '', /苹果/)
  const wb = await zip.file('xl/workbook.xml')?.async('string')
  assert.match(wb ?? '', /<sheet name="Sheet1"/)
})

test('blankXlsxBuffer: valid empty workbook', async () => {
  const bytes = await blankXlsxBuffer()
  const zip = await JSZip.loadAsync(bytes)
  assert.ok(zip.file('xl/workbook.xml'))
  assert.ok(zip.file('xl/worksheets/sheet1.xml'))
})

test('csvField / csvFromDisplayRows: Excel-style quoting + CRLF', () => {
  assert.equal(csvField('plain'), 'plain')
  assert.equal(csvField('a,b'), '"a,b"')
  assert.equal(csvField('say "ok"'), '"say ""ok"""')
  assert.equal(csvField('multi\nline'), '"multi\nline"')
  assert.equal(csvFromDisplayRows([['a', 'b'], ['c', '']]), 'a,b\r\nc,\r\n')
  assert.equal(csvFromDisplayRows([]), '')
})

test('gridToCsvBytes: UTF-8 BOM + round-trip through parseCsv', () => {
  const rows = [
    ['name', 'note', 'amount'],
    ['Ada, Bob', 'says "ok"', '1,234.50'],
    ['multi\nline', '', '-3'],
  ]
  const bytes = gridToCsvBytes(rows)
  assert.equal(bytes[0], 0xef)
  assert.equal(bytes[1], 0xbb)
  assert.equal(bytes[2], 0xbf)
  const text = new TextDecoder('utf-8', { ignoreBOM: true }).decode(bytes)
  assert.equal(text[0], '\ufeff')
  assert.deepEqual(parseCsv(text.slice(1)), rows)
})
