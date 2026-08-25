import type { Ref } from 'vue'

/**
 * Pure helpers for the wiki search history list (Build #9-A).
 *
 * Kept as a side-effect-free module so the rules (2-char minimum,
 * dedupe, FIFO cap at 10) can be unit-tested in plain Node without
 * pulling Pinia or the API client into the test runtime.
 */

export const HISTORY_MAX = 10
export const MIN_QUERY_LENGTH = 2

export function normalizeQuery(raw: string): string {
  return raw.trim()
}

export function shouldRecord(raw: string): boolean {
  return normalizeQuery(raw).length >= MIN_QUERY_LENGTH
}

/**
 * Returns the next history array. Pure: never mutates `current`.
 * - dedupe by exact string match
 * - newest at index 0
 * - capped at HISTORY_MAX
 */
export function pushHistory(current: readonly string[], raw: string): string[] {
  if (!shouldRecord(raw)) return current.slice()
  const t = normalizeQuery(raw)
  const without = current.filter((x) => x !== t)
  return [t, ...without].slice(0, HISTORY_MAX)
}

/**
 * Apply the history rules to a Vue `Ref<string[]>`. Used by the
 * Pinia store; tests use the underlying `pushHistory` to avoid
 * pulling Vue into the test runtime.
 */
export function pushHistoryRef(historyRef: Ref<string[]>, raw: string): void {
  historyRef.value = pushHistory(historyRef.value, raw)
}