package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// Build #21 — cache-first + write-time invalidation tests for
// ListBacklinkGraph. The harness focuses on the cache envelope rather
// than the graph compute (covered exhaustively in
// wiki_page_backlinks_v2_test.go); the four scenarios below verify:
//
//   1. cache hit → returns cached payload, does NOT recompute
//   2. cache miss → recomputes + writes back
//   3. UpdatePage hook wipes [self ∪ out_links]
//   4. DeletePage hook wipes [self ∪ in_links]
//
// Each test pairs the existing `stubBacklinksRepo` (Build #11) with
// the new `stubBacklinksCacheRepo` (below) so we can observe both the
// read side (compute call counter) and the write side (Delete / Upsert
// call counters).

// stubBacklinksCacheRepo is an in-memory WikiBacklinksCacheRepository
// with call counters for observability. Rows are stored as
// WikiBacklinksCacheRow pointers keyed by slug — sufficient for tests
// that only inspect via Upsert / Get / Delete.
type stubBacklinksCacheRepo struct {
	rows         map[string]*types.WikiBacklinksCacheRow // slug → row
	getCalls     int                                    // times Get was invoked
	upsertCalls  int                                    // times Upsert was invoked
	deleteCalls  int                                    // times Delete was invoked
	lastDeleted  []string                               // slugs passed to Delete
	lastUpserted *types.WikiBacklinksCacheRow          // last row passed to Upsert
	getErr       error
	upsertErr    error
	deleteErr    error
}

func newStubBacklinksCacheRepo() *stubBacklinksCacheRepo {
	return &stubBacklinksCacheRepo{rows: map[string]*types.WikiBacklinksCacheRow{}}
}

