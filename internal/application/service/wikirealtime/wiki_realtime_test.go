//go:build wikirealtime

package wikirealtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// stubAuthz is a minimal WikiRealtimeAuthorizer for unit tests. It records
// the calls so tests can assert on read/write access patterns.
type stubAuthz struct {
	mu       sync.Mutex
	readOK   bool
	writeOK  bool
	readErr  error
	writeErr error
}

func (s *stubAuthz) CanRead(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readOK, s.readErr
}

func (s *stubAuthz) CanWrite(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeOK, s.writeErr
}

// sharedTestDB is initialized once per package via TestMain; all tests
// use the same connection pool so the SQLite in-memory database is
// consistently visible across the GORM connection pool.
var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("wikirealtime_test_shared.db"))
	os.Remove(tmpFile) // start fresh
	dsn := tmpFile + "?_busy_timeout=5000&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	db = db.Session(&gorm.Session{Initialized: true})
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open sqlite: %v\n", err)
		os.Exit(1)
	}
	snapshotDDL := "CREATE TABLE IF NOT EXISTS wiki_doc_snapshots ("
	snapshotDDL += "id INTEGER PRIMARY KEY AUTOINCREMENT,"
	snapshotDDL += "tenant_id INTEGER NOT NULL,"
	snapshotDDL += "kb_id VARCHAR(36) NOT NULL,"
	snapshotDDL += "page_id VARCHAR(36) NOT NULL,"
	snapshotDDL += "ydoc_state BLOB NOT NULL,"
	snapshotDDL += "vector_clock BLOB NOT NULL,"
	snapshotDDL += "version INTEGER NOT NULL DEFAULT 1,"
	snapshotDDL += "size_bytes INTEGER NOT NULL,"
	snapshotDDL += "created_at DATETIME NOT NULL,"
	snapshotDDL += "updated_at DATETIME NOT NULL,"
	snapshotDDL += "UNIQUE (tenant_id, kb_id, page_id))"
	sessionDDL := "CREATE TABLE IF NOT EXISTS wiki_realtime_sessions ("
	sessionDDL += "id VARCHAR(36) PRIMARY KEY,"
	sessionDDL += "tenant_id INTEGER NOT NULL,"
	sessionDDL += "page_id VARCHAR(36) NOT NULL,"
	sessionDDL += "user_id INTEGER NOT NULL,"
	sessionDDL += "client_id INTEGER NOT NULL,"
	sessionDDL += "color VARCHAR(16) NOT NULL DEFAULT '#58a6ff',"
	sessionDDL += "display_name VARCHAR(128) NOT NULL DEFAULT '',"
	sessionDDL += "last_heartbeat DATETIME NOT NULL,"
	sessionDDL += "joined_at DATETIME NOT NULL,"
	sessionDDL += "UNIQUE (tenant_id, page_id, client_id))"
	for _, ddl := range []string{snapshotDDL, sessionDDL} {
		if err := db.Exec(ddl).Error; err != nil {
			fmt.Fprintf(os.Stderr, "create table: %v\n", err)
			os.Exit(1)
		}
	}
	sharedTestDB = db
	os.Exit(m.Run()) // keep file for debugging
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return sharedTestDB
}


func TestLoadDoc_EmptyCacheAndStore_ReturnsNil(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	state, err := svc.LoadDoc(context.Background(), 1, "kb-1", "page-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for fresh page, got %d bytes", len(state))
	}
}

func TestMergeUpdate_AccumulatesState(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	ctx := context.Background()

	delta1 := []byte("hello-")
	delta2 := []byte("world")

	state1, err := svc.MergeUpdate(ctx, 1, "kb-1", "page-1", delta1)
	if err != nil {
		t.Fatalf("merge1: %v", err)
	}
	if string(state1) != "hello-" {
		t.Fatalf("state1 = %q, want %q", state1, "hello-")
	}

	state2, err := svc.MergeUpdate(ctx, 1, "kb-1", "page-1", delta2)
	if err != nil {
		t.Fatalf("merge2: %v", err)
	}
	if string(state2) != "hello-world" {
		t.Fatalf("state2 = %q, want %q", state2, "hello-world")
	}
}

func TestMergeUpdate_EmptyDelta_NoOp(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	ctx := context.Background()

	// Seed with real data first.
	if _, err := svc.MergeUpdate(ctx, 1, "kb-1", "page-1", []byte("seed")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Empty delta should not change state.
	state, err := svc.MergeUpdate(ctx, 1, "kb-1", "page-1", nil)
	if err != nil {
		t.Fatalf("empty merge: %v", err)
	}
	if string(state) != "seed" {
		t.Fatalf("empty merge changed state: %q", state)
	}
}

func TestTouchSession_UpsertsPresence(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	ctx := context.Background()

	id1, err := svc.TouchSession(ctx, 7, "page-1", 42, 1001, "#58a6ff", "Alice")
	if err != nil {
		t.Fatalf("touch1: %v", err)
	}
	if id1 == "" {
		t.Fatalf("touch1 returned empty id")
	}

	// Second touch for same (tenant, page, client) should upsert.
	id2, err := svc.TouchSession(ctx, 7, "page-1", 42, 1001, "#f85149", "Alice Renamed")
	if err != nil {
		t.Fatalf("touch2: %v", err)
	}
	if id2 == "" {
		t.Fatalf("touch2 returned empty id")
	}

	rows, err := svc.ListPresence(ctx, 7, "page-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 presence row, got %d", len(rows))
	}
	if rows[0].DisplayName != "Alice Renamed" {
		t.Fatalf("display name = %q, want %q", rows[0].DisplayName, "Alice Renamed")
	}
	if rows[0].Color != "#f85149" {
		t.Fatalf("color = %q, want %q", rows[0].Color, "#f85149")
	}
}

