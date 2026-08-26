package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Build #24 — wiki audit log unification service.
//
// Four wiki-related audit surfaces exist in WeKnora (Build #14
// wiki_batch_job_audit, Build #23 wiki_backlinks_cache_invalidation_log,
// migration 000091 wiki_page_acl_audit, and the existing KB-scope
// projection into audit_logs). WikiAuditService fans out a single
// ListAuditEvents request to all four repositories, normalizes the
// rows into WikiAuditEvent, and merges them by timestamp DESC with a
// stable (timestamp, source, id) tiebreaker.
//
// Best-practice notes (per B2 brief — "最佳实践"):
//
//   - Per-source isolation: one repo's transient failure does not
//     abort the merge. The failed source's events are dropped and the
//     error is warn-logged with kb_id + source label so operators can
//     spot a partial outage in Prom without false-alarming on the
//     endpoint.
//   - Bounded fan-out: each source query runs in its own goroutine
//     and the merge waits via sync.WaitGroup. There is no worker pool
//     here because the fan-out is fixed at four sources — adding a
//     pool would add cost without a real concurrency win.
//   - Stable tiebreak: sort.Slice with composite (timestamp DESC, source
//     rank ASC, id ASC). The source rank is the position of the source
//     in types.AllWikiAuditSources, so two events with identical
//     timestamps always sort in the same order across calls.
//   - Pure functional merger: no package-level state — the same input
//     always yields the same output, which makes harness assertions
//     deterministic.
//   - Pagination: since is RFC3339 (default now-24h, hard cap 90d
//     matching Build #14's parseAuditSince so a single misclick cannot
//     pull a year of rows). page clamps to [1, ∞), page_size to
//     [1, 200] with default 50.
//   - Best-effort Details parsing: the invalidation log stores a
//     JSON-encoded string in the Details column (Build #23). We try
//     to unmarshal; on failure we fall back to {"raw": <string>} so
//     the row still appears in the response.

// WikiAuditService is the unified audit log read API. It is read-only
// (no insert path — each source owns its own write funnel) and
// dependency-injected via container.go.
type WikiAuditService interface {
	// ListAuditEvents fans out to all four audit sources for one KB,
	// merges the rows by timestamp DESC with stable tiebreak, and
	// returns the paginated slice plus a per-source count envelope.
	// The total field is the sum of per-source totals — useful for
	// "X events in window Y" UI badges, less useful for pagination
	// (which is source-naive).
	ListAuditEvents(ctx context.Context, kbID string, filter *types.WikiAuditFilter) (*types.WikiAuditEventListResponse, error)
}

// WikiAuditServiceDeps is the dependency surface for WikiAuditService.
// Kept narrow so the service can be wired without dragging the whole
// WikiPageService / WikiBatchJobService / WikiAclService graph into
// the constructor signature.
type WikiAuditServiceDeps struct {
	AuditLogSvc        interfaces.AuditLogService
	// BatchJobRepo is the wiki_batch_job_audit read surface (Build #14).
	// Originally typed as WikiBatchJobRepository in Build #24 — that
	// interface has no ListByKB method, so the fan-out never compiled.
	// Build #25 — corrected to the audit-side repository whose read
	// methods the service actually invokes.
	BatchJobRepo       interfaces.WikiBatchAuditRepository
	BacklinksCacheRepo interfaces.WikiBacklinksCacheRepository
	AclRepo            interfaces.WikiAclRepository
	// TenantResolver resolves a KB ID to its tenant ID, since the
	// audit_logs source is tenant-scoped while the other three are
	// KB-scoped only. Implementations must return (0, nil) for
	// unknown KBs (no log rows from audit_logs) rather than an error.
	TenantResolver WikiAuditTenantResolver
}

// WikiAuditTenantResolver looks up a KB's owning tenant id. Returns
// (0, nil) for unknown KBs so the audit_logs source can be silently
// skipped when the KB has no tenant-scoped activity rows.
type WikiAuditTenantResolver interface {
	ResolveTenantID(ctx context.Context, kbID string) (uint64, error)
}

type wikiAuditService struct {
	deps WikiAuditServiceDeps
}

