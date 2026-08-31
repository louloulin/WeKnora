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

// fakeWikiBacklinksCacheRepo is an in-memory implementation of the
// Build #22 cleanup-relevant subset of WikiBacklinksCacheRepository.
// It records what the service called so the test can assert the
// expected policy without spinning up a real DB.
//
// We only implement the 3 cleanup methods + Get (used by ListByKB and
// not relevant here, but the interface needs it). The other interface
// methods are stubs that error — the cleanup path doesn't call them.
type fakeWikiBacklinksCacheRepo struct {
	mu sync.Mutex

	// rows: kbID + slug → cache row
	rows map[string]*types.WikiBacklinksCacheRow

	// counters for assertions
	deleteStaleCalls       int
	deleteStaleLastBefore  time.Time
	deleteStaleLastLimit   int
	countRowsCalls         int
	listStaleForUpdateHits []string // "kbID\x00slug"
}

func newFakeCacheRepo() *fakeWikiBacklinksCacheRepo {
	return &fakeWikiBacklinksCacheRepo{
		rows: map[string]*types.WikiBacklinksCacheRow{},
	}
}

func (f *fakeWikiBacklinksCacheRepo) seed(kbID, slug string, updatedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[kbID+"\x00"+slug] = &types.WikiBacklinksCacheRow{
		KbID:       kbID,
		Slug:       slug,
		UpdatedAt:  updatedAt,
		ComputedAt: updatedAt,
	}
}

func (f *fakeWikiBacklinksCacheRepo) liveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

// --- interface methods actually called by CleanupService ---

func (f *fakeWikiBacklinksCacheRepo) DeleteStale(
	_ context.Context, before time.Time, limit int,
) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteStaleCalls++
	f.deleteStaleLastBefore = before
	f.deleteStaleLastLimit = limit
	var deleted int64
	for k, row := range f.rows {
		if row.UpdatedAt.Before(before) {
			delete(f.rows, k)
			deleted++
			if int(deleted) >= limit {
				break
			}
		}
	}
	return deleted, nil
}

func (f *fakeWikiBacklinksCacheRepo) CountRows(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.countRowsCalls++
	return int64(len(f.rows)), nil
}

func (f *fakeWikiBacklinksCacheRepo) CountBackrefRows(_ context.Context) (int64, error) {
	// Cleanup test fake doesn't track a separate backref set — for the
	// gauge refresh path we just return the cache row count as a
	// stand-in. The real impl queries the backref table.
	return 0, nil
}

func (f *fakeWikiBacklinksCacheRepo) ListStaleForUpdate(
	_ context.Context, _ *gorm.DB, before time.Time, limit int,
) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0)
	for k, row := range f.rows {
		if row.UpdatedAt.Before(before) {
			out = append(out, k)
			if len(out) >= limit {
				break
			}
		}
	}
	f.listStaleForUpdateHits = out
	return out, nil
}

// --- interface methods not exercised by the cleanup path ---
// These stubs return errors so a stray call from a test refactor is
// loud, not silent.

func (f *fakeWikiBacklinksCacheRepo) Get(_ context.Context, _, _ string) (*types.WikiBacklinksCacheRow, error) {
	return nil, errors.New("fakeWikiBacklinksCacheRepo.Get: not implemented in cleanup test")
}
func (f *fakeWikiBacklinksCacheRepo) Upsert(_ context.Context, _ *types.WikiBacklinksCacheRow) error {
	return errors.New("not implemented in cleanup test")
}
func (f *fakeWikiBacklinksCacheRepo) Delete(_ context.Context, _ string, _ []string) (int64, error) {
	return 0, errors.New("not implemented in cleanup test")
}
func (f *fakeWikiBacklinksCacheRepo) ListByKB(_ context.Context, _ string, _, _ int) ([]*types.WikiBacklinksCacheStatus, int64, error) {
	return nil, 0, errors.New("not implemented in cleanup test")
}

// --- tests ---