func TestSweepIdle_RemovesStaleSessions(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	// Clean slate — previous tests may have left presence rows.
	if err := db.Exec("DELETE FROM wiki_realtime_sessions").Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	ctx := context.Background()

	// Insert directly with an old heartbeat so the sweeper picks it up.
	// Use raw SQL so the connection matches the SweepOlderThan raw SQL path.
	if err := db.Exec(
		`INSERT INTO wiki_realtime_sessions (id, tenant_id, page_id, user_id, client_id, color, display_name, last_heartbeat, joined_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wrs_old", 7, "page-1", 99, 9999, "#000000", "Stale",
		time.Now().UTC().Add(-2*time.Minute), time.Now().UTC().Add(-3*time.Minute),
	).Error; err != nil {
		t.Fatalf("seed old: %v", err)
	}
	// Verify the row is visible
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM wiki_realtime_sessions WHERE id = ?", "wrs_old").Scan(&n).Error; err != nil {
		t.Fatalf("verify seed: %v", err)
	}
	if n != 1 {
		t.Fatalf("seed not visible: count=%d", n)
	}
	// Fresh row should survive.
	if _, err := svc.TouchSession(ctx, 7, "page-1", 1, 1, "#ffffff", "Fresh"); err != nil {
		t.Fatalf("touch fresh: %v", err)
	}

	n, err := svc.SweepIdle(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("sweep removed %d rows, want 1", n)
	}

	rows, err := svc.ListPresence(ctx, 7, "page-1")
	if err != nil {
		t.Fatalf("list post-sweep: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row post-sweep, got %d", len(rows))
	}
	if rows[0].DisplayName != "Fresh" {
		t.Fatalf("kept wrong row: %q", rows[0].DisplayName)
	}
}

func TestValidateReadAccess_DeniesWithoutAuthz(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: false},
	)
	err := svc.ValidateReadAccess(context.Background(), 1, 1, "kb-1", "page-1")
	if err == nil {
		t.Fatalf("expected read denial")
	}
}

func TestValidateReadAccess_PropagatesError(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readErr: errors.New("authz boom")},
	)
	err := svc.ValidateReadAccess(context.Background(), 1, 1, "kb-1", "page-1")
	if err == nil {
		t.Fatalf("expected propagated error")
	}
}

func TestStats_TracksCounters(t *testing.T) {
	db := newTestDB(t)
	svc := service.NewWikiRealtimeService(
		repository.NewWikiRealtimeSnapshotRepository(db),
		repository.NewWikiRealtimeSessionRepository(db),
		&stubAuthz{readOK: true, writeOK: true},
	)
	ctx := context.Background()
	if _, err := svc.MergeUpdate(ctx, 1, "kb-1", "page-1", []byte("a")); err != nil {
		t.Fatalf("merge: %v", err)
	}
	svc.IncrementOut()
	svc.IncrementOut()

	stats := svc.Stats()
	if stats.UpdatesIn != 1 {
		t.Fatalf("UpdatesIn = %d, want 1", stats.UpdatesIn)
	}
	if stats.UpdatesOut != 2 {
		t.Fatalf("UpdatesOut = %d, want 2", stats.UpdatesOut)
	}
	if stats.ActiveDocs != 1 {
		t.Fatalf("ActiveDocs = %d, want 1", stats.ActiveDocs)
	}
}

func TestSnapshotUpsert_Validates(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewWikiRealtimeSnapshotRepository(db)
	cases := []struct {
		name string
		in   types.WikiRealtimeSnapshotUpsert
		ok   bool
	}{
		{"missing tenant", types.WikiRealtimeSnapshotUpsert{KBID: "k", PageID: "p", YDocState: []byte("x"), VectorClock: []byte{}, SizeBytes: 1}, false},
		{"missing kb", types.WikiRealtimeSnapshotUpsert{TenantID: 1, PageID: "p", YDocState: []byte("x"), VectorClock: []byte{}, SizeBytes: 1}, false},
		{"missing page", types.WikiRealtimeSnapshotUpsert{TenantID: 1, KBID: "k", YDocState: []byte("x"), VectorClock: []byte{}, SizeBytes: 1}, false},
		{"missing state", types.WikiRealtimeSnapshotUpsert{TenantID: 1, KBID: "k", PageID: "p", VectorClock: []byte{}, SizeBytes: 1}, false},
		{"zero size", types.WikiRealtimeSnapshotUpsert{TenantID: 1, KBID: "k", PageID: "p", YDocState: []byte("x"), VectorClock: []byte{}}, false},
		{"valid", types.WikiRealtimeSnapshotUpsert{TenantID: 1, KBID: "k", PageID: "p", YDocState: []byte("x"), VectorClock: []byte{}, SizeBytes: 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.Upsert(context.Background(), tc.in)
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestSnapshotUpsert_ReplaceExisting(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewWikiRealtimeSnapshotRepository(db)
	ctx := context.Background()
	in := types.WikiRealtimeSnapshotUpsert{
		TenantID: 1, KBID: "k", PageID: "p",
		YDocState: []byte("first"), VectorClock: []byte{}, SizeBytes: 5,
	}
	if _, err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	in.YDocState = []byte("second-with-more-bytes")
	in.SizeBytes = len(in.YDocState)
	if _, err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	got, err := repo.Get(ctx, 1, "k", "p")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.YDocState) != string(in.YDocState) {
		t.Fatalf("got %q, want %q", got.YDocState, in.YDocState)
	}
	if got.Version < 1 {
		t.Fatalf("version should advance, got %d", got.Version)
	}
}
