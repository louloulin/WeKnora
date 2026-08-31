package service

import (
	"time"
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/Tencent/WeKnora/internal/types"
)

// Build #27 — acl_snapshot_hash lazy skip.
//
// PutAcl on an identical payload now skips the Build #24 cache wipe +
// invalidation-log row. The 6 tests below cover A1–A6 of the spec:
//
//   A1 / A2 — IdenticalPayload_SkipsWipe: revision bumps + audit row
//             is "noop_match" + invalidation log NOT appended + counter
//             increments (covers A1, A2, A6 in one test).
//   A3     — DifferentPayload_RunsWipe: regression — full B24 wipe path.
//   A3-bis — ReorderedAllowList_HashMatches: HashAcl canonicalizes slice
//             order, so [a,b,c] and [c,b,a] hash the same and skip.
//   A5     — LegacyRow_AlwaysWipes: a stored hash of "" never matches.
//   A4     — HashPersistedAcrossReads: GetAclBySlug returns the hash
//             written by the previous UpdateAclWithRevision.
//   Plus  — Deterministic: same input → same hash for 1000 iterations.
//
// The fixture reuses harnessStubBacklinksCache from wiki_audit_harness_test.go
// for the cache side and aclStubRepo (below) for the ACL side. Both are
// in-memory and do NOT exercise the real SQLite path — the goal is to
// confirm PutAcls branches on the hash, not to re-validate the repo.

// aclStubRepo is a stateful in-memory WikiAclRepo used by the B27 tests.
// It tracks the last action label passed to UpdateAclWithRevision so the
// tests can assert the action override (spec D5) when PutAcl takes the
// noop path.
type aclStubRepo struct {
	acl          *types.WikiPageAcl
	owner        string
	storedRev    int64
	lastAction   string
	lastSnapshot string
	updatedCount int
}

func (r *aclStubRepo) ListAudit(_ context.Context, _ string, _ time.Time, _, _ int) ([]*types.WikiAclAuditEntry, int64, error) {
	return nil, 0, nil
}

func (r *aclStubRepo) GetAclBySlug(_ context.Context, _, _ string) (*types.WikiPageAcl, error) {
	if r.acl == nil {
		return nil, nil
	}
	cp := *r.acl
	return &cp, nil
}

func (r *aclStubRepo) UpdateAclWithRevision(_ context.Context, _, _ string,
	newAcl types.WikiPageAcl, expectedRevision int64, snapshotHash string,
	_ string, _ string, action string) (*types.WikiPageAcl, error) {
	if r.acl != nil && r.acl.Revision != expectedRevision {
		return nil, types.ErrWikiPageAclRevisionConflict
	}
	r.storedRev = expectedRevision + 1
	merged := newAcl
	merged.Revision = r.storedRev
	merged.SnapshotHash = snapshotHash
	r.acl = &merged
	r.lastAction = action
	r.lastSnapshot = snapshotHash
	r.updatedCount++
	return &merged, nil
}

func (r *aclStubRepo) PageOwnerAndAdmin(_ context.Context, _, _, _ string) (string, bool, error) {
	return r.owner, false, nil
}

func (r *aclStubRepo) GroupMembers(_ context.Context, _ uint64, _ []string) ([]string, error) {
	return nil, nil
}

// newAclServiceWithCache returns a fully-wired wikiAclService backed by
// the given ACL + cache fixtures. Tests assert against the fixtures after
// each PutAcl call.
func newAclServiceWithCache(aclRepo *aclStubRepo, cacheRepo *harnessStubBacklinksCache) *wikiAclService {
	svc := &wikiAclService{repo: aclRepo, cache: newAclCache()}
	svc.SetCacheRepo(cacheRepo)
	return svc
}

// baselineCounters snapshots the per-test counter values that the tests
// assert against. Counter assertions must be deltas — the counters are
// package-global and other tests may have incremented them.
func baselineCounters(t *testing.T) (skippedHashMatch float64, invalidatedAclChange float64) {
	t.Helper()
	return testutil.ToFloat64(metricAclChangeSkippedTotal.WithLabelValues("hash_match")),
		testutil.ToFloat64(metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange)))
}