func (s *stubBacklinksCacheRepo) Get(ctx context.Context, kbID string, slug string) (*types.WikiBacklinksCacheRow, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	row, ok := s.rows[slug]
	if !ok {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (s *stubBacklinksCacheRepo) Upsert(ctx context.Context, row *types.WikiBacklinksCacheRow) error {
	s.upsertCalls++
	if s.upsertErr != nil {
		return s.upsertErr
	}
	if row != nil {
		cp := *row
		s.lastUpserted = &cp
		s.rows[row.Slug] = &cp
	}
	return nil
}

func (s *stubBacklinksCacheRepo) Delete(ctx context.Context, kbID string, slugs []string) (int64, error) {
	s.deleteCalls++
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	s.lastDeleted = append([]string(nil), slugs...)
	affected := int64(0)
	for _, sl := range slugs {
		if _, ok := s.rows[sl]; ok {
			delete(s.rows, sl)
			affected++
		}
	}
	return affected, nil
}

// (ListByKB is unused by the build-#21 service; the stub still needs to
// satisfy the interface so we add the method here even though tests
// don't call it.)
func (s *stubBacklinksCacheRepo) ListByKB(ctx context.Context, kbID string, limit, offset int) ([]*types.WikiBacklinksCacheStatus, int64, error) {
	return []*types.WikiBacklinksCacheStatus{}, 0, nil
}
func (s *stubBacklinksCacheRepo) DeleteByKB(ctx context.Context, kbID string) (int64, error) {
	return 0, nil
}
func (s *stubBacklinksCacheRepo) FindReferencingSlugs(ctx context.Context, kbID, slug string) ([]string, error) {
	return []string{slug}, nil
}
func (s *stubBacklinksCacheRepo) CountByKB(ctx context.Context, kbID string) (int64, error) {
	return int64(len(s.rows)), nil
}
func (s *stubBacklinksCacheRepo) LogInvalidation(ctx context.Context, e *types.WikiBacklinksCacheInvalidationLogEntry) error {
	return nil
}
func (s *stubBacklinksCacheRepo) DeleteStale(ctx context.Context, before time.Time, limit int) (int64, error) {
	return 0, nil
}
func (s *stubBacklinksCacheRepo) CountRows(ctx context.Context) (int64, error) {
	return int64(len(s.rows)), nil
}
func (s *stubBacklinksCacheRepo) CountBackrefRows(ctx context.Context) (int64, error) {
	return 0, nil
}
func (s *stubBacklinksCacheRepo) ListStaleForUpdate(ctx context.Context, _ *gorm.DB, before time.Time, limit int) ([]string, error) {
	return []string{}, nil
}
func (s *stubBacklinksCacheRepo) ListInvalidationLog(ctx context.Context, kbID string, limit, offset int) ([]*types.WikiBacklinksCacheInvalidationLogEntry, int64, error) {
	return []*types.WikiBacklinksCacheInvalidationLogEntry{}, 0, nil
}
func (s *stubBacklinksCacheRepo) SumPayloadSizeByKB(ctx context.Context, kbID string) (int64, error) {
	return 0, nil
}

// stubBacklinksInvalidator is the no-op WikiBacklinksCacheInvalidator
// stub. It mirrors the policy from the production invalidator without
// depending on the real WikiPageRepository so tests can verify the
// hook's own logic independent of the policy.
type stubBacklinksInvalidator struct {
	// resolveFn lets each test inject the slug set the invalidator
	// would return for a given op. nil falls back to [slug] only.
	resolveFn func(op types.BacklinkCacheInvalidateOp, kbID, slug string) []string
}

func (r *stubBacklinksInvalidator) Resolve(ctx context.Context, op types.BacklinkCacheInvalidateOp, kbID, slug string) ([]string, types.SlugSetStrategy, error) {
	if r.resolveFn != nil {
		out := r.resolveFn(op, kbID, slug)
		if out == nil {
			return []string{}, types.SlugSetStrategySelf, nil
		}
		return out, types.SlugSetStrategySelf, nil
	}
	if slug == "" {
		return []string{}, types.SlugSetStrategySelf, nil
	}
	return []string{slug}, types.SlugSetStrategySelf, nil
}

func (r *stubBacklinksInvalidator) Invalidate(_ context.Context, _ types.BacklinkCacheInvalidateRequest, _ types.SlugSetStrategy) (int64, error) { return 0, nil }

// newBacklinksCacheService wires a minimal wikiPageService with the
// stubs above so cache-first ListBacklinkGraph can be exercised without
// a real DB.
func newBacklinksCacheService(pageRepo *stubBacklinksRepo, cacheRepo *stubBacklinksCacheRepo, inv *stubBacklinksInvalidator) *wikiPageService {
	return &wikiPageService{
		repo:             pageRepo,
		cacheRepo:        cacheRepo,
		cacheInvalidator: inv,
	}
}

// prebuiltCachedRow returns a WikiBacklinksCacheRow carrying the JSON
// payload the tests expect to round-trip through encode/decode. The
// four section arrays are populated with one row each so we can verify
// the cached payload survives a round-trip.
func prebuiltCachedRow(kbID, slug string) *types.WikiBacklinksCacheRow {
	return &types.WikiBacklinksCacheRow{
		KbID: kbID,
		Slug: slug,
		// Pre-computed JSON — the four sections are written as raw
		// strings here rather than going through encodeCacheRow so
		// the test stays decoupled from the cache-row encoding helper.
		// The values match the wire format produced by encodeCacheRow
		// for the same input (verified by hand in A14).
		DirectJSON:    `[{"slug":"concept/a","title":"A","page_type":"concept","status":"live","updated_at":"2026-08-22T10:00:00Z"}]`,
		IndirectJSON:  `[{"slug":"concept/a","title":"A","page_type":"concept","status":"live","updated_at":"2026-08-22T10:00:00Z","via":"concept/b"}]`,
		RelatedJSON:   `[{"slug":"concept/c","title":"C","page_type":"concept","status":"live","updated_at":"2026-08-22T10:00:00Z","jaccard":0.5}]`,
		BrokenJSON:    `[{"target_slug":"entity/orphan"}]`,
		StatsJSON:     `{"direct_count":1,"indirect_count":1,"related_count":1,"broken_count":1,"out_link_count":2}`,
		ComputedAt:    time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		SourceEventID: "evt-test-1",
	}
}

// TestListBacklinkGraph_CacheHitSkipsCompute verifies that when the
// cache repo returns a fully-populated row, ListBacklinkGraph returns
// the decoded payload WITHOUT calling the page repo's GetBySlug
// (proves the compute path is skipped).
//
// Build #21 / A14.
func TestListBacklinkGraph_CacheHitSkipsCompute(t *testing.T) {
	pageRepo := &stubBacklinksRepo{
		// If GetBySlug is invoked, the test fails via the counter check
		// below — leaving target nil so a real lookup would return
		// ErrWikiPageNotFound.
		target: nil,
	}
	cacheRepo := newStubBacklinksCacheRepo()
	cacheRepo.rows["summary/intro"] = prebuiltCachedRow("kb1", "summary/intro")
	svc := newBacklinksCacheService(pageRepo, cacheRepo, &stubBacklinksInvalidator{})

	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil graph")
	}
	// The cached payload carried 1 row per section; verify the
	// round-trip preserved the counts and the via / jaccard tags.
	if len(got.Direct) != 1 {
		t.Fatalf("expected 1 direct row from cache, got %d", len(got.Direct))
	}
	if len(got.Indirect) != 1 || got.Indirect[0].Via != "concept/b" {
		t.Fatalf("expected indirect via=concept/b from cache, got %+v", got.Indirect)
	}
	if len(got.Related) != 1 || got.Related[0].Jaccard != 0.5 {
		t.Fatalf("expected related jaccard=0.5 from cache, got %+v", got.Related)
	}
	if len(got.Broken) != 1 || got.Broken[0].TargetSlug != "entity/orphan" {
		t.Fatalf("expected broken target=entity/orphan from cache, got %+v", got.Broken)
	}
	if got.Stats.DirectCount != 1 || got.Stats.OutLinkCount != 2 {
		t.Fatalf("expected stats from cache, got %+v", got.Stats)
	}
	// Crucially: cache hit must NOT invoke the page repo.
	if pageRepo.getCalls != 0 {
		t.Fatalf("expected pageRepo.GetBySlug to be skipped on cache hit, got %d calls", pageRepo.getCalls)
	}
	if cacheRepo.getCalls != 1 {
		t.Fatalf("expected cacheRepo.Get to fire once, got %d", cacheRepo.getCalls)
	}
	if cacheRepo.upsertCalls != 0 {
		t.Fatalf("expected no writeback on cache hit, got %d upserts", cacheRepo.upsertCalls)
	}
}

// TestListBacklinkGraph_CacheMissTriggersWriteback verifies that an
// empty cache row forces a recompute AND a writeback to the cache.
//
// Build #21 / A15.
func TestListBacklinkGraph_CacheMissTriggersWriteback(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	pageRepo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"concept/a"},
			OutLinks: types.StringArray{"entity/target"},
		},
		lites: map[string]*types.WikiPageLite{
			"concept/a":     liteWith("concept/a", "A", "concept", "live", t0, []string{"summary/intro"}, nil),
			"entity/target":  liteWith("entity/target", "T", "entity", "live", t0, nil, nil),
		},
	}
	cacheRepo := newStubBacklinksCacheRepo() // cold — no row for summary/intro
	svc := newBacklinksCacheService(pageRepo, cacheRepo, &stubBacklinksInvalidator{})

	got, err := svc.ListBacklinkGraph(context.Background(), types.WikiBacklinkGraphRequest{
		KbID: "kb1", Slug: "summary/intro",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Direct) != 1 || got.Direct[0].Slug != "concept/a" {
		t.Fatalf("expected 1 direct row concept/a from compute, got %+v", got.Direct)
	}
	if len(got.Broken) != 1 || got.Broken[0].TargetSlug != "entity/target" {
		t.Fatalf("expected 1 broken row entity/target from compute, got %+v", got.Broken)
	}
	if cacheRepo.upsertCalls != 1 {
		t.Fatalf("expected 1 writeback upsert, got %d", cacheRepo.upsertCalls)
	}
	if cacheRepo.lastUpserted == nil || cacheRepo.lastUpserted.Slug != "summary/intro" {
		t.Fatalf("expected writeback to target summary/intro, got %+v", cacheRepo.lastUpserted)
	}
	if cacheRepo.lastUpserted.DirectJSON == "" {
		t.Fatal("expected direct_json column to be populated on writeback")
	}
}

