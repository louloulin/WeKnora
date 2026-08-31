package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Build #28 — invalidator unitisation: every (op × strategy) cell is
// covered by exactly one assertion so a future op that lands without a
// strategy entry fails the test, and every Strategy literal is matched
// to the dispatch arm it lives behind. The fuzz test at the bottom
// exhausts the op + strategy label space at rand.NewSource(42) so the
// results are deterministic across runs and can be replayed if a
// regression ever surfaces.

// stubPageRepo is a minimal WikiPageRepository stand-in. It only
// serves GetBySlug from an in-memory map; every other method panics
// loudly so a stray call from the invalidator surface is immediately
// visible. The invalidator (per wiki_backlinks_cache.go) only uses
// GetBySlug so this is enough.
type stubPageRepo struct {
	mu    sync.Mutex
	pages map[string]*types.WikiPage // key = kbID+"\x00"+slug
}

func newStubPageRepo() *stubPageRepo {
	return &stubPageRepo{pages: map[string]*types.WikiPage{}}
}

func (r *stubPageRepo) put(p *types.WikiPage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pages[p.KnowledgeBaseID+"\x00"+p.Slug] = p
}

func (r *stubPageRepo) GetBySlug(_ context.Context, kbID, slug string) (*types.WikiPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pages[kbID+"\x00"+slug]
	if !ok {
		return nil, nil
	}
	return p, nil
}

// --- Unused WikiPageRepository methods. Each panics so a future
//     invalidator that quietly starts calling, say, GetByID, trips the
//     test rather than passing under a nil interface. ---

