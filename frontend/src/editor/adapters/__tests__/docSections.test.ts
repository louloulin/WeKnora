// v0.7.72 — DOC multi-section helpers (readSections + ui-friendly wrappers).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { ParsedDoc, Block } from '../../engines/docx-engine/types'
import {
  getDocumentSections,
  findSectionOfBlock,
  isPortrait,
  paperLabel,
  fromTwips,
  toTwips,
  samePaper,
  sameMargins,
  formatSectionSummary,
  defaultSectionSettings,
  sectionCount,
} from '../docSections'

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

function mkBlock(docxIndex: number, originalXml: string | null = null): Block {
  return {
    id: `b-${docxIndex}`,
    type: 'paragraph',
    docxIndex,
    originalXml,
  }
}

function mkParsed(blocks: Block[]): ParsedDoc {
  return {
    blocks,
    comments: [],
    footnotes: [],
    endnotes: [],
    sources: [],
    inks: [],
    protection: null,
    writeProtection: null,
  }
}

// OOXML uses w:sectPr inside the trailing paragraph's w:pPr; readSections
// picks up sections by looking for paragraphs whose originalXml contains
// `<w:sectPr`. In real DOCX files the LAST body paragraph always carries the
// document-level sectPr, so to simulate multi-section documents we put
// sectPr on each "section-closing" paragraph.
const SECT_PR_LETTER =
  '<w:p><w:pPr><w:sectPr>' +
  '<w:pgSz w:w="12240" w:h="15840"/>' +
  '<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>' +
  '<w:cols w:space="720"/>' +
  '<w:docGrid w:linePitch="360"/>' +
  '</w:sectPr></w:pPr></w:p>'

const SECT_PR_A4_LANDSCAPE =
  '<w:p><w:pPr><w:sectPr>' +
  '<w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>' +
  '<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>' +
  '</w:sectPr></w:pPr></w:p>'

const SECT_PR_A4_PORTRAIT =
  '<w:p><w:pPr><w:sectPr>' +
  '<w:pgSz w:w="11906" w:h="16838"/>' +
  '<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>' +
  '</w:sectPr></w:pPr></w:p>'

const PLAIN_PARA = (text: string) =>
  `<w:p><w:r><w:t>${text}</w:t></w:r></w:p>`

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

test('sections: empty parsed → single default section', () => {
  const parsed = mkParsed([])
  const sections = getDocumentSections(parsed)
  assert.equal(sections.length, 1, 'no blocks → one synthetic default section')
  assert.equal(sections[0]!.index, 0)
  assert.equal(sections[0]!.firstBlockIndex, 0)
  assert.equal(sections[0]!.lastBlockIndex, 0)
})

test('sections: no sectPr → one section covering [0, n-1]', () => {
  const parsed = mkParsed([
    mkBlock(0, PLAIN_PARA('Hello')),
    mkBlock(1, PLAIN_PARA('World')),
  ])
  const sections = getDocumentSections(parsed)
  assert.equal(sections.length, 1)
  assert.equal(sections[0]!.firstBlockIndex, 0)
  assert.equal(sections[0]!.lastBlockIndex, 1)
})

test('sections: 2 sectPrs → 2 sections (range covers from 0 to each sectPr block)', () => {
  const parsed = mkParsed([
    mkBlock(0, PLAIN_PARA('A')),
    mkBlock(1, SECT_PR_LETTER),
    mkBlock(2, PLAIN_PARA('B')),
    mkBlock(3, SECT_PR_LETTER),
  ])
  const sections = getDocumentSections(parsed)
  assert.equal(sections.length, 2)
  assert.equal(sections[0]!.firstBlockIndex, 0)
  assert.equal(sections[0]!.lastBlockIndex, 1)
  assert.equal(sections[1]!.firstBlockIndex, 2)
  assert.equal(sections[1]!.lastBlockIndex, 3)
})

test('sections: 3 sectPrs → 3 sections', () => {
  const parsed = mkParsed([
    mkBlock(0, PLAIN_PARA('A')),
    mkBlock(1, SECT_PR_LETTER),
    mkBlock(2, PLAIN_PARA('B')),
    mkBlock(3, SECT_PR_LETTER),
    mkBlock(4, PLAIN_PARA('C')),
    mkBlock(5, SECT_PR_LETTER),
  ])
  const sections = getDocumentSections(parsed)
  assert.equal(sections.length, 3)
  assert.deepEqual(sections.map((s) => [s.firstBlockIndex, s.lastBlockIndex]), [
    [0, 1],
    [2, 3],
    [4, 5],
  ])
})

test('sections: landscape section exposes orientation = landscape', () => {
  const parsed = mkParsed([
    mkBlock(0, SECT_PR_A4_LANDSCAPE),
    mkBlock(1, PLAIN_PARA('B')),
    mkBlock(2, SECT_PR_A4_PORTRAIT),
  ])
  const [s0, s1] = getDocumentSections(parsed)
  assert.ok(s0, 'first section exists')
  assert.ok(s1, 'second section exists')
  assert.equal(s0.settings.orientation, 'landscape')
  assert.equal(s1.settings.orientation, 'portrait')
})

