// v0.7.66 — DOC header/footer round-trip: options.header/footer → word/header1.xml.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  parseDocx,
  saveDocx,
  type SaveBlock,
} from '../../engines/docx-engine/index'

test('DOC header: options.header → word/header1.xml', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const blocks: SaveBlock[] = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, blocks, {
    header: { text: 'My Document Header' },
  })
  const zip = await JSZip.loadAsync(saved)
  const headerPaths = Object.keys(zip.files).filter((p) => /^word\/header\d+\.xml$/.test(p))
  assert.ok(headerPaths.length > 0, 'at least one header part created')
  const headerXml = await zip.file(headerPaths[0]!)?.async('string')
  assert.match(headerXml ?? '', /My Document Header/, 'header part contains text')
})

test('DOC footer: options.footer + pageNumber → PAGE field', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const blocks: SaveBlock[] = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, blocks, {
    footer: { text: 'Page ', pageNumber: true },
  })
  const zip = await JSZip.loadAsync(saved)
  const footerPaths = Object.keys(zip.files).filter((p) => /^word\/footer\d+\.xml$/.test(p))
  assert.ok(footerPaths.length > 0, 'at least one footer part created')
  const footerXml = await zip.file(footerPaths[0]!)?.async('string')
  assert.match(footerXml ?? '', /Page/, 'footer part contains text')
  assert.match(footerXml ?? '', /PAGE/, 'footer part contains PAGE field')
})

test('DOC hf: header + footer on same doc → 2 parts + sectPr references', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const blocks: SaveBlock[] = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, blocks, {
    header: { text: 'H' },
    footer: { text: 'F' },
  })
  const zip = await JSZip.loadAsync(saved)
  const docXml = await zip.file('word/document.xml')?.async('string')
  assert.match(docXml ?? '', /<w:headerReference[^>]+\/>/, 'document.xml has headerReference')
  assert.match(docXml ?? '', /<w:footerReference[^>]+\/>/, 'document.xml has footerReference')
})