// seedStubRow pre-populates the cache fixture with one row so the small-KB
// branch (DeleteByKB) has something to delete on a real wipe.
func seedStubRow(repo *harnessStubBacklinksCache, kbID, slug string) {
	repo.rows[kbID] = map[string]string{}
	repo.rows[kbID][slug] = `{"seeded":true}`
}

// Test 1 — A1 / A2 / A6: identical PutAcl → no wipe + no log + counter.
// Seeds a page whose hash matches the next PutAcl payload, fires
// PutAcl, asserts: revision bumps by 1, audit action is "noop_match",
// invalidation log was NOT appended, and the skipped counter
// incremented by exactly 1.
func TestPutAcl_IdenticalPayload_SkipsWipe(t *testing.T) {
	aclRepo := &aclStubRepo{
		acl: &types.WikiPageAcl{
			Mode:          types.WikiPageAclModePrivate,
			AllowUserIDs:  []string{"u-1"},
			AllowGroupIDs: nil,
			DenyInherited: false,
			Revision:      3,
		},
	}
	// Pre-compute the hash as PutAcl will — so we can populate the
	// stored SnapshotHash the way the production repo would after a
	// prior write.
	aclRepo.acl.SnapshotHash = HashAcl(types.WikiPageAclModePrivate, []string{"u-1"}, nil, false)

	cacheRepo := newHarnessStubBacklinksCache()
	seedStubRow(cacheRepo, "k1", "s1")
	svc := newAclServiceWithCache(aclRepo, cacheRepo)

	skippedBefore, invBefore := baselineCounters(t)
	ctx := withUserAndTenant(context.Background(), "alice", 1)

	updated, err := svc.PutAcl(ctx, "k1", "s1", types.WikiPageAclSaveRequest{
		Mode:          types.WikiPageAclModePrivate,
		AllowUserIDs:  []string{"u-1"},
		AllowGroupIDs: nil,
		DenyInherited: false,
		BaseRevision:  3,
	}, "alice", "owner")
	if err != nil {
		t.Fatalf("PutAcl(identical): %v", err)
	}
	if updated.Revision != 4 {
		t.Fatalf("expected revision 4, got %d", updated.Revision)
	}
	if aclRepo.lastAction != "noop_match" {
		t.Fatalf("expected action=noop_match, got %q", aclRepo.lastAction)
	}
	if len(cacheRepo.logEntries) != 0 {
		t.Fatalf("expected NO invalidation log row, got %d", len(cacheRepo.logEntries))
	}
	if cacheRepo.deletesByKB != 0 {
		t.Fatalf("expected NO DeleteByKB call, got %d", cacheRepo.deletesByKB)
	}
	if cacheRepo.deletesBySlugs != 0 {
		t.Fatalf("expected NO Delete call, got %d", cacheRepo.deletesBySlugs)
	}
	// Counter delta — must be exactly +1.
	if got := testutil.ToFloat64(metricAclChangeSkippedTotal.WithLabelValues("hash_match")) - skippedBefore; got != 1 {
		t.Fatalf("expected skipped counter +1, got %v", got)
	}
	// Counter delta — the invalidation counter must NOT have moved.
	if got := testutil.ToFloat64(metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange))) - invBefore; got != 0 {
		t.Fatalf("expected invalidation counter unchanged, got %v", got)
	}
}

