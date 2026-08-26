package types

import "time"

// Build #24 — unified wiki audit DTO + source enum.
//
// WeKnora has four wiki-related audit surfaces (Build #14
// wiki_batch_job_audit, Build #23 wiki_backlinks_cache_invalidation_log,
// migration 000091 wiki_page_acl_audit, and the existing KB-scoped
// projection into audit_logs from migration 000044+000073). Each has its
// own column family and there is no single endpoint that lets operators
// ask "what happened in this KB in the last 24 hours?" Build #24 adds a
// code-level merge service that fans out to all four repositories and
// normalizes the rows into WikiAuditEvent. The DTO below is the wire
// shape returned by GET /api/v1/knowledgebase/:kb_id/wiki/audit-events.
//
// Why a DTO and not four separate endpoints: the four source schemas
// diverge (slug exists in 2/4, actor is TEXT/BIGINT/VARCHAR across all
// four, the invalidation log is the only one with source_event_id).
// Building an SQL UNION view would need three dialect variants and
// could not reconcile the actor types. A code-level merge at the service
// layer keeps dialect neutrality and gives us a single join shape
// (`kb_id, timestamp DESC`) that operators can grep against.

// WikiAuditSource names the table a WikiAuditEvent row originated from.
// The set is closed — adding a fifth source means a new Build with a new
// constant; operators tooling depends on the string value.
type WikiAuditSource string

const (
	// WikiAuditSourceActivity is the KB-scope projection into audit_logs.
	// Rows arrive via recordKBActivity in kb_activity.go and cover wiki
	// reads/writes that opted into activity-stream surfacing (chat
	// messages, KB share, FAQ changes, etc).
	WikiAuditSourceActivity WikiAuditSource = "audit_logs"

	// WikiAuditSourceBatchJobAudit is the Build #14 table for
	// batch_job enqueue/start/finish/undo/cancel/expire events. The
	// slug is omitted by design — each row is job-level, not per-slug;
	// operators correlate to per-slug outcomes via wiki_batch_jobs and
	// the per-slug failure ledger added by Build #15.
	WikiAuditSourceBatchJobAudit WikiAuditSource = "wiki_batch_job_audit"

	// WikiAuditSourceBacklinksInvalidation is the Build #23 table for
	// every cache wipe event. This is the only source with a guaranteed
	// source_event_id (X-Request-ID from middleware.RequestID()).
	WikiAuditSourceBacklinksInvalidation WikiAuditSource = "wiki_backlinks_cache_invalidation_log"

	// WikiAuditSourcePageAclAudit is the migration 000091 table for
	// per-page ACL changes. It was write-only until Build #24 added the
	// first read path.
	WikiAuditSourcePageAclAudit WikiAuditSource = "wiki_page_acl_audit"
)

// AllWikiAuditSources is the canonical ordering used by ListAuditEvents
// to populate the source-counts envelope. New sources go at the end so
// API consumers can rely on stable indices.
var AllWikiAuditSources = []WikiAuditSource{
	WikiAuditSourceActivity,
	WikiAuditSourceBatchJobAudit,
	WikiAuditSourceBacklinksInvalidation,
	WikiAuditSourcePageAclAudit,
}

// WikiAuditActorKind normalises the heterogeneous actor columns into a
// stable enum for UI filter chips and group-by queries.
type WikiAuditActorKind string

const (
	// WikiAuditActorUser is set when the actor is a real user — the
	// audit row carries a non-empty actor_user_id (and not the literal
	// "system" sentinel).
	WikiAuditActorUser WikiAuditActorKind = "user"

	// WikiAuditActorSystem is set when the actor is a backend service,
	// the API key owner, or the literal "system" sentinel — any flow
	// that bypasses a human user.
	WikiAuditActorSystem WikiAuditActorKind = "system"

	// WikiAuditActorSweep is reserved for the Build #22 cleanup sweep,
	// which stamps itself with kb_id="system" + slug="*" + op=
	// "cleanup_sweep". Splitting it out of "system" makes the
	// sweeper-driven invalidations easy to spot in dashboards without
	// joining on op.
	WikiAuditActorSweep WikiAuditActorKind = "sweep"
)

// WikiAuditEvent is the unified wire shape returned by the merge
// service. Each row corresponds to one row in one source table; the
// fields below are the normalised projection so consumers don't have to
// know which source a row came from.
//
// Field semantics:
//
//   - ID: source-prefixed synthetic id ("wl:42", "ba:7", "il:123",
//     "al:901"). The prefix is the lowercase first letter of the source
//     enum ("a" for audit_logs, "b" for batch_job_audit, "i" for
//     invalidation log, "l" for ACL audit) — easy to grep. The numeric
//     suffix is the source row's primary key.
//   - Timestamp: the source's "when did this happen" column. Three of
//     four sources use created_at; wiki_batch_job_audit uses
//     occurred_at. The merge service maps both.
//   - KbID: every source has this column, possibly under different names
//     (knowledge_base_id for batch + ACL audit). Always populated.
//   - Slug: present for two sources (invalidation log + ACL audit),
//     empty for the others. omitempty on the wire.
//   - Op: the source's action label, normalised to a single string. For
//     the invalidation log this is the BacklinkCacheInvalidateOp value;
//     for ACL audit this is actionForMode's output (set_inherit /
//     set_private / set_allow_list).
//   - Source: one of the WikiAuditSource constants.
//   - Actor: stringified regardless of source column type — UUID
//     strings pass through, BIGINTs go through strconv.FormatUint.
//   - ActorKind: derived from the actor value + op column.
//   - SourceEventID: present for the invalidation log, omitempty elsewhere.
//   - AffectedCount: present for the invalidation log (the wipe row
//     count), omitempty elsewhere.
//   - Details: free-form per-source payload. The merge service parses
//     the invalidation log's Details JSON string into a map and leaves
//     the others as-is (the activity-feed rows already carry details as
//     JSONB, the batch/ACL audit rows do too).
type WikiAuditEvent struct {
	ID            string         `json:"id"`
	Timestamp     time.Time      `json:"timestamp"`
	KbID          string         `json:"kb_id"`
	Slug          string         `json:"slug,omitempty"`
	Op            string         `json:"op"`
	Source        WikiAuditSource `json:"source"`
	Actor         string         `json:"actor"`
	ActorKind     WikiAuditActorKind `json:"actor_kind"`
	SourceEventID string         `json:"source_event_id,omitempty"`
	AffectedCount int            `json:"affected_count,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
}

// WikiAuditEventListResponse is the envelope returned by
// GET /api/v1/knowledgebase/:kb_id/wiki/audit-events. The SourceCounts
// field lets a UI render a 4-chip "by source" filter group without a
// second round-trip.
type WikiAuditEventListResponse struct {
	KbID         string                  `json:"kb_id"`
	Page         int                     `json:"page"`
	PageSize     int                     `json:"page_size"`
	Total        int64                   `json:"total"`
	Events       []*WikiAuditEvent       `json:"events"`
	SourceCounts map[WikiAuditSource]int64 `json:"source_counts"`
}

// WikiAuditFilter is the parsed query-string view of the endpoint's
// filter parameters. The service builds one of these per request and
// passes it down to each repo fan-out. Source and Op are optional; nil
// means "include all rows". Since is optional and zero means "no lower
// bound" — the service defaults it to now - 24h on the handler side.
type WikiAuditFilter struct {
	Source   WikiAuditSource
	Op       string
	Actor    string
	Since    time.Time
	Until    time.Time
	Page     int
	PageSize int
}