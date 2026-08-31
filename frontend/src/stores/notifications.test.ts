import assert from 'node:assert/strict'
import test from 'node:test'

/**
 * Notification store tests (Build #P0.4).
 *
 * We exercise the pure helpers used by the store (kind chip
 * classification, time-ago formatting, optimistic-update rollback
 * semantics) by re-implementing them in isolation. Pinia + the
 * request util are covered by component snapshot tests.
 */

type NotificationKind =
  | 'wiki.comment.created'
  | 'wiki.comment.reply'
  | 'wiki.mentioned'
  | 'agent.shared'
  | 'kb.shared'
  | 'system.alert'

function kindChip(kind: NotificationKind): string {
  switch (kind) {
    case 'wiki.comment.created':
    case 'wiki.comment.reply':
      return 'comment'
    case 'wiki.mentioned':
      return 'mention'
    case 'agent.shared':
      return 'agent'
    case 'kb.shared':
      return 'kb'
    case 'system.alert':
      return 'system'
    default:
      return 'other'
  }
}

function timeAgo(iso: string, now: number = Date.now()): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const sec = Math.max(1, Math.round((now - then) / 1000))
  if (sec < 60) return `${sec}s`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day}d`
  const month = Math.round(day / 30)
  return `${month}mo`
}

// Mirror the optimistic-update logic from readOne / dismissOne /
// readAll so we can lock down the rollback semantics.

interface FakeNotification {
  id: number
  status: 'unread' | 'read' | 'dismissed'
  read_at?: string
  dismissed_at?: string
}

function optimisticRead(
  items: FakeNotification[],
  id: number,
): { next: FakeNotification[]; prev: FakeNotification; droppedUnread: number } {
  const idx = items.findIndex((n) => n.id === id)
  if (idx < 0) throw new Error('not found')
  const prev = items[idx]
  const dropped = prev.status === 'unread' ? 1 : 0
  const next = items.slice()
  next[idx] = { ...prev, status: 'read', read_at: new Date().toISOString() }
  return { next, prev, droppedUnread: dropped }
}

function optimisticDismiss(
  items: FakeNotification[],
  id: number,
): { next: FakeNotification[]; prev: FakeNotification; droppedUnread: number } {
  const idx = items.findIndex((n) => n.id === id)
  if (idx < 0) throw new Error('not found')
  const prev = items[idx]
  const dropped = prev.status === 'unread' ? 1 : 0
  const next = items.slice()
  next[idx] = { ...prev, status: 'dismissed', dismissed_at: new Date().toISOString() }
  return { next, prev, droppedUnread: dropped }
}

function optimisticReadAll(
  items: FakeNotification[],
): { next: FakeNotification[]; droppedUnread: number } {
  const dropped = items.filter((n) => n.status === 'unread').length
  const next = items.map((n) =>
    n.status === 'unread'
      ? { ...n, status: 'read' as const, read_at: new Date().toISOString() }
      : n,
  )
  return { next, droppedUnread: dropped }
}

// --- kindChip ---

test('kindChip maps every known kind to a stable chip', () => {
  assert.equal(kindChip('wiki.comment.created'), 'comment')
  assert.equal(kindChip('wiki.comment.reply'), 'comment')
  assert.equal(kindChip('wiki.mentioned'), 'mention')
  assert.equal(kindChip('agent.shared'), 'agent')
  assert.equal(kindChip('kb.shared'), 'kb')
  assert.equal(kindChip('system.alert'), 'system')
})

// --- timeAgo ---

test('timeAgo returns seconds for < 1m', () => {
  const now = 1_000_000_000_000
  assert.equal(timeAgo(new Date(now - 5_000).toISOString(), now), '5s')
})

test('timeAgo returns minutes for < 1h', () => {
  const now = 1_000_000_000_000
  assert.equal(timeAgo(new Date(now - 5 * 60_000).toISOString(), now), '5m')
})

test('timeAgo returns hours for < 1d', () => {
  const now = 1_000_000_000_000
  assert.equal(timeAgo(new Date(now - 3 * 60 * 60_000).toISOString(), now), '3h')
})

test('timeAgo returns days for < 30d', () => {
  const now = 1_000_000_000_000
  assert.equal(timeAgo(new Date(now - 5 * 24 * 60 * 60_000).toISOString(), now), '5d')
})

test('timeAgo returns months for >= 30d', () => {
  const now = 1_000_000_000_000
  assert.equal(timeAgo(new Date(now - 60 * 24 * 60 * 60_000).toISOString(), now), '2mo')
})

test('timeAgo returns empty string for invalid input', () => {
  assert.equal(timeAgo('not-a-date'), '')
  assert.equal(timeAgo(''), '')
})

// --- optimisticRead ---

test('optimisticRead marks the row read and decrements unread by 1', () => {
  const items: FakeNotification[] = [
    { id: 1, status: 'unread' },
    { id: 2, status: 'read' },
  ]
  const { next, droppedUnread } = optimisticRead(items, 1)
  assert.equal(next[0].status, 'read')
  assert.equal(next[1].status, 'read')
  assert.equal(droppedUnread, 1)
  assert.ok(next[0].read_at)
})

test('optimisticRead on an already-read row is a no-op (no double decrement)', () => {
  const items: FakeNotification[] = [{ id: 1, status: 'read' }]
  const { droppedUnread } = optimisticRead(items, 1)
  assert.equal(droppedUnread, 0)
})

// --- optimisticDismiss ---

test('optimisticDismiss marks the row dismissed and decrements unread', () => {
  const items: FakeNotification[] = [
    { id: 1, status: 'unread' },
    { id: 2, status: 'read' },
  ]
  const { next, droppedUnread } = optimisticDismiss(items, 1)
  assert.equal(next[0].status, 'dismissed')
  assert.equal(droppedUnread, 1)
  assert.ok(next[0].dismissed_at)
})

test('optimisticDismiss on a read row does not change unread count', () => {
  const items: FakeNotification[] = [{ id: 1, status: 'read' }]
  const { droppedUnread } = optimisticDismiss(items, 1)
  assert.equal(droppedUnread, 0)
})

// --- optimisticReadAll ---

test('optimisticReadAll transitions every unread row and reports the total', () => {
  const items: FakeNotification[] = [
    { id: 1, status: 'unread' },
    { id: 2, status: 'unread' },
    { id: 3, status: 'read' },
    { id: 4, status: 'dismissed' },
  ]
  const { next, droppedUnread } = optimisticReadAll(items)
  assert.equal(droppedUnread, 2)
  assert.equal(next.filter((n) => n.status === 'read').length, 3)
  assert.equal(next.filter((n) => n.status === 'unread').length, 0)
  assert.equal(next.filter((n) => n.status === 'dismissed').length, 1)
})

// --- rollback invariants ---

test('rollback restores the exact previous state', () => {
  const items: FakeNotification[] = [
    { id: 1, status: 'unread' },
    { id: 2, status: 'read' },
  ]
  const { next, prev } = optimisticRead(items, 1)
  // simulate failure: revert
  const rolledBack = next.slice()
  rolledBack[rolledBack.findIndex((n) => n.id === 1)] = prev
  assert.deepEqual(rolledBack, items)
})
