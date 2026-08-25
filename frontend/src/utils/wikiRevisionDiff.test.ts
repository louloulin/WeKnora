import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DIFF_LONG_LINE_THRESHOLD,
  DIFF_MAX_LINES,
  diffLines,
  diffLinesSafe,
  diffSideBySideHtml,
  diffUnifiedHtml,
  diffWikiRevision,
  type WikiRevisionSnapshot,
  summarizeDiff,
  truncateLine,
} from './wikiRevisionDiff.ts'

test('truncateLine returns short lines unchanged', () => {
  const line = 'short line of text'
  assert.equal(truncateLine(line), line)
})

test('truncateLine clips long lines to head + ellipsis + tail', () => {
  const long = 'a'.repeat(DIFF_LONG_LINE_THRESHOLD + 100)
  const out = truncateLine(long)
  assert.ok(out.length < long.length)
  assert.ok(out.includes('…'))
  assert.ok(out.startsWith('a'.repeat(80)))
  assert.ok(out.endsWith('a'.repeat(40)))
})

test('diffLines returns empty array for two empty inputs', () => {
  assert.deepEqual(diffLines('', ''), [])
})

test('diffLines on identical inputs produces only eq segments', () => {
  const segs = diffLines('a\nb\nc', 'a\nb\nc')
  assert.equal(segs.length, 1)
  assert.equal(segs[0].kind, 'eq')
  assert.deepEqual(segs[0].newLines, ['a', 'b', 'c'])
})

test('diffLines on completely different inputs produces one del + one add', () => {
  const segs = diffLines('a\nb', 'c\nd')
  // The LCS is empty; backtrack produces one del-segment (lines from old)
  // followed by one add-segment (lines from new).
  assert.ok(segs.length >= 2)
  const kinds = segs.map((s) => s.kind)
  assert.ok(kinds.includes('del'))
  assert.ok(kinds.includes('add'))
  const delSeg = segs.find((s) => s.kind === 'del')!
  assert.deepEqual(delSeg.oldLines, ['a', 'b'])
  const addSeg = segs.find((s) => s.kind === 'add')!
  assert.deepEqual(addSeg.newLines, ['c', 'd'])
})

test('diffLines preserves common prefix and suffix as eq', () => {
  const segs = diffLines('a\nb\nc\nd', 'a\nX\nc\nd')
  const eqSegs = segs.filter((s) => s.kind === 'eq')
  // The shared prefix "a" + middle + shared suffix "c\nd".
  // We expect at least one eq segment covering "a" and one covering "c\nd".
  const allEqLines = eqSegs.flatMap((s) => s.newLines ?? [])
  assert.ok(allEqLines.includes('a'))
  assert.ok(allEqLines.includes('c'))
  assert.ok(allEqLines.includes('d'))
})

test('diffLines reports multi-segment changes', () => {
  const segs = diffLines('a\nb\nc\nd\ne', 'a\nX\nc\nY\ne')
  // LCS = a,c,e (length 3). Backtrack from (5,5) yields:
  //   eq(a), add(X), del(b), eq(c), add(Y), del(d), eq(e) — coalesced where adjacent
  //   kinds match. We expect exactly:
  //   ['eq','add','del','eq','add','del','eq']
  const kinds = segs.map((s) => s.kind)
  assert.deepEqual(kinds, ['eq', 'add', 'del', 'eq', 'add', 'del', 'eq'])
})

test('diffLines handles empty old input', () => {
  const segs = diffLines('', 'a\nb')
  assert.equal(segs.length, 1)
  assert.equal(segs[0].kind, 'add')
  assert.deepEqual(segs[0].newLines, ['a', 'b'])
})

test('diffLines handles empty new input', () => {
  const segs = diffLines('a\nb', '')
  assert.equal(segs.length, 1)
  assert.equal(segs[0].kind, 'del')
  assert.deepEqual(segs[0].oldLines, ['a', 'b'])
})

test('diffLines normalizes \\r\\n line endings', () => {
  const segs = diffLines('a\r\nb\r\nc', 'a\nb\nc')
  assert.equal(segs.length, 1)
  assert.equal(segs[0].kind, 'eq')
})

test('summarizeDiff counts add / del / unchanged lines correctly', () => {
  const segs = diffLines('a\nb\nc', 'a\nX\nc\nd')
  const summary = summarizeDiff(segs)
  assert.equal(summary.unchanged, 2) // 'a', 'c'
  assert.equal(summary.deleted, 1) // 'b'
  assert.equal(summary.added, 2) // 'X', 'd'
})

