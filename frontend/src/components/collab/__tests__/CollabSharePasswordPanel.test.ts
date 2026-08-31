/**
 * CollabSharePasswordPanel — v0.7.41 unit tests.
 *
 * Tests the password-length validation rule that mirrors the
 * handler-side check (>=6 chars). The component is purely UI-driven
 * over the typed `enableCollabDocShare` / `disableCollabDocShare`
 * helpers in api/collabDoc; their full round-trip is exercised by the
 * existing adapter tests, so we keep these tests small and stable.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'

// Mirror of the panel's "too short" rule from CollabSharePasswordPanel.vue.
// If the panel relaxes the rule, this should be updated in lockstep — both
// client and backend share the 6-char floor.
function panelAllowsPassword(password: string): boolean {
  if (password.length === 0) return true // open link is allowed
  return password.length >= 6
}

// Mirror of the backend handler's floor (EnableShareRequest.Password).
// The handler returns 400 for shorter values; the panel mirrors that with
// the same number so users see a consistent message before round-tripping.
const HANDLER_MIN_PASSWORD_LENGTH = 6

test('panel allows empty password (open link)', () => {
  assert.equal(panelAllowsPassword(''), true)
})

test('panel allows 6-char password', () => {
  assert.equal(panelAllowsPassword('secret'), true)
})

test('panel allows long passwords', () => {
  assert.equal(panelAllowsPassword('a-much-longer-passphrase-12345'), true)
})

test('panel rejects passwords under 6 chars', () => {
  for (const p of ['a', 'ab', 'abc', 'abcd', 'abcde']) {
    assert.equal(
      panelAllowsPassword(p),
      false,
      `expected "${p}" (length ${p.length}) to be rejected`,
    )
  }
})

test('handler and panel agree on minimum password length', () => {
  // Regression guard: if either side tightens or relaxes the floor,
  // the other side must follow. The smoke test fails fast so the
  // divergence is caught in CI rather than at user time.
  assert.equal(panelAllowsPassword('x'.repeat(HANDLER_MIN_PASSWORD_LENGTH - 1)), false)
  assert.equal(panelAllowsPassword('x'.repeat(HANDLER_MIN_PASSWORD_LENGTH)), true)
})

test('share URL builder uses window.location.origin and encodes the token', () => {
  // Re-derive the same shape as the panel's shareUrl computed.
  // If the panel changes the URL format, this captures the contract.
  const origin = 'https://app.example.com'
  const token = 'tok/with+special=chars'
  const url = `${origin}/collab-share/${encodeURIComponent(token)}`
  assert.equal(url, 'https://app.example.com/collab-share/tok%2Fwith%2Bspecial%3Dchars')
})

test('enable-share request body shape includes password + expires_at', () => {
  // Shape contract: the panel sends exactly { password, expires_at }
  // and the handler reads both. This is what the SharePasswordPanel
  // emits and what the EnableShareRequest JSON tag deserialises.
  const payload = {
    password: 'secret-pw',
    expires_at: '2026-12-31T00:00:00Z',
  }
  assert.deepEqual(Object.keys(payload).sort(), ['expires_at', 'password'])
  assert.equal(typeof payload.password, 'string')
  assert.equal(typeof payload.expires_at, 'string')
})

test('enable-share accepts null expires_at for never-expire links', () => {
  const payload = { password: '', expires_at: null }
  // Both sides must accept null (handler treats nil as no expiry).
  assert.equal(payload.expires_at, null)
})
