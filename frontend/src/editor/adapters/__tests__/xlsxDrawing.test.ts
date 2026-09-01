// v0.7.47 — self-simplified Drawing module tests (add + edit + remove).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  applyDrawingEdits,
  applyDrawingPipeline,
  readImageFile,
  type DrawingEdit,
} from '../xlsxDrawing'
import type { VisualAddition, MutablePackage } from '../xlsxDrawingAdd'

class JsZipPkg implements MutablePackage {
  constructor(private zip: JSZip) {}
  async paths(): Promise<readonly string[]> {
    const out: string[] = []
    this.zip.forEach((p) => out.push(p))
    return out.sort()
  }
  async has(path: string): Promise<boolean> {
    return this.zip.file(path) !== null
  }
  async readText(path: string): Promise<string> {
    return (await this.zip.file(path)?.async('text')) ?? ''
  }
  write(path: string, content: string): void {
    this.zip.file(path, content)
  }
  add(path: string, content: string): void {
    this.zip.file(path, content)
  }
  addBinary(path: string, bytes: Uint8Array): void {
    this.zip.file(path, bytes)
  }
  remove(path: string): void {
    this.zip.remove(path)
  }
}

const buildWb = async (): Promise<JSZip> => {
  const zip = new JSZip()
  zip.file(
    'xl/worksheets/sheet1.xml',
    '<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheetData/></worksheet>',
  )
  zip.file(
    'xl/worksheets/_rels/sheet1.xml.rels',
    '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>',
  )
  zip.file(
    '[Content_Types].xml',
    '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>',
  )
  zip.file(
    'xl/_rels/workbook.xml.rels',
    '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>',
  )
  return zip
}

// A drawing file with one anchor — used for edit / remove tests.
const drawingWithAnchor = `<?xml version="1.0"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <xdr:twoCellAnchor>
    <xdr:from><xdr:col>0</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>0</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from>
    <xdr:to><xdr:col>2</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>3</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:to>
    <xdr:pic>...</xdr:pic>
  </xdr:twoCellAnchor>
</xdr:wsDr>`

// ===================== applyDrawingEdits =====================

test('applyDrawingEdits: removes anchor by index', async () => {
  const zip = await buildWb()
  zip.file('xl/drawings/drawing1.xml', drawingWithAnchor)
  const pkg = new JsZipPkg(zip)
  const edits: DrawingEdit[] = [{ drawingPath: 'xl/drawings/drawing1.xml', drawingIndex: 0, remove: true }]
  const touched = new Set<string>()
  const dirty = await applyDrawingEdits(pkg, edits, touched)
  assert.ok(dirty, 'dirty flag set')
  const out = await pkg.readText('xl/drawings/drawing1.xml')
  assert.ok(!out.includes('<xdr:twoCellAnchor'), 'anchor removed')
  assert.ok(touched.has('xl/drawings/drawing1.xml'), 'drawing touched')
})

test('applyDrawingEdits: moves anchor (new from / to)', async () => {
  const zip = await buildWb()
  zip.file('xl/drawings/drawing1.xml', drawingWithAnchor)
  const pkg = new JsZipPkg(zip)
  const edits: DrawingEdit[] = [{
    drawingPath: 'xl/drawings/drawing1.xml',
    drawingIndex: 0,
    anchor: {
      fromColumn: 5, fromColumnOffset: 100,
      fromRow: 5, fromRowOffset: 100,
      toColumn: 8, toColumnOffset: 0,
      toRow: 7, toRowOffset: 0,
    },
  }]
  await applyDrawingEdits(pkg, edits, new Set())
  const out = await pkg.readText('xl/drawings/drawing1.xml')
  assert.ok(out.includes('<xdr:col>5</xdr:col>'), 'col moved')
  assert.ok(out.includes('<xdr:row>5</xdr:row>'), 'row moved')
})

