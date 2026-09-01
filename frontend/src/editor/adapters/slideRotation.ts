/**
 * slideRotation — PPT shape rotation helpers (v0.7.79).
 *
 * Local implementation; genoffice has rotateEnabled on the Konva transformer
 * but no programmatic 90° step buttons or numeric input. We expose:
 *
 *  - normalizeRotation: wrap any angle to [0, 360)
 *  - stepRotation: round rotation to the nearest 90° step (Word/PPT "snap to 15°")
 *  - relativeRotation: add a delta and normalize
 *  - snapToDiscrete: snap a continuous angle to the closest of [0, 15, 30, ..., 345]
 */

export const DEG_STEP = 15

/** Wrap any angle in degrees into [0, 360). */
export function normalizeRotation(deg: number): number {
  const m = deg % 360
  return m < 0 ? m + 360 : m
}

/** Round to the nearest 90° stop (Word's Rotate 90° Left/Right keeps it on a stop). */
export function stepRotation90(deg: number, direction: 1 | -1): number {
  const cur = normalizeRotation(deg)
  const target = direction > 0 ? Math.ceil(cur / 90) * 90 : Math.floor(cur / 90) * 90
  return normalizeRotation(target === cur ? (cur + direction * 90) : target)
}

/** Snap a free angle to the nearest `DEG_STEP°` step (Word behavior while dragging). */
export function snapToDiscrete(deg: number, step: number = DEG_STEP): number {
  const cur = normalizeRotation(deg)
  return normalizeRotation(Math.round(cur / step) * step)
}

/** Apply a delta and re-normalize. */
export function relativeRotation(deg: number, delta: number): number {
  return normalizeRotation(deg + delta)
}

/** Human-readable label for a rotation value, e.g. "45°". */
export function formatRotation(deg: number): string {
  return `${Math.round(normalizeRotation(deg))}°`
}

/** Compute the snap target used by Konva's transformer (15° when Shift is held). */
export function transformerSnapAngle(shiftKey: boolean): number | undefined {
  return shiftKey ? DEG_STEP : undefined
}