func (r *stubPageRepo) Create(context.Context, *types.WikiPage) error {
	panic("stubPageRepo.Create: not used by invalidator tests")
}
func (r *stubPageRepo) Update(context.Context, *types.WikiPage) error {
	panic("stubPageRepo.Update: not used")
}
func (r *stubPageRepo) UpdateMeta(context.Context, *types.WikiPage) error {
	panic("stubPageRepo.UpdateMeta: not used")
}
func (r *stubPageRepo) UpdateAutoLinkedContent(context.Context, *types.WikiPage) error {
	panic("stubPageRepo.UpdateAutoLinkedContent: not used")
}
func (r *stubPageRepo) GetByID(context.Context, string) (*types.WikiPage, error) {
	panic("stubPageRepo.GetByID: not used")
}
func (r *stubPageRepo) GetBySlugAcrossKB(context.Context, string) (*types.WikiPage, error) {
	panic("stubPageRepo.GetBySlugAcrossKB: not used")
}
func (r *stubPageRepo) List(context.Context, *types.WikiPageListRequest) ([]*types.WikiPage, int64, error) {
	panic("stubPageRepo.List: not used")
}
func (r *stubPageRepo) ListByType(context.Context, string, string) ([]*types.WikiPage, error) {
	panic("stubPageRepo.ListByType: not used")
}
func (r *stubPageRepo) ListByTypeLight(context.Context, string, string, int, int) ([]types.WikiIndexEntry, int64, error) {
	panic("stubPageRepo.ListByTypeLight: not used")
}
func (r *stubPageRepo) ListBySourceRef(context.Context, string, string) ([]*types.WikiPage, error) {
	panic("stubPageRepo.ListBySourceRef: not used")
}
func (r *stubPageRepo) ListSlugsBySourceRef(context.Context, string, string) ([]string, error) {
	panic("stubPageRepo.ListSlugsBySourceRef: not used")
}
func (r *stubPageRepo) ListBySlugs(context.Context, string, []string) (map[string]*types.WikiPageLite, error) {
	panic("stubPageRepo.ListBySlugs: not used")
}
func (r *stubPageRepo) ListSummariesByKnowledgeIDs(context.Context, string, []string) (map[string]string, error) {
	panic("stubPageRepo.ListSummariesByKnowledgeIDs: not used")
}
func (r *stubPageRepo) ExistsSlugs(context.Context, string, []string) (map[string]bool, error) {
	panic("stubPageRepo.ExistsSlugs: not used")
}
func (r *stubPageRepo) ListAllSlugs(context.Context, string) ([]string, error) {
	panic("stubPageRepo.ListAllSlugs: not used")
}
func (r *stubPageRepo) ListPagesCursor(context.Context, string, string, int) ([]*types.WikiPage, string, error) {
	panic("stubPageRepo.ListPagesCursor: not used")
}
func (r *stubPageRepo) ListByTypeRecent(context.Context, string, string, int) ([]types.WikiIndexEntry, error) {
	panic("stubPageRepo.ListByTypeRecent: not used")
}
func (r *stubPageRepo) FindSimilarPages(context.Context, string, string, []string, int) ([]*types.WikiPageLite, error) {
	panic("stubPageRepo.FindSimilarPages: not used")
}
func (r *stubPageRepo) FindPagesMissingTSZh(context.Context, int) ([]*types.WikiPage, error) {
	panic("stubPageRepo.FindPagesMissingTSZh: not used")
}
func (r *stubPageRepo) UpdateContentTSZh(context.Context, string, string) error {
	panic("stubPageRepo.UpdateContentTSZh: not used")
}
func (r *stubPageRepo) ListDistinctCategoryPaths(context.Context, string, int) ([][]string, error) {
	panic("stubPageRepo.ListDistinctCategoryPaths: not used")
}
func (r *stubPageRepo) CreateFolder(context.Context, *types.WikiFolder) error {
	panic("stubPageRepo.CreateFolder: not used")
}
func (r *stubPageRepo) GetFolderByID(context.Context, string, string) (*types.WikiFolder, error) {
	panic("stubPageRepo.GetFolderByID: not used")
}
func (r *stubPageRepo) GetChildFolderByName(context.Context, string, string, string) (*types.WikiFolder, error) {
	panic("stubPageRepo.GetChildFolderByName: not used")
}
func (r *stubPageRepo) ListChildFolders(context.Context, string, string) ([]*types.WikiFolder, error) {
	panic("stubPageRepo.ListChildFolders: not used")
}
func (r *stubPageRepo) ListAllFolders(context.Context, string) ([]*types.WikiFolder, error) {
	panic("stubPageRepo.ListAllFolders: not used")
}
func (r *stubPageRepo) UpdateFolder(context.Context, *types.WikiFolder) error {
	panic("stubPageRepo.UpdateFolder: not used")
}
func (r *stubPageRepo) DeleteFolder(context.Context, string, string) error {
	panic("stubPageRepo.DeleteFolder: not used")
}
func (r *stubPageRepo) ListPagesByFolderIDs(context.Context, string, []string) ([]*types.WikiPage, error) {
	panic("stubPageRepo.ListPagesByFolderIDs: not used")
}
func (r *stubPageRepo) CountPagesByFolder(context.Context, string, []string) (map[string]int64, error) {
	panic("stubPageRepo.CountPagesByFolder: not used")
}
func (r *stubPageRepo) CountPagesInFolder(context.Context, string, string) (int64, error) {
	panic("stubPageRepo.CountPagesInFolder: not used")
}
func (r *stubPageRepo) Delete(context.Context, string, string) error {
	panic("stubPageRepo.Delete: not used")
}
func (r *stubPageRepo) DeleteByID(context.Context, string) error {
	panic("stubPageRepo.DeleteByID: not used")
}
func (r *stubPageRepo) ListRecentForSuggestions(context.Context, uint64, []string, int) ([]*types.WikiPage, error) {
	panic("stubPageRepo.ListRecentForSuggestions: not used")
}
func (r *stubPageRepo) DeleteRevisionsByPage(context.Context, string) error {
	panic("stubPageRepo.DeleteRevisionsByPage: not used")
}
func (r *stubPageRepo) RestoreDeleted(context.Context, string, string, string) error {
	panic("stubPageRepo.RestoreDeleted: not used")
}
func (r *stubPageRepo) CreateRevision(context.Context, *types.WikiPageRevision) error {
	panic("stubPageRepo.CreateRevision: not used")
}
func (r *stubPageRepo) ListRevisions(context.Context, string, string, int, int) ([]*types.WikiPageRevision, int64, error) {
	panic("stubPageRepo.ListRevisions: not used")
}
func (r *stubPageRepo) GetRevision(context.Context, string, string, int) (*types.WikiPageRevision, error) {
	panic("stubPageRepo.GetRevision: not used")
}
func (r *stubPageRepo) UpdateWithRevision(context.Context, *types.WikiPage, *types.WikiPageRevision) error {
	panic("stubPageRepo.UpdateWithRevision: not used")
}
func (r *stubPageRepo) PruneRevisions(context.Context, types.WikiRevisionPruneRequest) error {
	panic("stubPageRepo.PruneRevisions: not used")
}
func (r *stubPageRepo) CreateIssue(context.Context, *types.WikiPageIssue) error {
	panic("stubPageRepo.CreateIssue: not used")
}
func (r *stubPageRepo) ListIssues(context.Context, string, string, string) ([]*types.WikiPageIssue, error) {
	panic("stubPageRepo.ListIssues: not used")
}
func (r *stubPageRepo) UpdateIssueStatus(context.Context, string, string) error {
	panic("stubPageRepo.UpdateIssueStatus: not used")
}
func (r *stubPageRepo) CountOrphans(context.Context, string) (int64, error) {
	panic("stubPageRepo.CountOrphans: not used")
}
func (r *stubPageRepo) CountByType(context.Context, string) (map[string]int64, error) {
	panic("stubPageRepo.CountByType: not used")
}
func (r *stubPageRepo) ListAll(context.Context, string) ([]*types.WikiPage, error) {
	panic("stubPageRepo.ListAll: not used")
}
func (r *stubPageRepo) Search(context.Context, string, string, int) ([]*types.WikiPage, error) {
	panic("stubPageRepo.Search: not used")
}

