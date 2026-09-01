// v0.7.67 — DOC document protection round-trip: options.protection → w:documentProtection in settings.xml.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  parseDocx,
  saveDocx,
  hashProtectionPassword,
  verifyProtectionPassword,
  type SaveBlock,
} from '../../engines/docx-engine/index'

test('DOC protection: readOnly enforcement → w:documentProtection in settings.xml', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const blocks: SaveBlock[] = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const saved = await saveDocx(original, blocks, {
    protection: { edit: 'readOnly', enforced: true },
  })
  const zip = await JSZip.loadAsync(saved)
  const settingsXml = await zip.file('word/settings.xml')?.async('string')
  assert.ok(settingsXml)
  assert.match(settingsXml ?? '', /<w:documentProtection[^/]*w:edit="readOnly"[^/]*w:enforcement="1"[^/]*\/>/, 'settings.xml contains documentProtection readOnly enforcement=1')
})

test('DOC protection: with password hash → round-trip + verify', async () => {
  const { buildBlankDocx } = await import('../../engines/docx-engine/blank')
  const bytes = await buildBlankDocx()
  const original = await parseDocx(bytes)
  const blocks: SaveBlock[] = original.blocks.map((b: any) => ({ kind: 'original' as const, docxIndex: b.docxIndex }))
  const { hash, salt, spinCount, algorithmSid } = await hashProtectionPassword('test123')
  const saved = await saveDocx(original, blocks, {
    protection: { edit: 'readOnly', enforced: true, hash, salt, spinCount, algorithmSid },
  })
  const reopened = await parseDocx(saved)
  assert.ok(reopened.protection)
  assert.equal(reopened.protection?.edit, 'readOnly')
  assert.equal(reopened.protection?.enforced, true)
  assert.equal(await verifyProtectionPassword('test123', reopened.protection!), true)
  assert.equal(await verifyProtectionPassword('wrong', reopened.protection!), false)
})