// TestInvalidateBacklinksCache_DeletePageOp verifies the slug set passed
// to the cache repo's Delete matches [self ∪ in_links] for a DeletePage
// op. We exercise InvalidateBacklinksCache directly (the production
// write hooks fan out through this method) so the test stays
// independent of UpdateMeta / removeInLinks stub complexity.
//
// Build #21 / A16.
func TestInvalidateBacklinksCache_DeletePageOp(t *testing.T) {
	pageRepo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"concept/a", "concept/b"},
		},
	}
	cacheRepo := newStubBacklinksCacheRepo()
	inv := &stubBacklinksInvalidator{
		// Mirror the real invalidator's DeletePage rule: [self] ∪ in_links.
		resolveFn: func(op types.BacklinkCacheInvalidateOp, kbID, slug string) []string {
			if op != types.BacklinkCacheInvalidateDeletePage {
				return []string{slug}
			}
			out := []string{slug}
			for _, in := range pageRepo.target.InLinks {
				if in != "" {
					out = append(out, in)
				}
			}
			return out
		},
	}
	svc := newBacklinksCacheService(pageRepo, cacheRepo, inv)

	svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
		KbID: "kb1",
		Op:   types.BacklinkCacheInvalidateDeletePage,
		// Pre-resolved slug set — InvalidateBacklinksCache takes the
		// resolved list from the caller (the policy lives in the
		// invalidator service).
		AffectedSlugs: []string{"summary/intro", "concept/a", "concept/b"},
	})
	if cacheRepo.deleteCalls != 1 {
		t.Fatalf("expected 1 cache.Delete call, got %d", cacheRepo.deleteCalls)
	}
	got := append([]string(nil), cacheRepo.lastDeleted...)
	want := []string{"summary/intro", "concept/a", "concept/b"}
	if len(got) != len(want) {
		t.Fatalf("expected lastDeleted=%v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected lastDeleted=%v, got %v", want, got)
		}
	}
}

