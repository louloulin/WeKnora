// v0.7.80 — DOC multi-column canvas layout extension (TipTap plugin + decoration API).
//
// Most of the heavy lifting lives in setColumnLayout(view, specs) which needs
// a real EditorView + DOM; that path is exercised in browser E2E. Here we
// just verify the module surface compiles and the exported names are usable.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  ColumnLayoutExtension,
  setColumnLayout,
  type ColumnBlockSpec,
} from '../docColumnLayout'

test('ColumnLayoutExtension: is a TipTap Extension', () => {
  // Extension.create returns an object with .name and .addProseMirrorPlugins
  assert.equal(typeof ColumnLayoutExtension, 'object')
  assert.equal((ColumnLayoutExtension as any).name, 'columnLayout')
  assert.equal(typeof (ColumnLayoutExtension as any).config.addProseMirrorPlugins, "function")
})

test('setColumnLayout: is a function', () => {
  assert.equal(typeof setColumnLayout, 'function')
})

test('ColumnBlockSpec: shape is structurally compatible (compile check)', () => {
  // Construct a spec without invoking setColumnLayout; the type system validates shape
  const spec: ColumnBlockSpec = {
    el: null as unknown as HTMLElement, // intentionally null for compile-only check
    widthPx: 200,
    contentWPx: 800,
    marginLeftPx: 50,
    marginRightPx: 50,
    dx: 0,
    dy: -120,
  }
  assert.equal(spec.dx, 0)
  assert.equal(spec.dy, -120)
  assert.equal(spec.widthPx, 200)
})