// Test 2 — A3 regression: a different payload runs the full wipe.
func TestPutAcl_DifferentPayload_RunsWipe(t *testing.T) {
	aclRepo := &aclStubRepo{
		acl: &types.WikiPageAcl{
			Mode:          types.WikiPageAclModePrivate,
			AllowUserIDs:  []string{"u-1"},
			DenyInherited: false,
			Revision:      3,
		},
	}
	aclRepo.acl.SnapshotHash = HashAcl(types.WikiPageAclModePrivate, []string{"u-1"}, nil, false)

	cacheRepo := newHarnessStubBacklinksCache()
	seedStubRow(cacheRepo, "k1", "s1")
	svc := newAclServiceWithCache(aclRepo, cacheRepo)

	skippedBefore, invBefore := baselineCounters(t)
	ctx := withUserAndTenant(context.Background(), "alice", 1)

	// Change the mode — hash must differ.
	updated, err := svc.PutAcl(ctx, "k1", "s1", types.WikiPageAclSaveRequest{
		Mode:          types.WikiPageAclModeAllowList,
		AllowUserIDs:  []string{"u-1"},
		DenyInherited: false,
		BaseRevision:  3,
	}, "alice", "owner")
	if err != nil {
		t.Fatalf("PutAcl(different): %v", err)
	}
	if updated.Revision != 4 {
		t.Fatalf("expected revision 4, got %d", updated.Revision)
	}
	if aclRepo.lastAction == "noop_match" {
		t.Fatalf("action must NOT be noop_match when payload differs, got %q", aclRepo.lastAction)
	}
	if len(cacheRepo.logEntries) != 1 {
		t.Fatalf("expected 1 invalidation log row, got %d", len(cacheRepo.logEntries))
	}
	if cacheRepo.deletesByKB < 1 {
		t.Fatalf("expected DeleteByKB to wipe at least 1 row, got %d", cacheRepo.deletesByKB)
	}
	if got := testutil.ToFloat64(metricAclChangeSkippedTotal.WithLabelValues("hash_match")) - skippedBefore; got != 0 {
		t.Fatalf("expected skipped counter unchanged on real wipe, got %v", got)
	}
	if got := testutil.ToFloat64(metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange))) - invBefore; got != 1 {
		t.Fatalf("expected invalidation counter +1, got %v", got)
	}
}

// Test 3 — HashAcl canonicalization: two ACL payloads whose allow_list
// slices are in different orders MUST hash to the same value, otherwise
// the skip optimization never fires in practice. A canonicalization
// regression here would silently disable Build #27.
func TestPutAcl_ReorderedAllowList_HashMatches(t *testing.T) {
	a := HashAcl(types.WikiPageAclModeAllowList, []string{"u-1", "u-2", "u-3"}, nil, false)
	b := HashAcl(types.WikiPageAclModeAllowList, []string{"u-3", "u-1", "u-2"}, nil, false)
	if a != b {
		t.Fatalf("HashAcl must canonicalize slice order: %q vs %q", a, b)
	}
	// Drive it through PutAcl to confirm the noop path actually fires.
	aclRepo := &aclStubRepo{
		acl: &types.WikiPageAcl{
			Mode:         types.WikiPageAclModeAllowList,
			AllowUserIDs: []string{"u-1", "u-2", "u-3"},
			Revision:     7,
		},
	}
	aclRepo.acl.SnapshotHash = a

	cacheRepo := newHarnessStubBacklinksCache()
	svc := newAclServiceWithCache(aclRepo, cacheRepo)
	ctx := withUserAndTenant(context.Background(), "alice", 1)

	_, err := svc.PutAcl(ctx, "k1", "s1", types.WikiPageAclSaveRequest{
		Mode:         types.WikiPageAclModeAllowList,
		AllowUserIDs: []string{"u-3", "u-1", "u-2"}, // reordered
		BaseRevision: 7,
	}, "alice", "owner")
	if err != nil {
		t.Fatalf("PutAcl(reordered): %v", err)
	}
	if aclRepo.lastAction != "noop_match" {
		t.Fatalf("expected noop_match on reordered allow_list, got %q", aclRepo.lastAction)
	}
}