test('diffSideBySideHtml emits one row per line with correct classes', () => {
  const segs = diffLines('a', 'a\nb')
  const html = diffSideBySideHtml(segs)
  assert.ok(html.includes('wiki-diff-side-by-side'))
  // 'a' is shared → eq row.
  assert.ok(html.includes('wiki-diff-eq'))
  // 'b' is added → add row with empty left cell.
  assert.ok(html.includes('wiki-diff-add'))
  assert.ok(html.includes('<ins>'))
})

test('diffSideBySideHtml escapes HTML in lines', () => {
  const segs = diffLines('<script>alert(1)</script>', '')
  const html = diffSideBySideHtml(segs)
  assert.ok(!html.includes('<script>'))
  assert.ok(html.includes('&lt;script&gt;'))
  assert.ok(html.includes('<del>'))
})

test('diffUnifiedHtml uses diff -u style prefix (+, -, space)', () => {
  const segs = diffLines('a\nb', 'a\nc')
  const html = diffUnifiedHtml(segs)
  assert.ok(html.includes('wiki-diff-unified'))
  // 'a' is shared, prefixed with space.
  assert.ok(html.match(/wiki-diff-eq[^<]* a/))
  // 'b' is deleted.
  assert.ok(html.includes('<del>- b</del>'))
  // 'c' is added.
  assert.ok(html.includes('<ins>+ c</ins>'))
})

test('diffLinesSafe returns truncated marker when inputs exceed the line cap', () => {
  const big = (i: number) => `line ${i}`.repeat(1).concat('\n').repeat(DIFF_MAX_LINES + 100)
  const oldText = Array.from({ length: DIFF_MAX_LINES + 50 }, (_, i) => `old ${i}`).join('\n')
  const newText = Array.from({ length: DIFF_MAX_LINES + 50 }, (_, i) => `new ${i}`).join('\n')
  const { segments, truncated } = diffLinesSafe(oldText, newText)
  assert.equal(truncated, true)
  assert.equal(segments.length, 0)
  // ensure we did not accidentally invoke `big` (touch guard)
  void big
})

test('diffLinesSafe falls through to diffLines when inputs are within cap', () => {
  const segs = diffLinesSafe('a\nb', 'a\nc')
  assert.equal(segs.truncated, false)
  assert.ok(segs.segments.length > 0)
})

test('diffWikiRevision returns three sections in title/summary/content order', () => {
  const from: WikiRevisionSnapshot = { title: 't1', summary: 's1', content: 'c1' }
  const to: WikiRevisionSnapshot = { title: 't2', summary: 's1', content: 'c2' }
  const sections = diffWikiRevision(from, to)
  assert.deepEqual(
    sections.map((s) => s.field),
    ['title', 'summary', 'content'],
  )
})

test('diffWikiRevision shows changed title and content, no change for summary', () => {
  const from: WikiRevisionSnapshot = {
    title: 'old title',
    summary: 'same summary',
    content: 'old body',
  }
  const to: WikiRevisionSnapshot = {
    title: 'new title',
    summary: 'same summary',
    content: 'new body',
  }
  const sections = diffWikiRevision(from, to)
  const titleLines = sections.find((s) => s.field === 'title')!.lines
  assert.ok(titleLines.some((l) => l.type === 'del'))
  assert.ok(titleLines.some((l) => l.type === 'add'))
  const summaryLines = sections.find((s) => s.field === 'summary')!.lines
  assert.ok(summaryLines.every((l) => l.type === 'same'))
})

test('diffWikiRevision handles identical snapshots as all-same', () => {
  const snap: WikiRevisionSnapshot = { title: 't', summary: 's', content: 'c' }
  const sections = diffWikiRevision(snap, snap)
  for (const section of sections) {
    assert.ok(section.lines.every((l) => l.type === 'same'))
  }
})

test('diffWikiRevision tolerates missing fields as empty strings', () => {
  const sections = diffWikiRevision(
    { title: '', summary: '', content: '' },
    { title: '', summary: '', content: '' },
  )
  assert.equal(sections.length, 3)
  for (const section of sections) {
    assert.ok(section.lines.every((l) => l.type === 'same'))
  }
})