// TestCleanupService_EmptySweep: an empty table yields 0 deletions
// and dry-run returns the count as 0 too. The gauge refresh happens
// in both paths.
func TestCleanupService_EmptySweep(t *testing.T) {
	repo := newFakeCacheRepo()
	svc := NewWikiBacklinksCacheCleanupService(
		WikiBacklinksCacheCleanupConfig{
			TTL:       30 * 24 * time.Hour,
			Period:    24 * time.Hour,
			BatchSize: 100,
			DryRun:    false,
			MaxRows:   1_000_000,
		},
		nil, // DB unused in real-cleanup path with this fake
		repo,
	)

	deleted, dryRun, _, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted on empty table, got %d", deleted)
	}
	if dryRun != false {
		t.Errorf("expected dryRun=false, got true")
	}
	if repo.deleteStaleCalls != 1 {
		t.Errorf("expected 1 DeleteStale call, got %d", repo.deleteStaleCalls)
	}
	if repo.countRowsCalls < 1 {
		t.Errorf("expected at least 1 CountRows call (gauge refresh), got %d", repo.countRowsCalls)
	}
}

// TestCleanupService_PartialDelete: only rows older than TTL are
// deleted. Rows at the boundary (exactly TTL ago) survive because
// the comparison is strict `<`.
func TestCleanupService_PartialDelete(t *testing.T) {
	repo := newFakeCacheRepo()
	now := time.Now().UTC()

	// Three rows: one very old (40d), one at the TTL boundary (30d
	// exactly — should survive because the comparison is strict),
	// one fresh (1d).
	repo.seed("kb1", "old", now.Add(-40*24*time.Hour))
	repo.seed("kb1", "boundary", now.Add(-30*24*time.Hour))
	repo.seed("kb1", "fresh", now.Add(-1*24*time.Hour))

	svc := NewWikiBacklinksCacheCleanupService(
		WikiBacklinksCacheCleanupConfig{
			TTL:       30 * 24 * time.Hour,
			BatchSize: 100,
			DryRun:    false,
		},
		nil,
		repo,
	)

	deleted, _, _, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected exactly 1 deletion (the 40d-old row), got %d", deleted)
	}
	if repo.liveCount() != 2 {
		t.Errorf("expected 2 surviving rows, got %d", repo.liveCount())
	}
	// Confirm the surviving rows are the boundary + fresh ones.
	if _, ok := repo.rows["kb1\x00boundary"]; !ok {
		t.Errorf("expected boundary row to survive")
	}
	if _, ok := repo.rows["kb1\x00fresh"]; !ok {
		t.Errorf("expected fresh row to survive")
	}
	if _, ok := repo.rows["kb1\x00old"]; ok {
		t.Errorf("expected old row to be deleted")
	}
}

// TestCleanupService_DryRunSkipsDelete: in dry-run mode, the service
// counts stale rows via ListStaleForUpdate but never invokes
// DeleteStale. The count is observable through the return value.
func TestCleanupService_DryRunSkipsDelete(t *testing.T) {
	repo := newFakeCacheRepo()
	now := time.Now().UTC()

	// Five rows, all older than TTL.
	for i := 0; i < 5; i++ {
		repo.seed("kb1", "stale-"+string(rune('a'+i)), now.Add(-40*24*time.Hour))
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

	deleted, dryRun, _, err := svc.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !dryRun {
		t.Errorf("expected dryRun=true, got false")
	}
	if deleted != 5 {
		t.Errorf("expected dry-run to report 5 stale rows, got %d", deleted)
	}
	// DeleteStale must NOT have been called.
	if repo.deleteStaleCalls != 0 {
		t.Errorf("expected 0 DeleteStale calls in dry-run, got %d", repo.deleteStaleCalls)
	}
	// All 5 rows must still be present.
	if repo.liveCount() != 5 {
		t.Errorf("expected 5 surviving rows after dry-run, got %d", repo.liveCount())
	}
	// ListStaleForUpdate was called once for the count.
	if len(repo.listStaleForUpdateHits) != 5 {
		t.Errorf("expected ListStaleForUpdate to surface 5 keys, got %d", len(repo.listStaleForUpdateHits))
	}
}

// CountByKB satisfies the new interfaces.WikiBacklinksCacheRepository method.
func (r *fakeWikiBacklinksCacheRepo) CountByKB(_ context.Context, _ string) (int64, error) { return 0, nil }

func (r *fakeWikiBacklinksCacheRepo) DeleteByKB(_ context.Context, _ string) (int64, error) { return 0, nil }

func (r *fakeWikiBacklinksCacheRepo) ListInvalidationLog(_ context.Context, _ string, _, _ int) ([]*types.WikiBacklinksCacheInvalidationLogEntry, int64, error) { return nil, 0, nil }