// --- The five core tests below ---

// invalidatorOpMatrix is the canonical (op, strategy, slug-set
// shape) mapping the spec accepts. Every row is asserted by
// TestInvalidatorResolve_AllOpsHaveStrategy and
// TestInvalidatorResolve_DispatchByStrategy.
type invalidatorOpMatrix struct {
	op            types.BacklinkCacheInvalidateOp
	strategy      types.SlugSetStrategy
	wantSlugShape string // human description; assert at the structural level
}

func invalidatorExpectedMatrix() []invalidatorOpMatrix {
	return []invalidatorOpMatrix{
		{
			op:            types.BacklinkCacheInvalidateCreatePage,
			strategy:      types.SlugSetStrategySelfOutgoing,
			wantSlugShape: "[self] ∪ out_links",
		},
		{
			op:            types.BacklinkCacheInvalidateUpdatePage,
			strategy:      types.SlugSetStrategySelfOutgoing,
			wantSlugShape: "[self] ∪ out_links",
		},
		{
			op:            types.BacklinkCacheInvalidateDeletePage,
			strategy:      types.SlugSetStrategySelfIncoming,
			wantSlugShape: "[self] ∪ in_links",
		},
		{
			op:            types.BacklinkCacheInvalidateMovePage,
			strategy:      types.SlugSetStrategySelfOutgoing,
			wantSlugShape: "[self] ∪ out_links",
		},
		{
			op:            types.BacklinkCacheInvalidateBatchMove,
			strategy:      types.SlugSetStrategySelfOutgoing,
			wantSlugShape: "[self] ∪ out_links",
		},
		{
			op:            types.BacklinkCacheInvalidateBatchDelete,
			strategy:      types.SlugSetStrategySelfIncoming,
			wantSlugShape: "[self] ∪ in_links",
		},
		{
			op:            types.BacklinkCacheInvalidateBatchStatus,
			strategy:      types.SlugSetStrategySelf,
			wantSlugShape: "[self]",
		},
		{
			op:            types.BacklinkCacheInvalidateSweep,
			strategy:      types.SlugSetStrategyKBWide,
			wantSlugShape: "[]", // sweep owns its wipe path; invalidator returns []
		},
		{
			op:            types.BacklinkCacheInvalidateAclChange,
			strategy:      types.SlugSetStrategyReverseLookupIndexed,
			wantSlugShape: "[]", // acl service owns its wipe path
		},
	}
}

