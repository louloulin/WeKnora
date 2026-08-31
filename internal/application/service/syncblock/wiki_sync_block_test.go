//go:build syncblock

package syncblock

import (
	"context"
	"encoding/json"
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

type stubAuthz struct {
	mu       sync.Mutex
	readOK   bool
	writeOK  bool
}

func (s *stubAuthz) CanReadKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readOK, nil
}

func (s *stubAuthz) CanWriteKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeOK, nil
}

var sharedTestDB *gorm.DB

func TestMain(m *testing.M) {
	tmpFile := filepath.Join(os.TempDir(), "syncblock_test_shared.db")
	os.Remove(tmpFile)
	dsn := tmpFile + "?_busy_timeout=5000&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open sqlite: %v\n", err)
		os.Exit(1)
	}
	blockDDL := "CREATE TABLE IF NOT EXISTS wiki_sync_blocks ("
	blockDDL += "id INTEGER PRIMARY KEY AUTOINCREMENT,"
	blockDDL += "tenant_id INTEGER NOT NULL,"
	blockDDL += "kb_id VARCHAR(36) NOT NULL,"
	blockDDL += "block_id VARCHAR(36) NOT NULL,"
	blockDDL += "title VARCHAR(256) NOT NULL DEFAULT '',"
	blockDDL += "content_json TEXT NOT NULL,"
	blockDDL += "content_md TEXT NOT NULL DEFAULT '',"
	blockDDL += "version INTEGER NOT NULL DEFAULT 1,"
	blockDDL += "owner_id INTEGER NOT NULL,"
	blockDDL += "created_at DATETIME NOT NULL,"
	blockDDL += "updated_at DATETIME NOT NULL,"
	blockDDL += "UNIQUE (tenant_id, block_id))"
	refDDL := "CREATE TABLE IF NOT EXISTS wiki_sync_block_refs ("
	refDDL += "id INTEGER PRIMARY KEY AUTOINCREMENT,"
	refDDL += "tenant_id INTEGER NOT NULL,"
	refDDL += "kb_id VARCHAR(36) NOT NULL,"
	refDDL += "block_id VARCHAR(36) NOT NULL,"
	refDDL += "page_id VARCHAR(36) NOT NULL,"
	refDDL += "anchor_slug VARCHAR(256) NOT NULL DEFAULT '',"
	refDDL += "content_version INTEGER NOT NULL DEFAULT 0,"
	refDDL += "rendered_at DATETIME NOT NULL,"
	refDDL += "created_at DATETIME NOT NULL,"
	refDDL += "UNIQUE (tenant_id, block_id, page_id, anchor_slug))"
	for _, ddl := range []string{blockDDL, refDDL} {
		if err := db.Exec(ddl).Error; err != nil {
			fmt.Fprintf(os.Stderr, "create table: %v\n", err)
			os.Exit(1)
		}
	}
	sharedTestDB = db
	os.Exit(m.Run())
}

func newTestDB(t *testing.T) *gorm.DB { t.Helper(); return sharedTestDB }

func newSvc(t *testing.T, ok bool) *service.WikiSyncBlockService {
	t.Helper()
	if err := sharedTestDB.Exec("DELETE FROM wiki_sync_blocks").Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if err := sharedTestDB.Exec("DELETE FROM wiki_sync_block_refs").Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	return service.NewWikiSyncBlockService(
		repository.NewWikiSyncBlockRepository(sharedTestDB),
		repository.NewWikiSyncBlockRefRepository(sharedTestDB),
		&stubAuthz{readOK: ok, writeOK: ok},
	)
}

func uuidLike(i int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
}

