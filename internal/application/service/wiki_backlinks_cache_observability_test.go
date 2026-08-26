package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// obsFakeRepo is an in-memory WikiBacklinksCacheRepository that records
// the Build #23-observable calls (LogInvalidation, Delete, Get, Upsert)
// and tracks the in-process atomic counter snapshots. The repo
// deliberately returns errors for non-Backlinks-cache methods so a
// stray call from a refactor is loud, not silent.
//
// The fake exists alongside the Build #22 cleanup fake — they share
// the same shape but Build #23 needs more methods (LogInvalidation,
// ListInvalidationLog, CountByKB, SumPayloadSizeByKB) and the
// observability helpers care about the audit log + Upsert counters in
// addition to Delete.
type obsFakeRepo struct {
	mu sync.Mutex

	rows map[string]*types.WikiBacklinksCacheRow

	// Recorded audit log entries. Newest at the end (append).
	logEntries []*types.WikiBacklinksCacheInvalidationLogEntry

	// Counters for assertions.
	deleteCalls       int
	deleteLastKBID    string
	deleteLastSlugs   []string
	upsertCalls       int
	upsertLastRow     *types.WikiBacklinksCacheRow
	getCalls          int
	listByKBCalls     int
	countByKBCalls    int
	sumPayloadCalls   int
}

func newObsFakeRepo() *obsFakeRepo {
	return &obsFakeRepo{
		rows:       map[string]*types.WikiBacklinksCacheRow{},
		logEntries: []*types.WikiBacklinksCacheInvalidationLogEntry{},
	}
}

func (f *obsFakeRepo) seed(kbID, slug string, computedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[kbID+"\x00"+slug] = &types.WikiBacklinksCacheRow{
		KbID:       kbID,
		Slug:       slug,
		ComputedAt: computedAt,
		UpdatedAt:  computedAt,
	}
}

// --- Build #23 surface: cache repo methods we care about ---

func (f *obsFakeRepo) Get(_ context.Context, kbID, slug string) (*types.WikiBacklinksCacheRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	row, ok := f.rows[kbID+"\x00"+slug]
	if !ok {
		return nil, nil
	}
	return row, nil
}

func (f *obsFakeRepo) Upsert(_ context.Context, row *types.WikiBacklinksCacheRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertCalls++
	f.upsertLastRow = row
	now := time.Now().UTC()
	row.ComputedAt = now
	row.UpdatedAt = now
	f.rows[row.KbID+"\x00"+row.Slug] = row
	return nil
}

func (f *obsFakeRepo) Delete(_ context.Context, kbID string, slugs []string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	f.deleteLastKBID = kbID
	f.deleteLastSlugs = append([]string{}, slugs...)
	var affected int64
	for _, slug := range slugs {
		if _, ok := f.rows[kbID+"\x00"+slug]; ok {
			delete(f.rows, kbID+"\x00"+slug)
			affected++
		}
	}
	return affected, nil
}

func (f *obsFakeRepo) ListByKB(_ context.Context, kbID string, _, _ int) ([]*types.WikiBacklinksCacheStatus, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listByKBCalls++
	out := make([]*types.WikiBacklinksCacheStatus, 0)
	for _, row := range f.rows {
		if row.KbID == kbID {
			out = append(out, &types.WikiBacklinksCacheStatus{
				Slug:          row.Slug,
				KbID:          row.KbID,
				ComputedAt:    row.ComputedAt,
				UpdatedAt:     row.UpdatedAt,
				SourceEventID: row.SourceEventID,
			})
		}
	}
	return out, int64(len(out)), nil
}

// --- Build #23 surface: LogInvalidation + ListInvalidationLog ---

