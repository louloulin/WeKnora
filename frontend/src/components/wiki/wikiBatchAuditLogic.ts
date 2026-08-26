// Build #14 — shared helpers for the audit-log drawer components.
// Pure functions only — no vue / i18n imports so the helpers can be unit-tested
// with `tsx --test` without a DOM.

import type { WikiBatchAuditAction } from '../../api/wiki'

// AllAuditActions is the closed set of action kinds the UI knows how to
// render. Mirrors the Go enum on the server side; the order is the order
// the select filter presents them.
export const AllAuditActions: WikiBatchAuditAction[] = [
  'enqueue',
  'start',
  'finish',
  'undo_request',
  'undo_done',
  'cancel',
  'expire',
]

// Terminal actions cap a job's lifecycle — once one of these fires the
// audit chain is done. The UI uses this to bolden the label and the
// history drawer to drop the connector line.
export function isTerminalAuditAction(a: WikiBatchAuditAction): boolean {
  return a === 'finish' || a === 'undo_done' || a === 'cancel' || a === 'expire'
}

// shortJobId returns a stable short identifier for the per-job link
// without leaking the full UUID into the audit log cell.
export function shortJobId(jobId: string): string {
  if (!jobId) return ''
  return jobId.slice(0, 8)
}

// jobDrawerTitleTokens extracts `{id}` from a job-id for i18n interpolation.
export function jobDrawerTitleTokens(
  jobId: string,
): { id: string } {
  return { id: shortJobId(jobId) }
}