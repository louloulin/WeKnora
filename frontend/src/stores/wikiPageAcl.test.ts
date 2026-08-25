import assert from 'node:assert/strict'
import test from 'node:test'

import {
  decodeAclError,
  emitAclEvent,
  normalizeAcl,
  onAclEvent,
} from './wikiPageAclConflict.ts'

test('normalizeAcl coerces unknown shapes to a valid WikiPageAcl', () => {
  const fallback = normalizeAcl(null)
  assert.equal(fallback.mode, 'inherit')
  assert.deepEqual(fallback.allowUserIds, [])
  assert.equal(fallback.denyInherited, false)

  const allowList = normalizeAcl({
    mode: 'allow_list',
    allowUserIds: ['u1', 'u2', 42, 'u3'],
    allowGroupIds: ['g1'],
    denyInherited: true,
    revision: 7,
    updatedAt: '2026-08-26T00:00:00Z',
  })
  assert.equal(allowList.mode, 'allow_list')
  assert.deepEqual(allowList.allowUserIds, ['u1', 'u2', 'u3'])
  assert.equal(allowList.revision, 7)
  assert.equal(allowList.updatedAt, '2026-08-26T00:00:00Z')

  // unknown mode falls back to inherit
  const bogus = normalizeAcl({ mode: 'super-secret', allowUserIds: ['x'] })
  assert.equal(bogus.mode, 'inherit')
})

test('decodeAclError extracts the canonical ACL from a 409 body (currentAcl)', () => {
  const decoded = decodeAclError({
    status: 409,
    data: {
      currentAcl: {
        mode: 'private',
        allowUserIds: ['owner'],
        allowGroupIds: [],
        denyInherited: false,
        revision: 9,
        updatedAt: '2026-08-26T01:02:03Z',
      },
    },
  })
  assert.equal(decoded.kind, 'conflict')
  if (decoded.kind === 'conflict') {
    assert.equal(decoded.current.mode, 'private')
    assert.deepEqual(decoded.current.allowUserIds, ['owner'])
    assert.equal(decoded.current.revision, 9)
  }
})

test('decodeAclError tolerates snake_case and nested data shapes for 409', () => {
  const snake = decodeAclError({
    status: 409,
    data: { current_acl: { mode: 'allow_list', allowUserIds: ['x'] } },
  })
  assert.equal(snake.kind, 'conflict')
  if (snake.kind === 'conflict') {
    assert.equal(snake.current.mode, 'allow_list')
    assert.deepEqual(snake.current.allowUserIds, ['x'])
  }

  const nested = decodeAclError({
    status: 409,
    data: { data: { mode: 'private', allowUserIds: [] } },
  })
  assert.equal(nested.kind, 'conflict')

  // Missing payload still produces a usable conflict branch (default = inherit).
  const missing = decodeAclError({ status: 409, data: {} })
  assert.equal(missing.kind, 'conflict')
  if (missing.kind === 'conflict') {
    assert.equal(missing.current.mode, 'inherit')
  }
})

test('decodeAclError classifies non-409 errors as error with the message', () => {
  const denied = decodeAclError({ status: 403, message: 'acl.denied' })
  assert.equal(denied.kind, 'error')
  if (denied.kind === 'error') {
    assert.equal(denied.message, 'acl.denied')
  }

  const fallback = decodeAclError({ status: 500 })
  assert.equal(fallback.kind, 'error')
  if (fallback.kind === 'error') {
    assert.equal(fallback.message, 'acl.saveFailed')
  }

  // Bare unknown values still decode gracefully.
  const weird = decodeAclError('not-an-object')
  assert.equal(weird.kind, 'error')
})

test('onAclEvent delivers events to listeners and supports unsubscribe', () => {
  const received: Array<{ kind: string; kbId: string; slug: string }> = []
  const listener = (event: { kind: string; kbId: string; slug: string }) => {
    received.push(event)
  }
  const off = onAclEvent(listener)
  emitAclEvent({ kind: 'updated', kbId: 'kb1', slug: 'page-a', acl: normalizeAcl(null) })
  emitAclEvent({ kind: 'conflict', kbId: 'kb1', slug: 'page-a', current: normalizeAcl(null) })
  assert.equal(received.length, 2)
  assert.equal(received[0].kind, 'updated')
  assert.equal(received[1].kind, 'conflict')

  // After unsubscribe, further deliveries are ignored.
  off()
  emitAclEvent({ kind: 'updated', kbId: 'kb1', slug: 'page-a', acl: normalizeAcl(null) })
  assert.equal(received.length, 2)
})

test('onAclEvent swallows listener exceptions and keeps delivering', () => {
  const received: string[] = []
  onAclEvent(() => {
    throw new Error('boom')
  })
  const good = onAclEvent((event) => {
    received.push(event.kind)
  })
  emitAclEvent({ kind: 'updated', kbId: 'kb1', slug: 'page-a', acl: normalizeAcl(null) })
  assert.deepEqual(received, ['updated'])
  good()
})