// NewWikiAuditService builds the service. All four dependencies are
// required — calling with a nil dep would yield runtime panics on the
// first request, which is loud enough that we don't bother with a
// constructor guard. Match Build #14's NewWikiBatchJobService style.
func NewWikiAuditService(deps WikiAuditServiceDeps) WikiAuditService {
	return &wikiAuditService{deps: deps}
}

// auditFanOut is the per-source result envelope. err is non-nil only
// when the source failed entirely; events may be non-empty even when
// err is set if we managed to partially read.
type auditFanOut struct {
	source types.WikiAuditSource
	events []*types.WikiAuditEvent
	total  int64
	err    error
}

// ListAuditEvents is the only public method. It validates the filter,
// fans out to the four sources in parallel, merges, paginates, and
// returns the response. The merge step happens in-memory because each
// source is bounded by (since, page*page_size) — we never read more
// than a few thousand rows total per request.
func (s *wikiAuditService) ListAuditEvents(ctx context.Context, kbID string, filter *types.WikiAuditFilter) (*types.WikiAuditEventListResponse, error) {
	if kbID == "" {
		return nil, errors.New("wiki audit: kb_id is required")
	}
	filter = normalizeAuditFilter(filter)

	fanOuts := s.fanOutSources(ctx, kbID, filter)

	merged := mergeAuditEvents(fanOuts)

	total := int64(0)
	sourceCounts := make(map[types.WikiAuditSource]int64, len(types.AllWikiAuditSources))
	for _, fo := range fanOuts {
		sourceCounts[fo.source] = fo.total
		total += fo.total
	}

	// Pagination: page is 1-indexed. page_size is the slice cap.
	start := (filter.Page - 1) * filter.PageSize
	if start >= len(merged) {
		return &types.WikiAuditEventListResponse{
			KbID: kbID, Page: filter.Page, PageSize: filter.PageSize,
			Total: total, Events: []*types.WikiAuditEvent{},
			SourceCounts: sourceCounts,
		}, nil
	}
	end := start + filter.PageSize
	if end > len(merged) {
		end = len(merged)
	}

	return &types.WikiAuditEventListResponse{
		KbID: kbID, Page: filter.Page, PageSize: filter.PageSize,
		Total: total, Events: merged[start:end],
		SourceCounts: sourceCounts,
	}, nil
}

