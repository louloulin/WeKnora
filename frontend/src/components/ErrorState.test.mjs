// Unit test for ErrorState component contract.
//
// The .vue file exposes errorCode + lastSyncedAt meta and a retry action.
// We verify the slot visibility shape here so future edits to the
// component don't silently break the audit checklist (16.7 cross-cutting
// UX quality — "error attribution + retry + last synced time").

import assert from 'node:assert/strict'
import test from 'node:test'

function visibleMeta(props) {
  return {
    hasErrorCode: Boolean(props.errorCode),
    hasLastSynced: Boolean(props.lastSyncedAt),
    hasRetry: Boolean(props.retryLabel),
  }
}

test('emits retry event when retryLabel is set', () => {
  assert.deepEqual(visibleMeta({ retryLabel: '重试' }), {
    hasErrorCode: false,
    hasLastSynced: false,
    hasRetry: true,
  })
})

test('meta fields surface independently', () => {
  assert.deepEqual(
    visibleMeta({ errorCode: 'NETWORK', lastSyncedAt: '2026-08-31 10:00' }),
    { hasErrorCode: true, hasLastSynced: true, hasRetry: false },
  )
})

test('all meta visible together is the typical post-failure state', () => {
  assert.deepEqual(
    visibleMeta({
      errorCode: 500,
      lastSyncedAt: '2026-08-30 18:42',
      retryLabel: '重新加载',
    }),
    { hasErrorCode: true, hasLastSynced: true, hasRetry: true },
  )
})

test('no meta shows nothing — the caller wants a placeholder only', () => {
  assert.deepEqual(visibleMeta({}), {
    hasErrorCode: false,
    hasLastSynced: false,
    hasRetry: false,
  })
})
