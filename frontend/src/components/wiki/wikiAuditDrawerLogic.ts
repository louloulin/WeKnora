// Build #24 — pure helpers for the unified wiki audit drawer.
//
// No vue / i18n imports so the helpers can be unit-tested in Node
// without a DOM. The companion component WikiAuditDrawer.vue wires
// these to the live store / API client.

import type { WikiAuditSource } from '../../api/wiki/auditTypes'

// AllAuditSources is the closed set of source kinds the drawer can
// render. Mirrors the Go enum on the server side; the order is the
// order the filter chips present them.
export const AllAuditSources: WikiAuditSource[] = [
  'audit_logs',
  'wiki_batch_job_audit',
  'wiki_backlinks_cache_invalidation_log',
  'wiki_page_acl_audit',
]

// SourceLabel returns the i18n key suffix for a source. The component
// composes this with `wiki.audit.source.${source}` to produce the
// full key.
export function sourceLabelSuffix(source: WikiAuditSource): string {
  switch (source) {
    case 'audit_logs':
      return 'activity'
    case 'wiki_batch_job_audit':
      return 'batch'
    case 'wiki_backlinks_cache_invalidation_log':
      return 'invalidation'
    case 'wiki_page_acl_audit':
      return 'acl'
  }
}

// ActorKindLabel returns the i18n key suffix for an actor kind.
// Mirrors WikiAuditActorKind so the badge can render a chip per kind.
export type ActorKindLabelKind = 'user' | 'system' | 'sweep'

export function actorKindLabelSuffix(kind: string): ActorKindLabelKind {
  if (kind === 'system') return 'system'
  if (kind === 'sweep') return 'sweep'
  return 'user'
}

// OpBadgeKind groups ops into render buckets so the chip color matches
// the source family. The component uses this to pick a t-tag theme.
export type OpBadgeKind = 'activity' | 'batch' | 'acl' | 'invalidation' | 'unknown'

export function opBadgeKind(op: string, source: WikiAuditSource): OpBadgeKind {
  if (source === 'wiki_batch_job_audit') return 'batch'
  if (source === 'wiki_page_acl_audit') return 'acl'
  if (source === 'wiki_backlinks_cache_invalidation_log') return 'invalidation'
  if (source === 'audit_logs') return 'activity'
  return 'unknown'
}

// FormatTimestamp turns an RFC3339 string into a locale-aware
// `YYYY-MM-DD HH:mm:ss` shape. Pure — Date input is normalized via
// `new Date()` so the function tolerates the server's microsecond
// precision.
export function formatAuditTimestamp(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => (n < 10 ? `0${n}` : `${n}`)
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}

// shortActorId collapses a user id to its prefix for log readability.
// Empty / "system" / "sweep" pass through unchanged.
export function shortActorId(actor: string): string {
  if (!actor) return ''
  if (actor === 'system' || actor === 'sweep') return actor
  if (actor.length <= 12) return actor
  return `${actor.slice(0, 8)}…`
}

// SourceLabelI18nKey is the full i18n key for a source label.
// Convenience for the component template.
export function sourceLabelI18nKey(source: WikiAuditSource): string {
  return `wiki.audit.source.${sourceLabelSuffix(source)}`
}

// ActorKindI18nKey is the full i18n key for an actor-kind badge.
export function actorKindI18nKey(kind: string): string {
  return `wiki.audit.actorKind.${actorKindLabelSuffix(kind)}`
}

// OpI18nKey is the full i18n key for an op label. Some ops have a
// dedicated key under `wiki.audit.op.<op>`; unknown ops fall back to
// `wiki.audit.op.fallback`. The component renders the raw op string
// when the i18n key is missing.
export function opI18nKey(op: string): string {
  return `wiki.audit.op.${op}`
}

// EmptySourceCounts returns a fresh zero-fill record. The component
// uses this to render an empty state without crashing on a cold cache.
export function emptySourceCounts(): Record<WikiAuditSource, number> {
  return {
    audit_logs: 0,
    wiki_batch_job_audit: 0,
    wiki_backlinks_cache_invalidation_log: 0,
    wiki_page_acl_audit: 0,
  }
}