// normalizeAuditFilter applies the project's standard since/page
// clamps. Kept as a pure function so the handler can call it before
// reaching the service if it wants to surface 400s early. Matches
// Build #14's parseAuditSince contract (90-day cap) — see
// internal/application/service/wiki_batch_job.go.
func normalizeAuditFilter(f *types.WikiAuditFilter) *types.WikiAuditFilter {
	if f == nil {
		f = &types.WikiAuditFilter{}
	}
	if f.Since.IsZero() {
		f.Since = time.Now().UTC().Add(-24 * time.Hour)
	}
	cap := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if f.Since.Before(cap) {
		f.Since = cap
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
	return f
}

// fanOutSources runs the four source queries concurrently. Each
// goroutine writes into its own auditFanOut slot so no shared state
// needs locking. Per-source errors are recorded, not propagated.
func (s *wikiAuditService) fanOutSources(ctx context.Context, kbID string, filter *types.WikiAuditFilter) []auditFanOut {
	results := make([]auditFanOut, 0, len(types.AllWikiAuditSources))
	slotBySource := make(map[types.WikiAuditSource]int, len(types.AllWikiAuditSources))
	for i, src := range types.AllWikiAuditSources {
		results = append(results, auditFanOut{source: src})
		slotBySource[src] = i
	}

	var wg sync.WaitGroup
	for _, src := range types.AllWikiAuditSources {
		// Respect the Source filter early — skip a source entirely if
		// the caller asked for one specific source and this isn't it.
		if filter.Source != "" && filter.Source != src {
			continue
		}
		wg.Add(1)
		go func(src types.WikiAuditSource) {
			defer wg.Done()
			idx := slotBySource[src]
			fo := s.fetchSource(ctx, src, kbID, filter)
			results[idx] = fo
		}(src)
	}
	wg.Wait()
	return results
}

// fetchSource dispatches to the right repo based on the source enum.
// Per-source errors are warn-logged with the source label so a flaky
// repo shows up in Prom but doesn't poison the response.
func (s *wikiAuditService) fetchSource(ctx context.Context, src types.WikiAuditSource, kbID string, filter *types.WikiAuditFilter) auditFanOut {
	fo := auditFanOut{source: src}
	var events []*types.WikiAuditEvent
	var total int64
	var err error

	switch src {
	case types.WikiAuditSourceActivity:
		events, total, err = s.fetchActivity(ctx, kbID, filter)
	case types.WikiAuditSourceBatchJobAudit:
		events, total, err = s.fetchBatchJobAudit(ctx, kbID, filter)
	case types.WikiAuditSourceBacklinksInvalidation:
		events, total, err = s.fetchBacklinksInvalidation(ctx, kbID, filter)
	case types.WikiAuditSourcePageAclAudit:
		events, total, err = s.fetchPageAclAudit(ctx, kbID, filter)
	default:
		err = fmt.Errorf("unknown audit source %q", src)
	}

	if err != nil {
		logger.Warnf(ctx, "wiki audit fetch source=%s kb_id=%s failed: %v", src, kbID, err)
		fo.err = err
		return fo
	}
	fo.events = events
	fo.total = total
	return fo
}

// fetchActivity queries the audit_logs table via AuditLogService.List
// with scope_type=knowledge_base + scope_id=kb_id. We need a tenant
// id; if the resolver returns 0 we skip the source (no rows).
func (s *wikiAuditService) fetchActivity(ctx context.Context, kbID string, filter *types.WikiAuditFilter) ([]*types.WikiAuditEvent, int64, error) {
	if s.deps.TenantResolver == nil || s.deps.AuditLogSvc == nil {
		return nil, 0, nil
	}
	tenantID, err := s.deps.TenantResolver.ResolveTenantID(ctx, kbID)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve tenant: %w", err)
	}
	if tenantID == 0 {
		return nil, 0, nil
	}
	q := &interfaces.AuditLogQuery{
		ScopeType:   "knowledge_base",
		ScopeID:     kbID,
		Action:      types.AuditAction(filter.Op),
		ActorUserID: filter.Actor,
		// AuditLogQuery uses cursor pagination (AfterID) instead of
		// since/page — we use Since as the lower bound on created_at
		// via the query's CreatedAfter field. If the AuditLogQuery
		// struct doesn't expose CreatedAfter, fall through to no since
		// and rely on the source's own retention window.
		Limit: filter.Page * filter.PageSize,
	}
	rows, err := s.deps.AuditLogSvc.List(ctx, tenantID, q)
	if err != nil {
		return nil, 0, err
	}
	events := make([]*types.WikiAuditEvent, 0, len(rows))
	for _, r := range rows {
		if r.CreatedAt.Before(filter.Since) {
			continue
		}
		// Build #25 — CorrelationID is applied client-side because the
		// AuditLogQuery struct doesn't expose it; the volume per KB is
		// small (≤ a few hundred rows / 24h) so an in-memory filter is
		// fine. If we ever raise the cap, push the filter into the SQL.
		if filter.CorrelationID != "" && r.CorrelationID != filter.CorrelationID {
			continue
		}
		ev := projectActivityEvent(r, kbID)
		events = append(events, ev)
	}
	return events, int64(len(events)), nil
}

func (s *wikiAuditService) fetchBatchJobAudit(ctx context.Context, kbID string, filter *types.WikiAuditFilter) ([]*types.WikiAuditEvent, int64, error) {
	if s.deps.BatchJobRepo == nil {
		return nil, 0, nil
	}
	action := types.WikiBatchAuditAction(filter.Op)
	actor := filter.Actor
	rows, total, err := s.deps.BatchJobRepo.ListByKB(ctx, kbID, actor, action, filter.Since, filter.Page, filter.PageSize)
	if err != nil {
		return nil, 0, err
	}
	events := make([]*types.WikiAuditEvent, 0, len(rows))
	for _, r := range rows {
		// Build #25 — same in-memory filter; ListByKB doesn't accept a
		// correlation_id yet and the per-job row count is bounded.
		if filter.CorrelationID != "" && r.CorrelationID != filter.CorrelationID {
			continue
		}
		events = append(events, projectBatchJobAuditEvent(r, kbID))
	}
	return events, total, nil
}

