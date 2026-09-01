// v0.7.77 — SHEET cell-lock helpers (soft optimistic lock from awareness).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  isCellLockedByOther,
  buildLockMap,
  cellKey,
  checkEditAllowed,
  type RemoteCellPeer,
} from '../xlsxCellLock'

const myId = 100
const peerAlice: RemoteCellPeer = { clientId: 1, displayName: 'Alice', color: '#f00', cell: { ri: 0, ci: 0 } }
const peerBob: RemoteCellPeer = { clientId: 2, displayName: 'Bob', color: '#0f0', cell: { ri: 1, ci: 1 } }
const peerWithNoCell: RemoteCellPeer = { clientId: 3, displayName: 'Carol', color: '#00f', cell: null }

test('isCellLockedByOther: empty remote → not locked', () => {
  assert.deepEqual(isCellLockedByOther([], myId, 0, 0), { locked: false, locker: null })
})

test('isCellLockedByOther: peer holds cell → locked', () => {
  const result = isCellLockedByOther([peerAlice], myId, 0, 0)
  assert.equal(result.locked, true)
  assert.equal(result.locker?.displayName, 'Alice')
})

test('isCellLockedByOther: peer at different cell → not locked', () => {
  const result = isCellLockedByOther([peerAlice], myId, 5, 5)
  assert.equal(result.locked, false)
  assert.equal(result.locker, null)
})

test('isCellLockedByOther: my own selection → not locked (filter by myClientId)', () => {
  const me: RemoteCellPeer = { clientId: myId, displayName: 'Me', color: '#fff', cell: { ri: 0, ci: 0 } }
  const result = isCellLockedByOther([me], myId, 0, 0)
  assert.equal(result.locked, false)
})

test('isCellLockedByOther: peer with cell=null → not locked', () => {
  const result = isCellLockedByOther([peerWithNoCell], myId, 0, 0)
  assert.equal(result.locked, false)
})

test('buildLockMap: collects one entry per locked cell', () => {
  const map = buildLockMap([peerAlice, peerBob], myId)
  assert.equal(map.size, 2)
  assert.equal(map.get('0:0')?.displayName, 'Alice')
  assert.equal(map.get('1:1')?.displayName, 'Bob')
})

test('buildLockMap: skips me and no-cell peers', () => {
  const me: RemoteCellPeer = { clientId: myId, displayName: 'Me', color: '#fff', cell: { ri: 9, ci: 9 } }
  const map = buildLockMap([me, peerAlice, peerWithNoCell], myId)
  assert.equal(map.size, 1)
  assert.equal(map.has('9:9'), false, 'my own cell not in map')
})

test('buildLockMap: when two peers lock the same cell, last write wins (Map keyed by cell)', () => {
  const peerBobSameCell: RemoteCellPeer = { ...peerBob, cell: { ri: 0, ci: 0 } }
  const map = buildLockMap([peerAlice, peerBobSameCell], myId)
  assert.equal(map.size, 1)
  assert.equal(map.get('0:0')?.displayName, 'Bob', 'later peer overwrites')
})

test('cellKey: encodes (ri, ci)', () => {
  assert.equal(cellKey(0, 0), '0:0')
  assert.equal(cellKey(42, 17), '42:17')
})

test('checkEditAllowed: no peer → allowed', () => {
  assert.deepEqual(checkEditAllowed([], myId, 0, 0), { allowed: true })
})

test('checkEditAllowed: peer locks → blocked with locker name', () => {
  const result = checkEditAllowed([peerAlice], myId, 0, 0)
  assert.equal(result.allowed, false)
  if (!result.allowed) {
    assert.equal(result.locker, 'Alice')
  }
})

test('checkEditAllowed: my own selection → allowed (own cell)', () => {
  const me: RemoteCellPeer = { clientId: myId, displayName: 'Me', color: '#fff', cell: { ri: 0, ci: 0 } }
  assert.deepEqual(checkEditAllowed([me], myId, 0, 0), { allowed: true })
})