func TestCreateCanonical_InsertsRow(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	row, err := svc.CreateCanonical(ctx, types.WikiSyncBlockUpsert{
		TenantID:    1,
		KBID:        "kb-1",
		BlockID:     uuidLike(1),
		Title:       "Disclaimer",
		ContentJSON: json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","text":"hello"}]}`),
		ContentMD:   "hello",
		OwnerID:     42,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Version != 1 {
		t.Fatalf("version = %d, want 1", row.Version)
	}
	if row.ContentMD != "hello" {
		t.Fatalf("md = %q, want %q", row.ContentMD, "hello")
	}
}

func TestUpdateCanonical_BumpsVersionAndCacheInvalidates(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	in := types.WikiSyncBlockUpsert{
		TenantID: 1, KBID: "kb-1", BlockID: uuidLike(2),
		Title: "v1", ContentJSON: json.RawMessage(`{"v":1}`), ContentMD: "v1", OwnerID: 42,
	}
	if _, err := svc.CreateCanonical(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Prime the cache.
	if _, err := svc.GetCanonical(ctx, 1, uuidLike(2)); err != nil {
		t.Fatalf("get: %v", err)
	}
	// Update with new content.
	in.Title = "v2"
	in.ContentMD = "v2"
	in.ContentJSON = json.RawMessage(`{"v":2}`)
	row, err := svc.UpdateCanonical(ctx, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if row.Version < 2 {
		t.Fatalf("version after update = %d, want >= 2", row.Version)
	}
	// Re-read should pick up v2 (cache was invalidated).
	got, err := svc.GetCanonical(ctx, 1, uuidLike(2))
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if got.ContentMD != "v2" {
		t.Fatalf("got md = %q, want %q", got.ContentMD, "v2")
	}
}

func TestSyncPageRefs_RecordsEmbeddedBlocks(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	if _, err := svc.CreateCanonical(ctx, types.WikiSyncBlockUpsert{
		TenantID: 1, KBID: "kb-1", BlockID: uuidLike(3),
		ContentJSON: json.RawMessage(`{"t":"a"}`), OwnerID: 42,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	content := "intro\n\n[[sync:" + uuidLike(3) + "]]\n\noutro"
	if err := svc.SyncPageRefs(ctx, 1, "kb-1", "page-1", content); err != nil {
		t.Fatalf("sync refs: %v", err)
	}
	stats, err := svc.Stats(ctx, 1, uuidLike(3))
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.RefCount != 1 {
		t.Fatalf("ref count = %d, want 1", stats.RefCount)
	}
	if stats.CurrentVersion != 1 {
		t.Fatalf("current version = %d, want 1", stats.CurrentVersion)
	}
}

func TestParseSyncMarkers_ExtractsAll(t *testing.T) {
	content := "before [[sync:" + uuidLike(4) + "]] middle [[sync:" + uuidLike(5) + "]] after"
	markers := service.ParseSyncMarkers(content)
	if len(markers) != 2 {
		t.Fatalf("markers = %d, want 2", len(markers))
	}
	if markers[0].BlockID != uuidLike(4) || markers[1].BlockID != uuidLike(5) {
		t.Fatalf("marker ids wrong: %+v", markers)
	}
}

func TestParseSyncMarkers_Empty(t *testing.T) {
	if got := service.ParseSyncMarkers("plain content with no markers"); len(got) != 0 {
		t.Fatalf("got %d markers, want 0", len(got))
	}
}

func TestDeleteCanonical_CascadeRemovesRefs(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	if _, err := svc.CreateCanonical(ctx, types.WikiSyncBlockUpsert{
		TenantID: 1, KBID: "kb-1", BlockID: uuidLike(6),
		ContentJSON: json.RawMessage(`{}`), OwnerID: 42,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.SyncPageRefs(ctx, 1, "kb-1", "page-1", "[[sync:"+uuidLike(6)+"]]"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := svc.DeleteCanonical(ctx, 1, 42, "kb-1", uuidLike(6), "cascade"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	refs, err := svc.ListRefsForBlock(ctx, 1, uuidLike(6))
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("refs after cascade = %d, want 0", len(refs))
	}
}

func TestDeleteCanonical_RejectsUnknownMode(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	err := svc.DeleteCanonical(ctx, 1, 42, "kb-1", "any", "unknown-mode")
	if err == nil {
		t.Fatalf("expected error for unknown mode")
	}
}

func TestGetCanonical_MissingReturnsNil(t *testing.T) {
	svc := newSvc(t, true)
	got, err := svc.GetCanonical(context.Background(), 1, "missing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("got = %+v, want nil", got)
	}
}

func TestListForKB_OrdersByUpdatedDesc(t *testing.T) {
	svc := newSvc(t, true)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		_, err := svc.CreateCanonical(ctx, types.WikiSyncBlockUpsert{
			TenantID: 1, KBID: "kb-1", BlockID: uuidLike(100 + i),
			ContentJSON: json.RawMessage(`{}`), OwnerID: 42,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	rows, err := svc.ListForKB(ctx, 1, "kb-1", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
}

func TestUpsertValidation(t *testing.T) {
	cases := []struct {
		name string
		in   types.WikiSyncBlockUpsert
		ok   bool
	}{
		{"missing tenant", types.WikiSyncBlockUpsert{KBID: "k", BlockID: "b", ContentJSON: json.RawMessage(`{}`), OwnerID: 1}, false},
		{"missing kb", types.WikiSyncBlockUpsert{TenantID: 1, BlockID: "b", ContentJSON: json.RawMessage(`{}`), OwnerID: 1}, false},
		{"missing block", types.WikiSyncBlockUpsert{TenantID: 1, KBID: "k", ContentJSON: json.RawMessage(`{}`), OwnerID: 1}, false},
		{"missing content", types.WikiSyncBlockUpsert{TenantID: 1, KBID: "k", BlockID: "b", OwnerID: 1}, false},
		{"missing owner", types.WikiSyncBlockUpsert{TenantID: 1, KBID: "k", BlockID: "b", ContentJSON: json.RawMessage(`{}`)}, false},
		{"valid", types.WikiSyncBlockUpsert{TenantID: 1, KBID: "k", BlockID: "b", ContentJSON: json.RawMessage(`{}`), OwnerID: 1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestValidateJSONContent(t *testing.T) {
	if _, err := service.ValidateJSONContent(nil); err != nil {
		t.Fatalf("nil raw should yield empty obj: %v", err)
	}
	if _, err := service.ValidateJSONContent([]byte("not json")); err == nil {
		t.Fatalf("invalid json should error")
	}
	if _, err := service.ValidateJSONContent([]byte(`{"x":1}`)); err != nil {
		t.Fatalf("valid json should pass: %v", err)
	}
}
