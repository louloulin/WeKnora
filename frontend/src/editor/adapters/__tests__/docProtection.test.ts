// v0.7.67 — DOC document protection adapter: Vue-friendly wrappers around the
// engine's hashProtectionPassword / verifyProtectionPassword / DocProtection.
//
// These tests run the makeProtectionPatch / applyDocProtection state-machine
// against the real docx-engine hash (SHA-512, 100000 iterations) so the
// round-trip stays aligned with the on-disk XML format.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  makeProtectionPatch,
  applyDocProtection,
  validateProtectionPatch,
  PROTECTION_KEEP,
  PROTECTION_MODES,
} from '../docProtection'
import {
  hashProtectionPassword,
  verifyProtectionPassword,
} from '../../engines/docx-engine/index'

test('docProtection: makeProtectionPatch returns defaults for empty current', () => {
  const patch = makeProtectionPatch(null)
  assert.equal(patch.enabled, false)
  assert.equal(patch.mode, 'trackedChanges')
  assert.equal(patch.password, '')
  assert.equal(patch.error, '')
})

test('docProtection: makeProtectionPatch preserves locked password via KEEP sentinel', () => {
  // pretend the docx already has an enforced+password protection
  const current = { edit: 'comments' as const, enforced: true, hash: 'abc', salt: 'xyz', spinCount: 100000, algorithmSid: 14 }
  const patch = makeProtectionPatch(current)
  assert.equal(patch.enabled, true)
  assert.equal(patch.mode, 'comments')
  assert.equal(patch.password, PROTECTION_KEEP)
})

test('docProtection: applyDocProtection creates a new restriction with hashed password', async () => {
  const patch = makeProtectionPatch(null)
  patch.enabled = true
  patch.mode = 'readOnly'
  patch.password = 'secret123'
  patch.passwordConfirm = 'secret123'
  const { protection, error } = await applyDocProtection(null, patch)
  assert.equal(error, '')
  assert.ok(protection)
  assert.equal(protection!.edit, 'readOnly')
  assert.equal(protection!.enforced, true)
  assert.ok(protection!.hash, 'hash should be set')
  assert.ok(protection!.salt, 'salt should be set')
  assert.equal(protection!.algorithmSid, 14)
  // round-trip with verifyProtectionPassword
  assert.equal(await verifyProtectionPassword('secret123', protection!), true)
  assert.equal(await verifyProtectionPassword('wrong', protection!), false)
})

test('docProtection: applyDocProtection mismatched password returns error', async () => {
  const patch = makeProtectionPatch(null)
  patch.enabled = true
  patch.mode = 'comments'
  patch.password = 'abc'
  patch.passwordConfirm = 'xyz'
  const { error } = await applyDocProtection(null, patch)
  assert.equal(error, 'pwdMismatch')
})

test('docProtection: applyDocProtection null restriction when disabled', async () => {
  const patch = makeProtectionPatch(null)
  patch.enabled = false
  const { protection, error } = await applyDocProtection(null, patch)
  assert.equal(error, '')
  assert.equal(protection, null)
})

test('docProtection: applyDocProtection keeps existing hash when KEEP sentinel is preserved', async () => {
  // existing lock
  const existing = await hashProtectionPassword('oldpwd')
  const current = {
    edit: 'trackedChanges' as const,
    enforced: true,
    hash: existing.hash,
    salt: existing.salt,
    spinCount: existing.spinCount,
    algorithmSid: existing.algorithmSid,
  }
  const patch = makeProtectionPatch(current)
  patch.enabled = true
  patch.mode = 'forms' // changed mode
  // password is KEEP, confirm is KEEP — unchanged
  const { protection, error } = await applyDocProtection(current, patch)
  assert.equal(error, '')
  assert.ok(protection)
  assert.equal(protection!.edit, 'forms')
  assert.equal(protection!.hash, existing.hash, 'hash should be unchanged')
  assert.equal(await verifyProtectionPassword('oldpwd', protection!), true)
})

test('docProtection: applyDocProtection requires unlock password to remove a locked restriction', async () => {
  const existing = await hashProtectionPassword('oldpwd')
  const current = {
    edit: 'readOnly' as const,
    enforced: true,
    hash: existing.hash,
    salt: existing.salt,
    spinCount: existing.spinCount,
    algorithmSid: existing.algorithmSid,
  }
  const patch = makeProtectionPatch(current)
  patch.enabled = false
  patch.unlockPassword = 'wrong'
  const { error } = await applyDocProtection(current, patch)
  assert.equal(error, 'wrongUnlock')

  patch.unlockPassword = 'oldpwd'
  const { protection, error: e2 } = await applyDocProtection(current, patch)
  assert.equal(e2, '')
  assert.equal(protection, null)
})

test('docProtection: validateProtectionPatch enforces 6-character minimum', () => {
  const patch = makeProtectionPatch(null)
  patch.enabled = true
  patch.password = 'short'
  assert.equal(validateProtectionPatch(patch), 'pwdTooShort')
  patch.password = 'longenough'
  assert.equal(validateProtectionPatch(patch), '')
})

test('docProtection: PROTECTION_MODES contains the four Word modes', () => {
  assert.deepEqual([...PROTECTION_MODES], [
    'trackedChanges',
    'comments',
    'readOnly',
    'forms',
  ])
})