// Test 4 — A5: a legacy row whose stored SnapshotHash is "" must always
// run the wipe. Spec D4 — backfill is "default empty, first write wipes",
// so any legacy row behaves identically to pre-B27 regardless of payload.
func TestPutAcl_LegacyRow_AlwaysWipes(t *testing.T) {
	aclRepo := &aclStubRepo{
		acl: &types.WikiPageAcl{
			Mode:          types.WikiPageAclModePrivate,
			AllowUserIDs:  nil,
			DenyInherited: false,
			Revision:      1,
			// SnapshotHash left empty → legacy row.
		},
	}

	cacheRepo := newHarnessStubBacklinksCache()
	seedStubRow(cacheRepo, "k1", "s1")
	svc := newAclServiceWithCache(aclRepo, cacheRepo)
	ctx := withUserAndTenant(context.Background(), "alice", 1)

	// Submit the EXACT SAME payload as the legacy row carries — still
	// must wipe because empty hash can never equal a real hash.
	_, err := svc.PutAcl(ctx, "k1", "s1", types.WikiPageAclSaveRequest{
		Mode:          types.WikiPageAclModePrivate,
		AllowUserIDs:  nil,
		DenyInherited: false,
		BaseRevision:  1,
	}, "alice", "owner")
	if err != nil {
		t.Fatalf("PutAcl(legacy): %v", err)
	}
	if aclRepo.lastAction == "noop_match" {
		t.Fatalf("legacy row must NOT be noop_match, got %q", aclRepo.lastAction)
	}
	if len(cacheRepo.logEntries) != 1 {
		t.Fatalf("expected invalidation log row on legacy wipe, got %d", len(cacheRepo.logEntries))
	}
}

// Test 5 — A4: a hash written by UpdateAclWithRevision is returned by
// the next GetAclBySlug. This is the round-trip that makes the skip
// optimization possible across multiple requests.
func TestPutAcl_HashPersistedAcrossReads(t *testing.T) {
	aclRepo := &aclStubRepo{
		acl: &types.WikiPageAcl{
			Mode:          types.WikiPageAclModePrivate,
			AllowUserIDs:  []string{"u-1"},
			DenyInherited: false,
			Revision:      1,
		},
	}
	cacheRepo := newHarnessStubBacklinksCache()
	svc := newAclServiceWithCache(aclRepo, cacheRepo)
	ctx := withUserAndTenant(context.Background(), "alice", 1)

	_, err := svc.PutAcl(ctx, "k1", "s1", types.WikiPageAclSaveRequest{
		Mode:          types.WikiPageAclModePrivate,
		AllowUserIDs:  []string{"u-1"},
		DenyInherited: false,
		BaseRevision:  1,
	}, "alice", "owner")
	if err != nil {
		t.Fatalf("PutAcl: %v", err)
	}
	expected := HashAcl(types.WikiPageAclModePrivate, []string{"u-1"}, nil, false)
	if aclRepo.acl.SnapshotHash != expected {
		t.Fatalf("expected stored SnapshotHash=%q, got %q", expected, aclRepo.acl.SnapshotHash)
	}

	// Round-trip through GetAclBySlug — the test stub mirrors the
	// production repo by copying SnapshotHash onto the returned ACL.
	got, err := aclRepo.GetAclBySlug(ctx, "k1", "s1")
	if err != nil {
		t.Fatalf("GetAclBySlug: %v", err)
	}
	if got.SnapshotHash != expected {
		t.Fatalf("round-trip SnapshotHash mismatch: stored=%q, read=%q", expected, got.SnapshotHash)
	}
}

// Test 6 — determinism: hashing the same ACL 1000 times produces the
// same string each iteration. A canonicalization regression (e.g. an
// accidental map iteration order in the canonical struct) would show up
// as a flake here.
func TestAclHash_Deterministic(t *testing.T) {
	first := HashAcl(types.WikiPageAclModeAllowList,
		[]string{"u-2", "u-1", "u-3"},
		[]string{"g-1", "g-2"},
		true)
	for i := 0; i < 1000; i++ {
		got := HashAcl(types.WikiPageAclModeAllowList,
			[]string{"u-3", "u-1", "u-2"},
			[]string{"g-2", "g-1"},
			true)
		if got != first {
			t.Fatalf("iteration %d: HashAcl drifted: first=%q, got=%q", i, first, got)
		}
	}
}