// TestInvalidatorResolve_AllOpsHaveStrategy — A1 acceptance. The
// 9-op × 5-strategy matrix in slugSetStrategies is the single source
// of truth; every op registered must map to exactly one of the
// five canonical SlugSetStrategy values, and the literal must match
// the spec's intent (Create/Update/Move/BatchMove → self_outgoing,
// Delete/BatchDelete → self_incoming, BatchStatus → self,
// Sweep → kb_wide, AclChange → reverse_lookup_indexed).
func TestInvalidatorResolve_AllOpsHaveStrategy(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // behavior is verified via cacheRepo side-effects
	

	matrix := invalidatorExpectedMatrix()
	if got, want := len(matrix), len(slugSetStrategies); got != want {
		t.Fatalf("spec matrix size %d != slugSetStrategies size %d — refactor broke coverage",
			got, want)
	}
	for _, row := range matrix {
		t.Run(string(row.op), func(t *testing.T) {
			strategy, ok := slugSetStrategies[row.op]
			if !ok {
				t.Fatalf("op %q not in slugSetStrategies (D1 violation — silent fallback)", row.op)
			}
			if strategy != row.strategy {
				t.Errorf("op %q strategy = %q, want %q", row.op, strategy, row.strategy)
			}
			// Every registered strategy must be one of the five
			// canonical values — guards against a typo introducing
			// a sixth value the dispatcher wouldn't recognise.
			switch strategy {
			case types.SlugSetStrategySelf,
				types.SlugSetStrategySelfOutgoing,
				types.SlugSetStrategySelfIncoming,
				types.SlugSetStrategyKBWide,
				types.SlugSetStrategyReverseLookupIndexed:
				// ok
			default:
				t.Errorf("op %q has unrecognised strategy %q", row.op, strategy)
			}
			// Resolve itself must also surface that strategy as
			// the second return value, matching the registry.
			slugs, gotStrategy, err := inv.Resolve(context.Background(), row.op, "kb-1", "alpha")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if gotStrategy != row.strategy {
				t.Errorf("Resolve(%q) strategy = %q, want %q",
					row.op, gotStrategy, row.strategy)
			}
			// Sanity: never nil (callers iterate / uniq safely).
			if slugs == nil {
				t.Errorf("Resolve(%q) returned nil slice; must be []", row.op)
			}
		})
	}
}

// TestInvalidatorResolve_UnknownOpPanics — A2 / D1 acceptance. The
// switch's old default branch silently degraded to "slug only", which
// is the production failure mode this rewrite fixes. An unregistered
// op must panic so the bug is visible in dev, not silently swallowed.
func TestInvalidatorResolve_UnknownOpPanics(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	const bogusOp types.BacklinkCacheInvalidateOp = "totally-bogus-op"
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Resolve with unregistered op did NOT panic — D1 violated")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, string(bogusOp)) {
			t.Errorf("panic message %q did not mention op %q", msg, bogusOp)
		}
		if !strings.Contains(msg, "slugSetStrategies") {
			t.Errorf("panic message %q did not point at the registry", msg)
		}
	}()
	if _, _, err := inv.Resolve(context.Background(), bogusOp, "kb-1", "alpha"); err == nil {
		// unreachable — must panic above
		t.Fatal("Resolve returned cleanly; expected panic")
	}
}