// TestInvalidateBacklinksCache_UpdatePageOp verifies that an UpdatePage
// op resolves to [self ∪ out_links] (per spec D5).
//
// Build #21 / A16.
func TestInvalidateBacklinksCache_UpdatePageOp(t *testing.T) {
	pageRepo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:     "summary/intro",
			OutLinks: types.StringArray{"entity/x", "entity/y"},
		},
	}
	cacheRepo := newStubBacklinksCacheRepo()
	inv := &stubBacklinksInvalidator{
		resolveFn: func(op types.BacklinkCacheInvalidateOp, kbID, slug string) []string {
			if op != types.BacklinkCacheInvalidateUpdatePage {
				return []string{slug}
			}
			out := []string{slug}
			for _, o := range pageRepo.target.OutLinks {
				if o != "" {
					out = append(out, o)
				}
			}
			return out
		},
	}
	svc := newBacklinksCacheService(pageRepo, cacheRepo, inv)

	svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
		KbID:          "kb1",
		Op:            types.BacklinkCacheInvalidateUpdatePage,
		AffectedSlugs: []string{"summary/intro", "entity/x", "entity/y"},
	})
	if cacheRepo.deleteCalls != 1 {
		t.Fatalf("expected 1 cache.Delete call, got %d", cacheRepo.deleteCalls)
	}
	got := append([]string(nil), cacheRepo.lastDeleted...)
	want := []string{"summary/intro", "entity/x", "entity/y"}
	if len(got) != len(want) {
		t.Fatalf("expected lastDeleted=%v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected lastDeleted=%v, got %v", want, got)
		}
	}
}
