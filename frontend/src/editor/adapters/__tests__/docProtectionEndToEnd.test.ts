// v0.7.67 — End-to-end: open a blank docx, set protection via the adapter,
// save back to .docx, re-open, and verify protection round-trips.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import { openDocx, saveDocxBytes } from '../docxAdapter'
import {
  hashProtectionPassword,
  verifyProtectionPassword,
  type SaveBlock,
} from '../../engines/docx-engine/index'

test('docProtection end-to-end: adapter passes protection to settings.xml', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const doc = await openDocx(bytes)
  // no protection on fresh docx
  assert.equal(doc.parsed.protection, null)

  // set readOnly + password
  const { hash, salt, spinCount, algorithmSid } = await hashProtectionPassword('hunter2')
  const blocks: SaveBlock[] = doc.parsed.blocks.map((b: any) => ({
    kind: 'original' as const,
    docxIndex: b.docxIndex,
  }))
  const saved = await saveDocxBytes(doc, blocks, {
    protection: { edit: 'readOnly', enforced: true, hash, salt, spinCount, algorithmSid },
  })

  // verify the bytes
  const zip = await JSZip.loadAsync(saved)
  const settingsXml = await zip.file('word/settings.xml')?.async('string')
  assert.ok(settingsXml, 'settings.xml exists')
  assert.match(
    settingsXml ?? '',
    /<w:documentProtection[^>]*w:edit="readOnly"[^>]*w:enforcement="1"[^>]*\/>/,
    'settings.xml contains enforced readOnly documentProtection',
  )
  assert.match(settingsXml ?? '', /w:hash="/, 'settings.xml contains hashed password')
  assert.match(settingsXml ?? '', /w:salt="/, 'settings.xml contains salt')

  // re-open and verify password
  const reopened = await openDocx(saved)
  assert.ok(reopened.parsed.protection)
  assert.equal(reopened.parsed.protection!.edit, 'readOnly')
  assert.equal(reopened.parsed.protection!.enforced, true)
  assert.equal(await verifyProtectionPassword('hunter2', reopened.parsed.protection!), true)
  assert.equal(await verifyProtectionPassword('nope', reopened.parsed.protection!), false)
})

test('docProtection end-to-end: adapter removes protection when option is null', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const doc = await openDocx(bytes)
  const blocks: SaveBlock[] = doc.parsed.blocks.map((b: any) => ({
    kind: 'original' as const,
    docxIndex: b.docxIndex,
  }))
  // save with no protection → settings.xml should not contain documentProtection
  const saved = await saveDocxBytes(doc, blocks, { protection: null })
  const zip = await JSZip.loadAsync(saved)
  const settingsXml = await zip.file('word/settings.xml')?.async('string')
  assert.ok(settingsXml)
  assert.doesNotMatch(settingsXml ?? '', /<w:documentProtection/, 'no documentProtection when null')
})

test('docProtection end-to-end: writeProtection (password to modify) round-trips', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const doc = await openDocx(bytes)
  const blocks: SaveBlock[] = doc.parsed.blocks.map((b: any) => ({
    kind: 'original' as const,
    docxIndex: b.docxIndex,
  }))
  const { hash, salt, spinCount, algorithmSid } = await hashProtectionPassword('modifyPwd')
  const saved = await saveDocxBytes(doc, blocks, {
    writeProtection: { recommended: true, hash, salt, spinCount, algorithmSid },
  })
  const zip = await JSZip.loadAsync(saved)
  const settingsXml = await zip.file('word/settings.xml')?.async('string')
  assert.ok(settingsXml)
  assert.match(settingsXml ?? '', /<w:writeProtection[^>]*w:recommended="1"/, 'writeProtection with recommended=1')
  assert.match(settingsXml ?? '', /w:hash="/, 'writeProtection has hash')

  const reopened = await openDocx(saved)
  assert.ok(reopened.parsed.writeProtection)
  assert.equal(reopened.parsed.writeProtection!.recommended, true)
})