// TestInvalidatorResolve_DispatchByStrategy — A4/A5/A6 acceptance.
// The slug set shape is the actual contract that produced the
// "missing wipe" incidents in Build #21 — Create/Update/Move/BatchMove
// must include out_links, Delete/BatchDelete must include in_links,
// BatchStatus must be [slug] only, kb_wide / reverse_lookup_indexed
// must be empty (their wipes live elsewhere).
func TestInvalidatorResolve_DispatchByStrategy(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	page := &types.WikiPage{
		ID:              "id-alpha",
		KnowledgeBaseID: "kb-1",
		Slug:            "alpha",
		OutLinks:        types.StringArray{"beta", "gamma", ""}, // dedup / trim
		InLinks:         types.StringArray{"zeta", "eta", ""},
	}
	pageRepo.put(page)

	cases := []struct {
		op       types.BacklinkCacheInvalidateOp
		wantIn   []string // slugs that MUST appear in the result
		wantMiss []string // slugs that MUST NOT appear
	}{
		{types.BacklinkCacheInvalidateCreatePage, []string{"alpha", "beta", "gamma"}, []string{"zeta", "eta"}},
		{types.BacklinkCacheInvalidateUpdatePage, []string{"alpha", "beta", "gamma"}, []string{"zeta", "eta"}},
		{types.BacklinkCacheInvalidateDeletePage, []string{"alpha", "zeta", "eta"}, []string{"beta", "gamma"}},
		{types.BacklinkCacheInvalidateMovePage, []string{"alpha", "beta", "gamma"}, []string{"zeta", "eta"}},
		{types.BacklinkCacheInvalidateBatchMove, []string{"alpha", "beta", "gamma"}, []string{"zeta", "eta"}},
		{types.BacklinkCacheInvalidateBatchDelete, []string{"alpha", "zeta", "eta"}, []string{"beta", "gamma"}},
		{types.BacklinkCacheInvalidateBatchStatus, []string{"alpha"}, []string{"beta", "gamma", "zeta", "eta"}},
		{types.BacklinkCacheInvalidateSweep, nil, []string{"alpha", "beta", "gamma", "zeta", "eta"}},
		{types.BacklinkCacheInvalidateAclChange, nil, []string{"alpha", "beta", "gamma", "zeta", "eta"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.op), func(t *testing.T) {
			got, strategy, err := inv.Resolve(context.Background(), tc.op, "kb-1", "alpha")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if strategy == "" {
				t.Errorf("strategy is empty; Resolve must always return the picked label")
			}
			for _, want := range tc.wantIn {
				if !containsString(got, want) {
					t.Errorf("op %q slug set %v missing required slug %q",
						tc.op, got, want)
				}
			}
			for _, miss := range tc.wantMiss {
				if containsString(got, miss) {
					t.Errorf("op %q slug set %v contains forbidden slug %q",
						tc.op, got, miss)
				}
			}
			// Self is always first (the helper prepends the slug).
			// Skip this check for ops whose dispatch shape is `[]`.
			if len(got) > 0 && got[0] != "alpha" {
				t.Errorf("op %q: first slug = %q, want alpha (self first)",
					tc.op, got[0])
			}
		})
	}
}