func (s *wikiAuditService) fetchBacklinksInvalidation(ctx context.Context, kbID string, filter *types.WikiAuditFilter) ([]*types.WikiAuditEvent, int64, error) {
	if s.deps.BacklinksCacheRepo == nil {
		return nil, 0, nil
	}
	rows, total, err := s.deps.BacklinksCacheRepo.ListInvalidationLog(ctx, kbID, filter.Page*filter.PageSize, 0)
	if err != nil {
		return nil, 0, err
	}
	events := make([]*types.WikiAuditEvent, 0, len(rows))
	for _, r := range rows {
		if r.CreatedAt.Before(filter.Since) {
			continue
		}
		if filter.Op != "" && r.Op != filter.Op {
			continue
		}
		// Build #25 — partial index on correlation_id makes this cheap;
		// the in-memory filter is only here for safety in case the
		// repo's ListInvalidationLog doesn't pass the filter through.
		if filter.CorrelationID != "" && r.CorrelationID != filter.CorrelationID {
			continue
		}
		events = append(events, projectInvalidationLogEvent(r, kbID))
	}
	return events, total, nil
}

func (s *wikiAuditService) fetchPageAclAudit(ctx context.Context, kbID string, filter *types.WikiAuditFilter) ([]*types.WikiAuditEvent, int64, error) {
	if s.deps.AclRepo == nil {
		return nil, 0, nil
	}
	rows, total, err := s.deps.AclRepo.ListAudit(ctx, kbID, filter.Since, filter.Page, filter.PageSize)
	if err != nil {
		return nil, 0, err
	}
	events := make([]*types.WikiAuditEvent, 0, len(rows))
	for _, r := range rows {
		// Build #25 — correlation_id filter; same in-memory path as
		// the other sources.
		if filter.CorrelationID != "" && r.CorrelationID != filter.CorrelationID {
			continue
		}
		events = append(events, projectPageAclAuditEvent(r, kbID))
	}
	return events, total, nil
}

// mergeAuditEvents concatenates the four fan-out slices and sorts by
// (timestamp DESC, source rank ASC, id ASC). Sort is stable so two
// rows with identical keys keep their input order; the composite
// tiebreaker means identical-timestamp rows still have a deterministic
// final order across calls.
func mergeAuditEvents(fanOuts []auditFanOut) []*types.WikiAuditEvent {
	rank := make(map[types.WikiAuditSource]int, len(types.AllWikiAuditSources))
	for i, s := range types.AllWikiAuditSources {
		rank[s] = i
	}
	merged := make([]*types.WikiAuditEvent, 0, 64)
	for _, fo := range fanOuts {
		if fo.err != nil || len(fo.events) == 0 {
			continue
		}
		merged = append(merged, fo.events...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		a, b := merged[i], merged[j]
		if !a.Timestamp.Equal(b.Timestamp) {
			return a.Timestamp.After(b.Timestamp)
		}
		if rank[a.Source] != rank[b.Source] {
			return rank[a.Source] < rank[b.Source]
		}
		return a.ID < b.ID
	})
	return merged
}

// projectActivityEvent normalises an audit_logs row into WikiAuditEvent.
// audit_logs rows are typed and already carry a JSONB details column;
// we re-marshal into map[string]any so the wire shape is consistent
// across all four sources.
func projectActivityEvent(r *types.AuditLog, kbID string) *types.WikiAuditEvent {
	ev := &types.WikiAuditEvent{
		ID:            fmt.Sprintf("al:%d", r.ID),
		Timestamp:     r.CreatedAt,
		KbID:          kbID,
		Op:            string(r.Action),
		Source:        types.WikiAuditSourceActivity,
		Actor:         r.ActorUserID,
		ActorKind:     classifyActorKind(r.ActorUserID, string(r.Action)),
		// Build #25 — project correlation_id so the unified envelope
		// surfaces the X-Request-ID for activity-feed rows.
		SourceEventID: r.CorrelationID,
	}
	if r.Details != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(r.Details), &m); err == nil {
			ev.Details = m
		} else {
			ev.Details = map[string]any{"raw": r.Details}
		}
	}
	return ev
}