test('findSectionOfBlock: locate section for each block', () => {
  const parsed = mkParsed([
    mkBlock(0, PLAIN_PARA('A')),
    mkBlock(1, SECT_PR_LETTER),
    mkBlock(2, PLAIN_PARA('B')),
    mkBlock(3, SECT_PR_A4_LANDSCAPE),
    mkBlock(4, PLAIN_PARA('C')),
    mkBlock(5, SECT_PR_LETTER),
  ])
  const sections = getDocumentSections(parsed)
  assert.equal(sections.length, 3)
  assert.equal(findSectionOfBlock(sections, 0), 0)
  assert.equal(findSectionOfBlock(sections, 1), 0)
  assert.equal(findSectionOfBlock(sections, 2), 1)
  assert.equal(findSectionOfBlock(sections, 3), 1)
  assert.equal(findSectionOfBlock(sections, 4), 2)
  assert.equal(findSectionOfBlock(sections, 5), 2)
  // out-of-range lands on the last section
  assert.equal(findSectionOfBlock(sections, 9999), sections.length - 1)
})

test('findSectionOfBlock: empty sections → -1', () => {
  assert.equal(findSectionOfBlock([], 0), -1)
})

test('isPortrait: defaults to true when orientation = portrait', () => {
  const s = defaultSectionSettings()
  assert.equal(isPortrait(s), true)
  const land = { ...s, orientation: 'landscape' as const }
  assert.equal(isPortrait(land), false)
})

test('paperLabel: recognises Letter / A4 / A3 / Legal', () => {
  const s = defaultSectionSettings()
  assert.equal(paperLabel(s), 'US Letter')
  assert.equal(paperLabel({ ...s, pageWidth: 11906, pageHeight: 16838 }), 'A4')
  assert.equal(paperLabel({ ...s, pageWidth: 16838, pageHeight: 23811 }), 'A3')
  assert.equal(paperLabel({ ...s, pageWidth: 12240, pageHeight: 20160 }), 'US Legal')
})

test('paperLabel: custom dims reported in inches', () => {
  const s = { ...defaultSectionSettings(), pageWidth: 10000, pageHeight: 14000 }
  const label = paperLabel(s)
  assert.match(label, /in/, 'custom paper label includes unit')
})

test('fromTwips / toTwips: round-trip at multiple units', () => {
  assert.equal(fromTwips(1440, 'inches'), 1)
  assert.equal(fromTwips(1440, 'mm'), 25.4)
  assert.equal(fromTwips(720, 'inches'), 0.5)
  assert.equal(toTwips(1, 'inches'), 1440)
  assert.equal(toTwips(25.4, 'mm'), 1440)
  assert.equal(toTwips(720, 'twips'), 720)
  // bounds: negative input is clamped to 0
  assert.equal(toTwips(-2, 'inches'), 0)
})

test('samePaper / sameMargins: equality check', () => {
  const a = defaultSectionSettings()
  const b = defaultSectionSettings()
  assert.equal(samePaper(a, b), true)
  assert.equal(sameMargins(a, b), true)
  const c = { ...a, marginTop: 2000 }
  assert.equal(sameMargins(a, c), false, 'different top margin')
  const d = { ...a, orientation: 'landscape' as const }
  assert.equal(samePaper(a, d), false, 'different orientation')
})

test('formatSectionSummary: includes paper / orientation / column count', () => {
  const s = getDocumentSections(
    mkParsed([
      mkBlock(0, SECT_PR_A4_LANDSCAPE),
      mkBlock(1, PLAIN_PARA('B')),
      mkBlock(2, SECT_PR_A4_PORTRAIT),
    ]),
  )[0]!
  const label = formatSectionSummary(s)
  assert.match(label, /第1节/)
  assert.match(label, /A4/)
  assert.match(label, /横向/)
})

test('formatSectionSummary: multi-column section includes "栏"', () => {
  const s = getDocumentSections(
    mkParsed([
      mkBlock(0, SECT_PR_A4_LANDSCAPE),
      mkBlock(1, PLAIN_PARA('B')),
      mkBlock(2, SECT_PR_A4_PORTRAIT),
    ]),
  )[0]!
  const s2cols = { ...s, settings: { ...s.settings, columns: 3 } }
  assert.match(formatSectionSummary(s2cols), /3栏/)
})

test('sectionCount: matches getDocumentSections length', () => {
  const parsed = mkParsed([
    mkBlock(0, SECT_PR_LETTER),
    mkBlock(1, PLAIN_PARA('B')),
    mkBlock(2, SECT_PR_LETTER),
  ])
  assert.equal(sectionCount(parsed), getDocumentSections(parsed).length)
})
