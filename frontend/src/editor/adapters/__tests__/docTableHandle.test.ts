// v0.7.55 — DOC table move handle extension config.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { DocTableHandle } from '../docTableHandle'

test('DocTableHandle registers a ProseMirror plugin', () => {
  assert.equal(DocTableHandle.name, 'docTableHandle')
  const plugins = DocTableHandle.config.addProseMirrorPlugins?.() ?? []
  assert.equal(plugins.length, 1)
  const key = plugins[0].spec.key as { key: string }
  assert.equal(key.key, 'docTableHandle$')
})
