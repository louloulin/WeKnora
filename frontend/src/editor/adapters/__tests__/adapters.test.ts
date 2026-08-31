/**
 * Adapter round-trip smoke tests. These verify that we can:
 *   1. Generate a real .docx/.pptx/.xlsx via the engine/adapter.
 *   2. Re-open it and recover the structured model.
 *   3. Mutate + save back to bytes and re-open again.
 *
 * Run with:  npm run test -- src/editor/adapters/__tests__
 *
 * These are integration tests over the genoffice engines we vendored
 * (docx-engine, pptxgenjs, xlsx). They do not require a running server.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { openDocx, saveDocxBytes, patchParagraphText } from '../docxAdapter'
import { openPptx, savePptxBytes, newPptxDeck } from '../pptxAdapter'
import { openXlsx, saveXlsxBytes, newXlsxWorkbook } from '../xlsxAdapter'

test('docx round-trip preserves paragraph text', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)
  patchParagraphText(doc, 0, 'hello round-trip')
  const out = await saveDocxBytes(doc)
  assert.ok(out.byteLength > 0, 'bytes must be non-empty')
  const doc2 = await openDocx(out)
  assert.equal(doc2.paragraphs[0].text.trim(), 'hello round-trip')
})

test('pptx round-trip produces a valid pptx blob', async () => {
  const deck = newPptxDeck()
  deck.slides = [
    { title: 'slide one', bullets: ['first', 'second'] },
    { title: 'slide two', bullets: ['alpha'] },
  ]
  const bytes = await savePptxBytes(deck)
  assert.ok(bytes.byteLength > 0, 'pptx bytes non-empty')
  const re = await openPptx(bytes)
  assert.ok(re.slides.length >= 2, 'should round-trip at least 2 slides')
  assert.equal(re.slides[0].title.trim(), 'slide one')
  assert.deepEqual(re.slides[0].bullets.map((s) => s.trim()), ['first', 'second'])
})

test('xlsx round-trip preserves cell values', async () => {
  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [{ v: 'a' }, { v: 1 }, { v: true }],
    [{ v: 'b' }, { v: 2 }, { v: null }],
  ]
  const bytes = await saveXlsxBytes(wb)
  assert.ok(bytes.byteLength > 0, 'xlsx bytes non-empty')
  const re = await openXlsx(bytes)
  assert.equal(String(re.sheets[0].rows[0][0].v), 'a')
  // SheetJS returns cell.v as string|number depending on cell type; we just
  // assert the value is non-empty / matches loosely.
  assert.ok(re.sheets[0].rows[0][1].v !== '')
  assert.equal(String(re.sheets[0].rows[1][0].v), 'b')
})

test('pmDocToSavePlan: text-only change emits xml kind, format change emits generated', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)

  // 1) text-only edit -> 'xml' kind, original block survives
  const textOnly: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        content: [{ type: 'text', text: 'Hello WeKnora' }],
      },
    ],
  }
  const plan1 = pmDocToSavePlan(textOnly, doc)
  assert.ok(plan1.blocks.length >= 1, 'must emit at least one block')
  assert.equal(plan1.blocks[0].kind, 'xml', 'text-only -> xml')
  assert.equal(plan1.textByIndex.get(0), 'Hello WeKnora')

  // 2) inline-format change (bold added) -> 'generated' kind
  const formatted: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        content: [
          { type: 'text', text: 'Hello', marks: [{ type: 'bold' }] },
          { type: 'text', text: ' WeKnora' },
        ],
      },
    ],
  }
  const plan2 = pmDocToSavePlan(formatted, doc)
  assert.equal(plan2.blocks[0].kind, 'generated', 'format change -> generated')

  // 3) untouched paragraph -> 'original' kind, no entry in textByIndex
  const untouched: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: doc.paragraphs[0].text }] },
    ],
  }
  const plan3 = pmDocToSavePlan(untouched, doc)
  assert.equal(plan3.blocks[0].kind, 'original', 'untouched -> original')
  assert.equal(plan3.textByIndex.size, 0, 'no text delta')
})
import { pmDocToSavePlan } from '../docxAdapter'

test('pmDocToSavePlan end-to-end: text-only edit round-trips through saveDocx', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const { saveDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)

  const pmDoc: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'Hello WeKnora' }] },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc, doc)
  assert.ok(plan.blocks.length >= 1)
  const bytes = await saveDocx(doc.parsed, plan.blocks, {})
  assert.ok(bytes.byteLength > 0)

  // Re-open and confirm the new text survived the round-trip.
  const re = await openDocx(bytes)
  assert.equal(re.paragraphs[0].text.trim(), 'Hello WeKnora')
})

test('pmDocToSavePlan end-to-end: bold edit round-trips through saveDocx', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const { saveDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)

  const pmDoc: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      {
        type: 'paragraph',
        content: [
          { type: 'text', text: 'Bold', marks: [{ type: 'bold' }] },
          { type: 'text', text: ' and ' },
          { type: 'text', text: 'plain', marks: [{ type: 'italic' }] },
        ],
      },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc, doc)
  assert.equal(plan.blocks[0].kind, 'generated', 'mixed marks -> generated')
  const bytes = await saveDocx(doc.parsed, plan.blocks, {})
  assert.ok(bytes.byteLength > 0)

  // Re-open and confirm runs preserved (text-only; engine re-emits as one
  // paragraph even when the source had mixed marks).
  const re = await openDocx(bytes)
  assert.match(re.paragraphs[0].text, /Bold/)
  assert.match(re.paragraphs[0].text, /plain/)
})

test('pptx-engine browser polyfills: sha256 + deflateRawSync + randomUUID', async () => {
  // Pure-TS SHA-256 polyfill matches Node's createHash byte-for-byte.
  const { sha256Hex, deflateRawSync, randomUUID } = await import(
    '../../engines/pptx-engine/polyfills'
  )
  // Vector from NIST FIPS 180-4 Appendix B: "abc" -> ba7816bf...
  assert.equal(
    sha256Hex(new TextEncoder().encode('abc')),
    'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    'SHA-256(abc) must match FIPS 180-4 vector',
  )
  // Empty input vector
  assert.equal(
    sha256Hex(new Uint8Array(0)),
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    'SHA-256(empty) must match FIPS 180-4 vector',
  )

  // deflateRawSync produces a valid deflate stream that pako can round-trip.
  const pako = await import('pako')
  const payload = new TextEncoder().encode('Hello WeKnora polyfill!')
  const compressed = deflateRawSync(payload)
  const decompressed = (pako.default ?? pako).inflateRaw(compressed)
  assert.equal(new TextDecoder().decode(decompressed), 'Hello WeKnora polyfill!')

  // randomUUID is a v4 UUID.
  const u = randomUUID()
  assert.match(u, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
})

test('pptx-engine: createBlankPptx round-trips through engine (no Node deps)', async () => {
  // Exercises the polyfilled zip.ts path: opens bytes -> enumerates parts.
  const { createBlankPptx } = await import('../../engines/pptx-engine/index')
  const blank = await createBlankPptx()
  assert.ok(blank instanceof Uint8Array)
  assert.ok(blank.byteLength > 0)
  // Re-open via JSZip to confirm it's a valid zip (no engine dependency).
  const JSZip = (await import('jszip')).default
  const zip = await JSZip.loadAsync(blank)
  const names = Object.keys(zip.files)
  assert.ok(names.some((n) => n.endsWith('presentation.xml')), 'has presentation.xml')
  assert.ok(names.some((n) => n.endsWith('slide1.xml')), 'has slide1.xml')
})

test('pptx-engine: PackageArchive.open uses polyfilled sha256', async () => {
  // End-to-end exercise of the polyfilled sha256 path: open the blank pptx
  // and confirm originalHash is a 64-char hex string.
  const { createBlankPptx } = await import('../../engines/pptx-engine/index')
  const { PackageArchive } = await import('../../engines/pptx-engine/zip')
  const bytes = await createBlankPptx()
  const archive = await PackageArchive.open(bytes)
  assert.equal(archive.originalHash.length, 64, 'sha256 hex should be 64 chars')
  assert.match(archive.originalHash, /^[0-9a-f]{64}$/, 'sha256 hex should be all lowercase hex')
  // Opening the same bytes a second time must yield the same hash.
  const archive2 = await PackageArchive.open(bytes)
  assert.equal(archive.originalHash, archive2.originalHash, 'hash is stable')
})

test('xlsx round-trip preserves formulas and number formats', async () => {
  const { openXlsx, saveXlsxBytes, newXlsxWorkbook } = await import('../xlsxAdapter')
  const XLSX = await import('xlsx')

  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [
      { v: 1 },
      { v: 2 },
      { v: 3 },
      { v: '=SUM(A1:C1)', f: 'SUM(A1:C1)' },
    ],
    [
      { v: 0.5, z: '0.00%' },
      { v: 0.25, z: '0.00%' },
      { v: '=A2+B2', f: 'A2+B2', z: '0.00%' },
      { v: '' },
    ],
  ]
  const bytes = await saveXlsxBytes(wb)
  assert.ok(bytes.byteLength > 0)

  // Re-open with SheetJS (formula-aware) and assert formula + format survive.
  const reXlsx = XLSX.read(bytes, { type: 'array', cellFormula: true, cellNF: true })
  const ws = reXlsx.Sheets[reXlsx.SheetNames[0]]
  const d1 = ws['D1'] as XLSX.CellObject | undefined
  assert.ok(d1, 'D1 should exist')
  assert.equal(String(d1.f).toUpperCase().replace(/\s+/g, ''), 'SUM(A1:C1)')
  const c2 = ws['C2'] as XLSX.CellObject | undefined
  assert.ok(c2, 'C2 should exist')
  assert.equal(String(c2.f).toUpperCase().replace(/\s+/g, ''), 'A2+B2')
  assert.match(String(c2.z), /%/)
})

test('xlsx round-trip via adapter surfaces formula + number format on cellExtras', async () => {
  const { newXlsxWorkbook, saveXlsxBytes, openXlsx } = await import('../xlsxAdapter')
  const wb = newXlsxWorkbook()
  wb.sheets[0].rows = [
    [{ v: 10 }, { v: 20 }, { v: '=A1+B1', f: 'A1+B1' }],
  ]
  const bytes = await saveXlsxBytes(wb)
  const re = await openXlsx(bytes)
  const row = re.sheets[0].rows[0]
  assert.equal(Number(row[0].v), 10, 'A1 round-trips')
  assert.equal(Number(row[1].v), 20, 'B1 round-trips')
  assert.equal(row[2].f, 'A1+B1', 'formula must round-trip through adapter')
})

test('pmTableToTableXml: minimal TipTap table -> valid OOXML w:tbl', async () => {
  const { pmTableToTableXml } = await import('../docxAdapter')
  const xml = pmTableToTableXml({
    type: 'table',
    content: [
      {
        type: 'tableRow',
        content: [
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'A' }] }] },
          { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'B' }] }] },
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
  })
  assert.match(xml, /^<w:tbl>/)
  assert.match(xml, /<\/w:tbl>$/, 'closes the table')
  assert.match(xml, /<w:tblGrid>/)
  assert.match(xml, /<w:tr>/)
  assert.match(xml, /<w:tc>/)
  assert.match(xml, />A</)
  assert.match(xml, />2</)
  assert.match(xml, /<w:tblBorders>/, 'includes borders')
})

test('pmImageToDrawingXml: minimal TipTap image -> valid OOXML w:drawing', async () => {
  const { pmImageToDrawingXml } = await import('../docxAdapter')
  const xml = pmImageToDrawingXml({
    type: 'image',
    attrs: { src: 'https://example.com/x.png', alt: 'demo', width: 200, height: 100 },
  })
  assert.match(xml, /^<w:p>/)
  assert.match(xml, /<w:drawing>/)
  assert.match(xml, /<wp:inline /)
  assert.match(xml, /<a:blip /)
  assert.match(xml, /<\/w:drawing>/)
})

test('pmDocToSavePlan end-to-end: new table round-trips through saveDocx', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/index')
  const { saveDocx } = await import('../../engines/docx-engine/index')
  const seed = await buildBlankDocx()
  const doc = await openDocx(seed)

  const pmDoc: import('../docxAdapter').PmNode = {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'Doc with table' }] },
      {
        type: 'table',
        content: [
          {
            type: 'tableRow',
            content: [
              { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Name' }] }] },
              { type: 'tableHeader', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Age' }] }] },
            ],
          },
          {
            type: 'tableRow',
            content: [
              { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Alice' }] }] },
              { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: '30' }] }] },
            ],
          },
        ],
      },
    ],
  }
  const plan = pmDocToSavePlan(pmDoc, doc)
  assert.ok(plan.blocks.length >= 2, 'must emit block for paragraph + table')
  const bytes = await saveDocx(doc.parsed, plan.blocks, {})
  assert.ok(bytes.byteLength > 0, 'bytes non-empty')
  // Re-open and confirm the paragraph text survives (table round-trip
  // through saveDocx requires more XML plumbing; we just verify the
  // pipeline does not throw).
  const re = await openDocx(bytes)
  assert.match(re.paragraphs[0].text, /Doc with table/)
})