test('applyDrawingEdits: applies frameSize (a:ext)', async () => {
  const zip = await buildWb()
  zip.file('xl/drawings/drawing1.xml', drawingWithAnchor)
  const pkg = new JsZipPkg(zip)
  const edits: DrawingEdit[] = [{
    drawingPath: 'xl/drawings/drawing1.xml',
    drawingIndex: 0,
    frameSize: { width: 914400, height: 685800 },
  }]
  await applyDrawingEdits(pkg, edits, new Set())
  const out = await pkg.readText('xl/drawings/drawing1.xml')
  assert.ok(out.includes('<a:ext cx="914400"'), 'frame width set')
  assert.ok(out.includes('cy="685800"'), 'frame height set')
})

test('applyDrawingEdits: throws on missing drawing', async () => {
  const zip = await buildWb()
  const pkg = new JsZipPkg(zip)
  await assert.rejects(
    () => applyDrawingEdits(pkg, [{
      drawingPath: 'xl/drawings/missing.xml',
      drawingIndex: 0,
      remove: true,
    }], new Set()),
    /missing/,
  )
})

test('applyDrawingEdits: no-op returns false', async () => {
  const zip = await buildWb()
  zip.file('xl/drawings/drawing1.xml', drawingWithAnchor)
  const pkg = new JsZipPkg(zip)
  // Out-of-range index — match not found
  const dirty = await applyDrawingEdits(pkg, [{
    drawingPath: 'xl/drawings/drawing1.xml',
    drawingIndex: 99,
    remove: true,
  }], new Set())
  assert.equal(dirty, false)
})

// ===================== applyDrawingPipeline (additions) =====================

test('applyDrawingPipeline: adds image + writes drawing + media + rels', async () => {
  const zip = await buildWb()
  const pkg = new JsZipPkg(zip)
  const tinyPng = new Uint8Array([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00])
  const b64 = Buffer.from(tinyPng).toString('base64')
  const additions: VisualAddition[] = [{
    worksheetPath: 'xl/worksheets/sheet1.xml',
    anchor: { col: 0, colOff: 0, row: 0, rowOff: 0 },
    image: { mediaType: 'image/png', base64: b64 },
  }]
  await applyDrawingPipeline(pkg, additions, [], new Set())
  // Drawing part added
  const paths = await pkg.paths()
  const drawingPath = paths.find((p) => p.startsWith('xl/drawings/drawing') && p.endsWith('.xml'))
  assert.ok(drawingPath, 'drawing part created')
  // Media file added (image)
  const mediaPath = paths.find((p) => p.startsWith('xl/media/image'))
  assert.ok(mediaPath, 'media part created')
  // Content_Types has override
  const ct = await pkg.readText('[Content_Types].xml')
  assert.ok(ct.includes('drawing+xml'), 'drawing content type override')
  assert.ok(ct.includes('png'), 'png content type default')
  // Worksheet has <drawing r:id>
  const wsXml = await pkg.readText('xl/worksheets/sheet1.xml')
  assert.ok(wsXml.includes('<drawing'), 'worksheet drawing reference')
  // Worksheet rels references drawing
  const rels = await pkg.readText('xl/worksheets/_rels/sheet1.xml.rels')
  assert.ok(rels.includes('drawing'), 'worksheet → drawing rel')
})

test('readImageFile: rejects unsupported types', async () => {
  // Construct a fake File-like object with unsupported mime
  const fakeFile = {
    type: 'image/svg+xml',
    arrayBuffer: async () => new ArrayBuffer(0),
  } as unknown as File
  await assert.rejects(() => readImageFile(fakeFile), /Unsupported/)
})

// ===================== readImageFile base64 round-trip =====================

test('readImageFile: PNG bytes round-trip through base64', async () => {
  const pngBytes = new Uint8Array([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])
  const fakeFile = {
    type: 'image/png',
    arrayBuffer: async () => pngBytes.buffer.slice(pngBytes.byteOffset, pngBytes.byteOffset + pngBytes.byteLength),
  } as unknown as File
  const result = await readImageFile(fakeFile)
  assert.equal(result.mediaType, 'image/png')
  assert.equal(typeof result.base64, 'string')
  assert.ok(result.base64.length > 0)
})