func (f *obsFakeRepo) LogInvalidation(_ context.Context, entry *types.WikiBacklinksCacheInvalidationLogEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry == nil {
		return errors.New("nil entry")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	// Defensive copy so the caller can't mutate after insert.
	clone := *entry
	f.logEntries = append(f.logEntries, &clone)
	return nil
}

func (f *obsFakeRepo) ListInvalidationLog(_ context.Context, kbID string, _, _ int) ([]*types.WikiBacklinksCacheInvalidationLogEntry, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*types.WikiBacklinksCacheInvalidationLogEntry
	for _, e := range f.logEntries {
		if e.KbID == kbID {
			out = append(out, e)
		}
	}
	return out, int64(len(out)), nil
}

func (f *obsFakeRepo) CountByKB(_ context.Context, kbID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.countByKBCalls++
	var c int64
	for _, r := range f.rows {
		if r.KbID == kbID {
			c++
		}
	}
	return c, nil
}

func (f *obsFakeRepo) SumPayloadSizeByKB(_ context.Context, kbID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sumPayloadCalls++
	var total int64
	for _, r := range f.rows {
		if r.KbID == kbID {
			// Pretend each payload is ~1KB — the test only cares
			// that the count was queried.
			total += 1024
		}
	}
	return total, nil
}

// --- Build #22 surface (unused by these tests but the interface
//     needs them; returning errors makes accidental calls loud). ---

func (f *obsFakeRepo) DeleteStale(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, errors.New("obsFakeRepo.DeleteStale: not used in obs test")
}

func (f *obsFakeRepo) CountRows(_ context.Context) (int64, error) {
	return 0, errors.New("obsFakeRepo.CountRows: not used in obs test")
}

func (f *obsFakeRepo) ListStaleForUpdate(_ context.Context, _ *gorm.DB, _ time.Time, _ int) ([]string, error) {
	return nil, errors.New("obsFakeRepo.ListStaleForUpdate: not used in obs test")
}

// --- tests ---

// TestObsAtomicCounters: wikiCacheObsIncHit/Miss bump both the
// in-process atomic pair AND the Prom counter. The Prom side is
// observable via the public Snapshot() (in-process) only — the Prom
// counters accumulate across the test binary so we don't assert their
// absolute value here.
func TestObsAtomicCounters(t *testing.T) {
	wikiCacheObsReset()
	if got := wikiCacheObsRead(); got.Hits != 0 || got.Misses != 0 {
		t.Fatalf("expected zeroed snapshot, got %+v", got)
	}
	wikiCacheObsIncHit("kb-a")
	wikiCacheObsIncHit("kb-a")
	wikiCacheObsIncMiss("kb-a")
	wikiCacheObsIncError("kb-b")
	wikiCacheObsIncWrite()

	got := wikiCacheObsRead()
	if got.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", got.Hits)
	}
	if got.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", got.Misses)
	}
	// kbLabelFor("") returns "global" — confirm an empty kb_id still
	// bumps the counter (operators get the rate, not a panic).
	wikiCacheObsIncHit("")
	got = wikiCacheObsRead()
	if got.Hits != 3 {
		t.Errorf("expected 3 hits after empty-kb_id bump, got %d", got.Hits)
	}
}

// TestInvalidateCache_LogsAuditRow: every successful InvalidateBacklinksCache
// call writes exactly one row to the audit log. The op label echoes
// the request's Op field; AffectedCount mirrors the Delete return.
func TestInvalidateCache_LogsAuditRow(t *testing.T) {
	repo := newObsFakeRepo()
	repo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
	repo.seed("kb-1", "beta", time.Now().Add(-time.Hour))
	repo.seed("kb-1", "gamma", time.Now().Add(-time.Hour))

	svc := &wikiPageService{cacheRepo: repo}
	svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
		KbID:           "kb-1",
		Op:             types.BacklinkCacheInvalidateUpdate,
		AffectedSlugs:  []string{"alpha", "beta", "gamma"},
	})

	if repo.deleteCalls != 1 {
		t.Errorf("expected 1 Delete call, got %d", repo.deleteCalls)
	}
	if len(repo.logEntries) != 1 {
		t.Fatalf("expected 1 audit log entry, got %d", len(repo.logEntries))
	}
	got := repo.logEntries[0]
	if got.KbID != "kb-1" {
		t.Errorf("audit kb_id = %q, want kb-1", got.KbID)
	}
	if got.Op != string(types.BacklinkCacheInvalidateUpdate) {
		t.Errorf("audit op = %q, want update", got.Op)
	}
	if got.AffectedCount != 3 {
		t.Errorf("audit affected_count = %d, want 3", got.AffectedCount)
	}
	if got.Slug != "alpha" {
		t.Errorf("audit slug = %q, want first-slug \"alpha\"", got.Slug)
	}
	if got.CreatedAt.IsZero() {
		t.Error("audit CreatedAt not stamped")
	}
}