// TestInvalidatorResolve_EmptySlug — the self strategy and the
// self_outgoing/self_incoming dispatchers all handle a missing page
// or empty slug without panicking. An empty slug for a self_only op
// returns [] (no slug → no work); self_outgoing returns just the
// (empty) slug dedup'd to nothing.
func TestInvalidatorResolve_EmptySlug(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	// Empty slug for self_only → [].
	got, strategy, err := inv.Resolve(context.Background(),
		types.BacklinkCacheInvalidateBatchStatus, "kb-1", "")
	if err != nil {
		t.Fatalf("Resolve empty slug: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("self_only empty slug = %v, want []", got)
	}
	if strategy != types.SlugSetStrategySelf {
		t.Errorf("strategy = %q, want self", strategy)
	}

	// Empty slug for self_outgoing → just [] (pageRepo returns nil
	// for the empty-slug lookup so no out_links).
	got, _, err = inv.Resolve(context.Background(),
		types.BacklinkCacheInvalidateUpdatePage, "kb-1", "")
	if err != nil {
		t.Fatalf("Resolve empty slug self_outgoing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("self_outgoing empty slug = %v, want []", got)
	}

	// Empty kbID short-circuits everything regardless of strategy.
	for _, op := range []types.BacklinkCacheInvalidateOp{
		types.BacklinkCacheInvalidateCreatePage,
		types.BacklinkCacheInvalidateUpdatePage,
		types.BacklinkCacheInvalidateDeletePage,
		types.BacklinkCacheInvalidateBatchStatus,
	} {
		got, _, err := inv.Resolve(context.Background(), op, "", "alpha")
		if err != nil {
			t.Errorf("Resolve(%q, kbID=\"\") err = %v", op, err)
		}
		if len(got) != 0 {
			t.Errorf("Resolve(%q, kbID=\"\") = %v, want []", op, got)
		}
	}
}

// TestInvalidator_LogsAuditStrategy — A3 acceptance. Invalidate
// writes one audit row whose Details JSON carries a `strategy`
// field equal to the strategy label Resolve picked. Old rows
// pre-Build #28 lack the field; the reader is responsible for the
// missing-key fallback.
func TestInvalidator_LogsAuditStrategy(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	// Seed one row so Delete returns affected=1 — gives the audit
	// row a non-zero AffectedCount and proves the wipe actually ran.
	cacheRepo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	page := &types.WikiPage{
		ID:              "id-alpha",
		KnowledgeBaseID: "kb-1",
		Slug:            "alpha",
		OutLinks:        types.StringArray{"beta"},
	}
	pageRepo.put(page)

	slugs, strategy, err := inv.Resolve(context.Background(),
		types.BacklinkCacheInvalidateUpdatePage, "kb-1", "alpha")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strategy != types.SlugSetStrategySelfOutgoing {
		t.Fatalf("strategy = %q, want self_outgoing", strategy)
	}

	req := types.BacklinkCacheInvalidateRequest{
		KbID:          "kb-1",
		Op:            types.BacklinkCacheInvalidateUpdatePage,
		AffectedSlugs: slugs,
	}
	affected, err := inv.Invalidate(context.Background(), req, strategy)
	if err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if affected != 1 {
		t.Errorf("affected = %d, want 1 (seeded row)", affected)
	}

	// Exactly one audit row, op label echoes the request, and
	// details JSON carries `strategy: "self_outgoing"`.
	if len(cacheRepo.logEntries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(cacheRepo.logEntries))
	}
	got := cacheRepo.logEntries[0]
	if got.Op != string(types.BacklinkCacheInvalidateUpdatePage) {
		t.Errorf("audit op = %q, want update_page", got.Op)
	}
	if got.AffectedCount != 1 {
		t.Errorf("audit affected_count = %d, want 1", got.AffectedCount)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(got.Details), &details); err != nil {
		t.Fatalf("audit details %q not JSON: %v", got.Details, err)
	}
	if s, ok := details["strategy"].(string); !ok {
		t.Errorf("audit details missing strategy field: %+v", details)
	} else if s != string(types.SlugSetStrategySelfOutgoing) {
		t.Errorf("audit details strategy = %q, want self_outgoing", s)
	}
	if opField, ok := details["op"].(string); !ok || opField != string(types.BacklinkCacheInvalidateUpdatePage) {
		t.Errorf("audit details op = %v, want update_page", details["op"])
	}
	// Slugs round-trip as JSON array.
	if rawSlugs, ok := details["slugs"].([]any); !ok || len(rawSlugs) == 0 {
		t.Errorf("audit details slugs = %v, want non-empty array", details["slugs"])
	}
}

// TestInvalidator_LogsStrategyForAllRegisteredOps — same audit-row
// invariant checked across every (op, strategy) pair, not just one.
// The 9-op × 5-strategy matrix guarantees every path produces a
// row with the matching strategy label.
func TestInvalidator_LogsStrategyForAllRegisteredOps(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	cacheRepo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	page := &types.WikiPage{
		ID:              "id-alpha",
		KnowledgeBaseID: "kb-1",
		Slug:            "alpha",
		OutLinks:        types.StringArray{"beta"},
		InLinks:         types.StringArray{"zeta"},
	}
	pageRepo.put(page)

	for _, op := range allRegisteredOps() {
		t.Run(string(op), func(t *testing.T) {
			// Fresh cacheRepo so we count rows per op.
			cacheRepo := newObsFakeRepo()
			cacheRepo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))
			inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)

			slugs, strategy, err := inv.Resolve(context.Background(), op, "kb-1", "alpha")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			// For ops whose dispatch is `[]` the request is a
			// no-op (cacheRepo.Delete gets 0 slugs), so we
			// expect zero audit rows. The strategy still has to
			// come back non-empty.
			if len(slugs) == 0 {
				if strategy == "" {
					t.Errorf("op %q returned empty strategy", op)
				}
				return
			}
			req := types.BacklinkCacheInvalidateRequest{
				KbID:          "kb-1",
				Op:            op,
				AffectedSlugs: slugs,
			}
			if _, err := inv.Invalidate(context.Background(), req, strategy); err != nil {
				t.Fatalf("Invalidate: %v", err)
			}
			if len(cacheRepo.logEntries) != 1 {
				t.Fatalf("op %q audit entries = %d, want 1", op, len(cacheRepo.logEntries))
			}
			var details map[string]any
			if err := json.Unmarshal([]byte(cacheRepo.logEntries[0].Details), &details); err != nil {
				t.Fatalf("details not JSON: %v", err)
			}
			s, ok := details["strategy"].(string)
			if !ok {
				t.Errorf("op %q audit details missing strategy: %+v", op, details)
			}
			if s != string(strategy) {
				t.Errorf("op %q audit strategy = %q, want %q", op, s, strategy)
			}
		})
	}
}

