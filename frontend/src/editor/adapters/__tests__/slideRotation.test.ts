// v0.7.79 — PPT shape rotation math helpers.

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  normalizeRotation,
  stepRotation90,
  snapToDiscrete,
  relativeRotation,
  formatRotation,
  transformerSnapAngle,
  DEG_STEP,
} from '../slideRotation'

test('normalizeRotation: positive angles < 360', () => {
  assert.equal(normalizeRotation(0), 0)
  assert.equal(normalizeRotation(45), 45)
  assert.equal(normalizeRotation(359.9), 359.9)
})

test('normalizeRotation: wraps exactly 360 → 0', () => {
  assert.equal(normalizeRotation(360), 0)
  assert.equal(normalizeRotation(720), 0)
})

test('normalizeRotation: negative angles', () => {
  assert.equal(normalizeRotation(-1), 359)
  assert.equal(normalizeRotation(-45), 315)
  assert.equal(normalizeRotation(-90), 270)
})

test('stepRotation90: clockwise from 0 → 90', () => {
  assert.equal(stepRotation90(0, 1), 90)
})

test('stepRotation90: clockwise from 45 → 90', () => {
  assert.equal(stepRotation90(45, 1), 90)
})

test('stepRotation90: clockwise from 90 → 180', () => {
  assert.equal(stepRotation90(90, 1), 180)
})

test('stepRotation90: counter-clockwise from 0 → 270', () => {
  assert.equal(stepRotation90(0, -1), 270)
})

test('stepRotation90: counter-clockwise from 45 → 0', () => {
  assert.equal(stepRotation90(45, -1), 0)
})

test('stepRotation90: clockwise from 350 → 0 (wraps)', () => {
  assert.equal(stepRotation90(350, 1), 0)
})

test('snapToDiscrete: snaps to nearest 15°', () => {
  assert.equal(snapToDiscrete(0), 0)
  assert.equal(snapToDiscrete(7), 0)
  assert.equal(snapToDiscrete(8), 15)
  assert.equal(snapToDiscrete(22), 15)
  assert.equal(snapToDiscrete(23), 30)
  // 355° → round(355/15)=24 → 360 → normalize → 0
  assert.equal(snapToDiscrete(355), 0)
  assert.equal(snapToDiscrete(352), 345)
})

test('snapToDiscrete: respects custom step', () => {
  assert.equal(snapToDiscrete(13, 30), 0)
  assert.equal(snapToDiscrete(20, 30), 30)
  // 45° is exactly between 30 and 60 — JS Math.round rounds half away from zero → 60
  assert.equal(snapToDiscrete(45, 30), 60)
  assert.equal(snapToDiscrete(46, 30), 60)
})

test('snapToDiscrete: wraps negative angles correctly', () => {
  assert.equal(snapToDiscrete(-7), 0)
  assert.equal(snapToDiscrete(-22), 345)
})

test('relativeRotation: adds delta and normalizes', () => {
  assert.equal(relativeRotation(0, 45), 45)
  assert.equal(relativeRotation(350, 45), 35, 'wraps 395 → 35')
  assert.equal(relativeRotation(180, -90), 90)
})

test('formatRotation: rounds to integer degrees with °', () => {
  assert.equal(formatRotation(45), '45°')
  assert.equal(formatRotation(45.7), '46°')
  assert.equal(formatRotation(-1), '359°')
})

test('transformerSnapAngle: shift held → 15°, otherwise undefined (free rotate)', () => {
  assert.equal(transformerSnapAngle(true), DEG_STEP)
  assert.equal(transformerSnapAngle(false), undefined)
})

test('DEG_STEP is 15 (Word/PPT default)', () => {
  assert.equal(DEG_STEP, 15)
})