// TestInvalidateCache_LogsAllOpsDistinct: the 8 op labels (7 write ops
// + cleanup_sweep) all flow through to the audit log without
// coalescing — important because operators grep `op = 'X'` to count
// per-write-path invalidations.
func TestInvalidateCache_LogsAllOpsDistinct(t *testing.T) {
	ops := []types.BacklinkCacheInvalidateOp{
		types.BacklinkCacheInvalidateCreate,
		types.BacklinkCacheInvalidateUpdate,
		types.BacklinkCacheInvalidateDelete,
		types.BacklinkCacheInvalidateMove,
		types.BacklinkCacheInvalidateBatchMove,
		types.BacklinkCacheInvalidateBatchDelete,
		types.BacklinkCacheInvalidateBatchStatus,
		types.BacklinkCacheInvalidateSweep,
	}
	for _, op := range ops {
		repo := newObsFakeRepo()
		repo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
		svc := &wikiPageService{cacheRepo: repo}
		svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
			KbID:          "kb-1",
			Op:            op,
			AffectedSlugs: []string{"alpha"},
		})
		if len(repo.logEntries) != 1 {
			t.Fatalf("op %q: expected 1 audit entry, got %d", op, len(repo.logEntries))
		}
		if repo.logEntries[0].Op != string(op) {
			t.Errorf("op %q: audit op = %q", op, repo.logEntries[0].Op)
		}
	}
}

// TestInvalidateCache_EmptySlugsSkipsAudit: a request with zero
// slugs must short-circuit BEFORE bumping any counter or writing a
// log row. Matches the existing Build #21 semantics.
func TestInvalidateCache_EmptySlugsSkipsAudit(t *testing.T) {
	repo := newObsFakeRepo()
	svc := &wikiPageService{cacheRepo: repo}
	svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
		KbID:          "kb-1",
		Op:            types.BacklinkCacheInvalidateUpdate,
		AffectedSlugs: nil,
	})
	if repo.deleteCalls != 0 {
		t.Errorf("expected 0 Delete calls, got %d", repo.deleteCalls)
	}
	if len(repo.logEntries) != 0 {
		t.Errorf("expected 0 audit entries, got %d", len(repo.logEntries))
	}
}

// TestInvalidateCache_XRequestIDPassthrough: when the request context
// carries an X-Request-ID (via types.RequestIDContextKey, set by the
// global middleware.RequestID), the audit row's SourceEventID echoes
// that id. This is the operator-correlation invariant.
func TestInvalidateCache_XRequestIDPassthrough(t *testing.T) {
	repo := newObsFakeRepo()
	repo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
	svc := &wikiPageService{cacheRepo: repo}

	const rid = "test-req-abc-123"
	ctx := context.WithValue(context.Background(), types.RequestIDContextKey, rid)
	svc.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
		KbID:          "kb-1",
		Op:            types.BacklinkCacheInvalidateCreate,
		AffectedSlugs: []string{"alpha"},
	})

	if len(repo.logEntries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.logEntries))
	}
	if repo.logEntries[0].SourceEventID != rid {
		t.Errorf("audit source_event_id = %q, want %q", repo.logEntries[0].SourceEventID, rid)
	}
}

// TestSourceEventID_FallsBackToRequestID: when the wiki-specific
// key is absent but the canonical request id key IS set (the
// middleware path), the fallback returns the request id. Empty /
// absent on both keys returns "".
func TestSourceEventID_FallsBackToRequestID(t *testing.T) {
	const rid = "rid-fallback-456"
	ctx := context.WithValue(context.Background(), types.RequestIDContextKey, rid)
	if got := wikiSourceEventIDFromContext(ctx); got != rid {
		t.Errorf("fallback = %q, want %q", got, rid)
	}
	if got := wikiSourceEventIDFromContext(context.Background()); got != "" {
		t.Errorf("empty fallback = %q, want \"\"", got)
	}
	// Explicit wiki key wins over request id.
	explicit := "wiki-explicit-789"
	ctx2 := WithWikiObsSourceEventID(ctx, explicit)
	if got := wikiSourceEventIDFromContext(ctx2); got != explicit {
		t.Errorf("explicit = %q, want %q", got, explicit)
	}
}

