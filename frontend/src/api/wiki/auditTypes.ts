/**
 * Build #24 — types for the unified wiki audit endpoint.
 *
 * Mirrors `internal/types/wiki_audit.go` so this module can be unit-tested
 * in Node without pulling in `utils/request.ts`. The endpoint merges four
 * sources:
 *
 *   - `audit_logs` — KB-scoped projection of the activity log
 *   - `wiki_batch_job_audit` — Build #14 batch audit events
 *   - `wiki_backlinks_cache_invalidation_log` — Build #23 wipe events
 *   - `wiki_page_acl_audit` — Build #7 ACL writes (was write-only)
 *
 * Each event is normalised to a common `WikiAuditEvent` shape. The
 * envelope includes `SourceCounts` so the drawer can render a filter-chip
 * group without a second round-trip, and `Total` so the pagination can
 * size its first page.
 */

// WikiAuditSource mirrors the Go-side enum. The string values match the
// `source_event_id` prefix on each row so the id prefix is also a stable
// source discriminator.
export type WikiAuditSource =
  | 'audit_logs'
  | 'wiki_batch_job_audit'
  | 'wiki_backlinks_cache_invalidation_log'
  | 'wiki_page_acl_audit';

// WikiAuditActorKind mirrors the Go-side enum. We default to 'user'
// when the server omits the field on legacy rows.
export type WikiAuditActorKind = 'user' | 'system' | 'sweep';

// WikiAuditSourceLabel is the user-facing label per source. Mirrors the
// 4 chip strings on the drawer's filter row.
export const WikiAuditSourceLabels: Record<WikiAuditSource, string> = {
  audit_logs: 'activity',
  wiki_batch_job_audit: 'batch',
  wiki_backlinks_cache_invalidation_log: 'invalidation',
  wiki_page_acl_audit: 'acl',
};

// WikiAuditEvent is the normalised event shape returned by the
// `audit_events` endpoint. Server-side field names use snake_case to
// match the rest of the public API surface.
export interface WikiAuditEvent {
  // Stable id with prefix (`al:`, `ba:`, `il:`, `la:`) + source row id.
  id: string;
  // RFC3339 timestamp. The drawer formats it as a localised date-time.
  timestamp: string;
  kb_id: string;
  // Optional slug; ACL events carry one, batch jobs may not.
  slug?: string;
  // Action / op label, e.g. `manual_create`, `set_private`,
  // `acl_change`. Free-form string — the drawer's per-source tables
  // format each op via its i18n entry.
  op: string;
  source: WikiAuditSource;
  actor: string;
  actor_kind: WikiAuditActorKind;
  // Source-side row id for cross-reference (Build #25 / Build #24 B25).
  source_event_id?: string;
  // Affected row count for invalidation events; 0 otherwise.
  affected_count?: number;
  // Free-form JSON for source-specific fields (mode change, batch
  // metadata, etc.). The drawer renders it as a small pre block.
  details?: string;
}

// WikiAuditEventListResponse is the envelope returned by
// GET /knowledgebase/{kb_id}/wiki/audit-events.
export interface WikiAuditEventListResponse {
  events: WikiAuditEvent[];
  total: number;
  page: number;
  page_size: number;
  // Source counts so the drawer's filter chips can show per-source
  // totals without a second round-trip.
  source_counts: Record<WikiAuditSource, number>;
  kb_id: string;
}

// WikiAuditFilter mirrors the server's filter struct. `since` is
// RFC3339; the server defaults to now-24h and caps at 90 days.
export interface WikiAuditFilter {
  source?: WikiAuditSource;
  op?: string;
  actor?: string;
  since?: string;
  page?: number;
  page_size?: number;
}

// wikiAuditDefaultSince returns now-24h formatted as RFC3339 with
// second precision. The server clamps to 90 days so a very old
// `since` is auto-corrected — this is just a friendly default.
export function wikiAuditDefaultSince(): string {
  const d = new Date(Date.now() - 24 * 60 * 60 * 1000);
  return d.toISOString();
}