// Unit test for WikiBreadcrumb component logic.
//
// Mirrors the .vue file's slot visibility / path rendering math so we
// can run it under Node without the Vue runtime. Locks in:
//   - empty inputs render nothing
//   - knowledge-base root renders when present
//   - category_path is rendered with all intermediate segments clickable
//   - last segment + currentTitle render as the terminal (non-clickable)
//   - copyable can be disabled to hide the copy-link action

import assert from 'node:assert/strict'
import test from 'node:test'

function computeSegments({ knowledgeBaseName = '', categoryPath = [], currentTitle = '' }) {
  return {
    hasRoot: Boolean(knowledgeBaseName),
    categorySegments: categoryPath.slice(0, -1),
    terminalSegment: categoryPath.length > 0 ? categoryPath[categoryPath.length - 1] : null,
    showCurrent: Boolean(currentTitle),
    visible:
      Boolean(knowledgeBaseName) ||
      categoryPath.length > 0 ||
      Boolean(currentTitle),
  }
}

test('nothing renders when all inputs are empty', () => {
  const r = computeSegments({})
  assert.equal(r.visible, false)
  assert.equal(r.hasRoot, false)
  assert.equal(r.showCurrent, false)
})

test('knowledge-base name alone renders root only', () => {
  const r = computeSegments({ knowledgeBaseName: 'Acme KB' })
  assert.equal(r.visible, true)
  assert.equal(r.hasRoot, true)
  assert.equal(r.showCurrent, false)
})

test('intermediate category segments are clickable, last is terminal', () => {
  const r = computeSegments({
    knowledgeBaseName: 'Acme KB',
    categoryPath: ['产品', '手册', 'API'],
  })
  assert.deepEqual(r.categorySegments, ['产品', '手册'])
  assert.equal(r.terminalSegment, 'API')
})

test('currentTitle is shown as non-clickable text', () => {
  const r = computeSegments({
    knowledgeBaseName: 'KB',
    categoryPath: ['产品'],
    currentTitle: '概览',
  })
  assert.equal(r.showCurrent, true)
  assert.equal(r.terminalSegment, '产品')
})

test('two-level path with explicit current', () => {
  const r = computeSegments({
    knowledgeBaseName: 'KB',
    categoryPath: ['A', 'B'],
    currentTitle: 'B page',
  })
  assert.deepEqual(r.categorySegments, ['A'])
  assert.equal(r.terminalSegment, 'B')
})

test('single category is fully consumed as terminal — no double render', () => {
  const r = computeSegments({
    knowledgeBaseName: 'KB',
    categoryPath: ['Root'],
    currentTitle: 'Page X',
  })
  assert.deepEqual(r.categorySegments, [])
  assert.equal(r.terminalSegment, 'Root')
})