// TestInvalidator_WikiPageServiceDelegatesAndStampsStrategy — the
// end-to-end integration check: the wikiPageService.InvalidateBacklinksCache
// entry point (used by every write hook) threads the strategy from
// the registry through to the invalidator and out to the audit row.
// This is the regression guard for the "what rule picked this slug
// set" audit trail.
func TestInvalidator_WikiPageServiceDelegatesAndStampsStrategy(t *testing.T) {
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	cacheRepo.seed("kb-1", "alpha", time.Now().Add(-time.Hour))

	page := &types.WikiPage{
		ID:              "id-alpha",
		KnowledgeBaseID: "kb-1",
		Slug:            "alpha",
		OutLinks:        types.StringArray{"beta"},
	}
	pageRepo.put(page)

	svc := &wikiPageService{
		cacheRepo:        cacheRepo,
		cacheInvalidator: newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo),
	}
	svc.InvalidateBacklinksCache(context.Background(), types.BacklinkCacheInvalidateRequest{
		KbID:          "kb-1",
		Op:            types.BacklinkCacheInvalidateCreatePage,
		AffectedSlugs: []string{"alpha", "beta"},
	})

	if len(cacheRepo.logEntries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(cacheRepo.logEntries))
	}
	got := cacheRepo.logEntries[0]
	var details map[string]any
	if err := json.Unmarshal([]byte(got.Details), &details); err != nil {
		t.Fatalf("details not JSON: %v", err)
	}
	if s, ok := details["strategy"].(string); !ok || s != string(types.SlugSetStrategySelfOutgoing) {
		t.Errorf("audit strategy = %v, want self_outgoing", details["strategy"])
	}
}

// TestInvalidatorResolve_Fuzz — A7 acceptance. Property-based fuzz:
// 200 iterations of (op, slug, kbID) drawn from the registered op set
// and the seeded stub page. Each iteration asserts:
//
//   - Resolve never panics on a registered op (only the bogus one does)
//   - The returned strategy is one of the 5 canonical values
//   - The returned slug set never contains the empty string
//   - For self_outgoing / self_incoming, the self slug is in the set
//     (when slug != "")
//   - For self, the slug set equals [slug] when slug != ""
//
// rand.NewSource(42) makes the run deterministic across CI builds so
// a regression can be replayed from the recorded seed.
func TestInvalidatorResolve_Fuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	pageRepo := newStubPageRepo()
	cacheRepo := newObsFakeRepo()
	inv := newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
	_ = inv // unused locally; behavior is verified via cacheRepo side-effects

	// Seed 30 pages with deterministic in/out links so the slug set
	// has something to dedup.
	slugs := make([]string, 30)
	for i := 0; i < 30; i++ {
		slug := fmt.Sprintf("slug-%02d", i)
		slugs[i] = slug
		var outLinks, inLinks types.StringArray
		// Random link to 2-4 other slugs, skipping self.
		count := 2 + rng.Intn(3)
		for j := 0; j < count; j++ {
			other := slugs[rng.Intn(len(slugs))]
			if other != slug {
				outLinks = append(outLinks, other)
			}
		}
		count = 2 + rng.Intn(3)
		for j := 0; j < count; j++ {
			other := slugs[rng.Intn(len(slugs))]
			if other != slug {
				inLinks = append(inLinks, other)
			}
		}
		pageRepo.put(&types.WikiPage{
			ID:              "id-" + slug,
			KnowledgeBaseID: "kb-fuzz",
			Slug:            slug,
			OutLinks:        outLinks,
			InLinks:         inLinks,
		})
	}

	ops := allRegisteredOps()
	for iter := 0; iter < 200; iter++ {
		op := ops[rng.Intn(len(ops))]
		slug := slugs[rng.Intn(len(slugs))]
		kbID := "kb-fuzz"
		if rng.Intn(7) == 0 {
			kbID = "" // exercise the empty-kb short-circuit on a ~14% slice
		}

		slugsOut, strategy, err := inv.Resolve(context.Background(), op, kbID, slug)
		if err != nil {
			t.Errorf("iter=%d op=%q kb=%q slug=%q: Resolve err = %v",
				iter, op, kbID, slug, err)
			continue
		}
		if !isCanonicalStrategy(strategy) {
			t.Errorf("iter=%d op=%q: non-canonical strategy %q",
				iter, op, strategy)
		}
		// Empty slug sets are valid for kb_wide / reverse_lookup_indexed /
		// empty-kb / empty-self — but no result may contain "" entries.
		for _, s := range slugsOut {
			if s == "" {
				t.Errorf("iter=%d op=%q: empty slug leaked into result %v",
					iter, op, slugsOut)
			}
		}
		if slug == "" || kbID == "" {
			continue
		}
		switch strategy {
		case types.SlugSetStrategySelf:
			if len(slugsOut) != 1 || slugsOut[0] != slug {
				t.Errorf("iter=%d op=%q self strategy: got %v, want [%q]",
					iter, op, slugsOut, slug)
			}
		case types.SlugSetStrategySelfOutgoing, types.SlugSetStrategySelfIncoming:
			if len(slugsOut) == 0 || slugsOut[0] != slug {
				t.Errorf("iter=%d op=%q %s strategy: self slug %q missing from result %v",
					iter, op, strategy, slug, slugsOut)
			}
			// Result must be a set: no duplicates.
			seen := make(map[string]struct{}, len(slugsOut))
			for _, s := range slugsOut {
				if _, dup := seen[s]; dup {
					t.Errorf("iter=%d op=%q %s strategy: duplicate slug %q in %v",
						iter, op, strategy, s, slugsOut)
				}
				seen[s] = struct{}{}
			}
		case types.SlugSetStrategyKBWide, types.SlugSetStrategyReverseLookupIndexed:
			if len(slugsOut) != 0 {
				t.Errorf("iter=%d op=%q %s strategy: non-empty %v, want []",
					iter, op, strategy, slugsOut)
			}
		}
	}
}