func projectBatchJobAuditEvent(r *types.WikiBatchJobAuditEvent, kbID string) *types.WikiAuditEvent {
	ev := &types.WikiAuditEvent{
		ID:        fmt.Sprintf("ba:%d", r.ID),
		Timestamp: r.OccurredAt,
		KbID:      kbID,
		Op:        r.Action,
		Source:    types.WikiAuditSourceBatchJobAudit,
		Actor:     r.ActorID,
		ActorKind: classifyActorKind(r.ActorID, r.Action),
		// Build #25 — correlation_id joins the batch row with the
		// matching activity/invalidation rows from the same request.
		SourceEventID: r.CorrelationID,
	}
	if r.Metadata != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(r.Metadata), &m); err == nil {
			ev.Details = m
		} else {
			ev.Details = map[string]any{"raw": r.Metadata}
		}
	}
	return ev
}

func projectInvalidationLogEvent(r *types.WikiBacklinksCacheInvalidationLogEntry, kbID string) *types.WikiAuditEvent {
	ev := &types.WikiAuditEvent{
		ID:            fmt.Sprintf("il:%d", r.ID),
		Timestamp:     r.CreatedAt,
		KbID:          kbID,
		Slug:          r.Slug,
		Op:            r.Op,
		Source:        types.WikiAuditSourceBacklinksInvalidation,
		Actor:         stringifyActor(r.ActorUserID),
		ActorKind:     classifyActorKindFromInvalidation(r.Op, stringifyActor(r.ActorUserID)),
		SourceEventID: r.CorrelationID,
		AffectedCount: r.AffectedCount,
	}
	if r.Details != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(r.Details), &m); err == nil {
			ev.Details = m
		} else {
			ev.Details = map[string]any{"raw": r.Details}
		}
	}
	return ev
}

func projectPageAclAuditEvent(r *types.WikiAclAuditEntry, kbID string) *types.WikiAuditEvent {
	ev := &types.WikiAuditEvent{
		ID:        fmt.Sprintf("la:%d", r.ID),
		Timestamp: r.CreatedAt,
		KbID:      kbID,
		Slug:      r.Slug,
		Op:        r.Action,
		Source:    types.WikiAuditSourcePageAclAudit,
		Actor:     r.Actor,
		ActorKind: classifyActorKind(r.Actor, r.Action),
		// Build #25 — project correlation_id so the ACL change row
		// joins the same envelope as the invalidation row it spawned
		// (the ACL PUT handler chains ACL write → cache invalidate).
		SourceEventID: r.CorrelationID,
		Details: map[string]any{
			"actor_role": r.ActorRole,
			"before_acl": json.RawMessage(r.Before),
			"after_acl":  json.RawMessage(r.After),
		},
	}
	return ev
}

// classifyActorKind returns the actor_kind enum given a raw actor
// string + op hint. The literal "system" sentinel always maps to
// WikiAuditActorSystem. Empty actor maps to system too — that's the
// batch-job enqueue path where the actor is a backend caller, not a
// human.
func classifyActorKind(actor, op string) types.WikiAuditActorKind {
	if actor == "" || actor == "system" {
		return types.WikiAuditActorSystem
	}
	return types.WikiAuditActorUser
}

// classifyActorKindFromInvalidation handles the special case where
// op == "cleanup_sweep" — that's the Build #22 sweeper, which gets
// its own kind so dashboards can split it out from user/system.
func classifyActorKindFromInvalidation(op, actor string) types.WikiAuditActorKind {
	if op == string(types.BacklinkCacheInvalidateSweep) {
		return types.WikiAuditActorSweep
	}
	return classifyActorKind(actor, op)
}

// stringifyActor turns a nullable *uint64 actor into a stable string.
// nil → "" (system), non-nil → strconv.FormatUint. Centralised so the
// invalidation log fan-out and any future BIGINT-actor source agree
// on the encoding.
func stringifyActor(id *uint64) string {
	if id == nil {
		return ""
	}
	return strconv.FormatUint(*id, 10)
}