// TestSweeper_LogsCleanupSweepAuditRow: the Build #22 sweeper's
// per-batch DeleteStale writes one audit row with op=cleanup_sweep,
// kb_id=system, slug=*, and the affected_count mirroring the
// deleted-row count. The audit log is the bridge that lets
// Build #23 operators see "the sweeper ran and cleaned N rows" in
// the same view as the write-path invalidations.
func TestSweeper_LogsCleanupSweepAuditRow(t *testing.T) {
	repo := newObsFakeRepo()
	now := time.Now().UTC()
	for i, slug := range []string{"alpha", "beta", "gamma", "delta"} {
		repo.seed("kb-1", slug, now.Add(-time.Duration(40+i)*24*time.Hour))
	}

	svc := NewWikiBacklinksCacheCleanupService(
		WikiBacklinksCacheCleanupConfig{
			TTL:       30 * 24 * time.Hour,
			Period:    24 * time.Hour,
			BatchSize: 100,
			DryRun:    false,
			MaxRows:   1_000_000,
		},
		nil, // DB unused — fake handles DeleteStale in-memory
		repo,
	)

	deleted, _, _, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if deleted != 4 {
		t.Errorf("expected 4 deleted rows, got %d", deleted)
	}
	if len(repo.logEntries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.logEntries))
	}
	got := repo.logEntries[0]
	if got.Op != string(types.BacklinkCacheInvalidateSweep) {
		t.Errorf("op = %q, want cleanup_sweep", got.Op)
	}
	if got.KbID != "system" {
		t.Errorf("kb_id = %q, want system", got.KbID)
	}
	if got.Slug != "*" {
		t.Errorf("slug = %q, want *", got.Slug)
	}
	if got.AffectedCount != 4 {
		t.Errorf("affected_count = %d, want 4", got.AffectedCount)
	}
	if got.SourceEventID != "" {
		t.Errorf("sweeper source_event_id = %q, want \"\" (cron has no request id)", got.SourceEventID)
	}
	if got.ActorUserID != nil {
		t.Errorf("sweeper actor_user_id = %v, want nil", got.ActorUserID)
	}
}

// TestSweeper_DryRunSkipsAuditRow: dry-run mode is non-destructive —
// the sweep count is reported but NO audit row should appear because
// nothing was actually invalidated.
func TestSweeper_DryRunSkipsAuditRow(t *testing.T) {
	repo := newObsFakeRepo()
	now := time.Now().UTC()
	for i, slug := range []string{"alpha", "beta"} {
		repo.seed("kb-1", slug, now.Add(-time.Duration(40+i)*24*time.Hour))
	}

	svc := NewWikiBacklinksCacheCleanupService(
		WikiBacklinksCacheCleanupConfig{
			TTL:       30 * 24 * time.Hour,
			BatchSize: 100,
			DryRun:    true,
		},
		nil,
		repo,
	)

	_, dryRun, _, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !dryRun {
		t.Error("expected dryRun=true")
	}
	if len(repo.logEntries) != 0 {
		t.Errorf("dry-run should not log audit rows, got %d", len(repo.logEntries))
	}
}

// TestListBacklinksCacheStatuses_AggregatesFromRepo: the new admin
// service method returns the rollup envelope with the four aggregate
// fields populated. Best-effort aggregates (CountByKB,
// SumPayloadSizeByKB) succeed via the fake; the in-process hit ratio
// starts at 0 until something bumps it.
func TestListBacklinksCacheStatuses_AggregatesFromRepo(t *testing.T) {
	repo := newObsFakeRepo()
	now := time.Now().UTC()
	repo.seed("kb-1", "alpha", now.Add(-time.Hour))
	repo.seed("kb-1", "beta", now.Add(-2*time.Hour))
	repo.seed("kb-2", "gamma", now.Add(-3*time.Hour))

	svc := &wikiPageService{cacheRepo: repo}
	resp, err := svc.ListBacklinksCacheStatuses(context.Background(), "kb-1", 50, 0)
	if err != nil {
		t.Fatalf("ListBacklinksCacheStatuses: %v", err)
	}
	if resp.KbID != "kb-1" {
		t.Errorf("kb_id = %q, want kb-1", resp.KbID)
	}
	if resp.RowCount != 2 {
		t.Errorf("row_count = %d, want 2", resp.RowCount)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if resp.PayloadSizeBytes != 2*1024 {
		t.Errorf("payload_size_bytes = %d, want 2048", resp.PayloadSizeBytes)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.KbID != "kb-1" {
			t.Errorf("item kb_id = %q, want kb-1", item.KbID)
		}
	}
	if repo.countByKBCalls != 1 {
		t.Errorf("expected 1 CountByKB call, got %d", repo.countByKBCalls)
	}
	if repo.sumPayloadCalls != 1 {
		t.Errorf("expected 1 SumPayloadSizeByKB call, got %d", repo.sumPayloadCalls)
	}
}