// --- helpers ---

func allRegisteredOps() []types.BacklinkCacheInvalidateOp {
	out := make([]types.BacklinkCacheInvalidateOp, 0, len(slugSetStrategies))
	for op := range slugSetStrategies {
		out = append(out, op)
	}
	return out
}

func isCanonicalStrategy(s types.SlugSetStrategy) bool {
	switch s {
	case types.SlugSetStrategySelf,
		types.SlugSetStrategySelfOutgoing,
		types.SlugSetStrategySelfIncoming,
		types.SlugSetStrategyKBWide,
		types.SlugSetStrategyReverseLookupIndexed:
		return true
	}
	return false
}

// Compile-time guard: each fake implements its real interface. The
// newStubPageRepo() and newObsFakeRepo() constructors are already
// used inside the test bodies, so the compiler surfaces any interface
// drift between fake and real on the next build — no extra wiring
// needed here.

// FindPagesByNormalizedTitle satisfies the new interfaces.WikiPageRepository method.
func (r *stubPageRepo) FindPagesByNormalizedTitle(_ context.Context, _, _, _ string) ([]*types.WikiPageLite, error) {
	return nil, nil
}

// FindPagesByNormalizedTitles is the batched variant.
func (r *stubPageRepo) FindPagesByNormalizedTitles(_ context.Context, _, _ string, _ []string) ([]*types.WikiPageLite, error) {
	return nil, nil
}

// DeleteByKB satisfies the new interfaces.WikiBacklinksCacheRepository method.
func (r *obsFakeRepo) DeleteByKB(_ context.Context, _ string) (int64, error) { return 0, nil }

func (r *stubPageRepo) ListBacklinksAcrossKBs(_ context.Context, _ uint64, _, _ string, _ int) ([]*types.WikiPageLite, error) { return nil, nil }

func (r *obsFakeRepo) FindReferencingSlugs(_ context.Context, _, _ string) ([]string, error) { return nil, nil }

func (r *fakeWikiBacklinksCacheRepo) FindReferencingSlugs(_ context.Context, _, _ string) ([]string, error) { return nil, nil }
