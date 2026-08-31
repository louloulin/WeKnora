// Package repository - v0.7.26 collab_doc_file repo integration test.
//
// Drives the binary repo against an in-memory SQLite DB to verify:
//   - SaveFile auto-increments version
//   - GetLatestFile / GetFileByVersion return the right row
//   - ListByDoc returns descending versions
//   - CurrentVersion returns the max
//   - DeleteByDoc removes every row
package repository

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&types.CollabDocFile{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCollabDocFileRepoSaveAndGet(t *testing.T) {
	db := newRepoTestDB(t)
	r := NewCollabDocFileRepository(db)
	tenant := uint64(1)
	docID := "doc-1"

	// Save v1
	v1, err := r.SaveFile(nil, types.CollabDocFileUpsert{
		TenantID: tenant,
		DocID:    docID,
		Format:   types.CollaborativeDocKindDoc,
		Content:  []byte("hello"),
		Version:  0, // auto
	})
	if err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if v1.Version != 1 {
		t.Errorf("expected v1, got %d", v1.Version)
	}

	// Save v2
	v2, err := r.SaveFile(nil, types.CollabDocFileUpsert{
		TenantID: tenant,
		DocID:    docID,
		Format:   types.CollaborativeDocKindDoc,
		Content:  []byte("hello v2"),
		Version:  0,
	})
	if err != nil {
		t.Fatalf("save v2: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("expected v2, got %d", v2.Version)
	}

	// CurrentVersion
	cur, err := r.CurrentVersion(nil, tenant, docID)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if cur != 2 {
		t.Errorf("expected current=2, got %d", cur)
	}

	// GetLatestFile
	latest, err := r.GetLatestFile(nil, tenant, docID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil || string(latest.Content) != "hello v2" {
		t.Errorf("latest content wrong: %+v", latest)
	}

	// GetFileByVersion
	v1Row, err := r.GetFileByVersion(nil, tenant, docID, 1)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if v1Row == nil || string(v1Row.Content) != "hello" {
		t.Errorf("v1 content wrong: %+v", v1Row)
	}

	// ListByDoc
	rows, err := r.ListByDoc(nil, tenant, docID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Version != 2 || rows[1].Version != 1 {
		t.Errorf("list order wrong: %+v", rows)
	}

	// DeleteByDoc
	if err := r.DeleteByDoc(nil, tenant, docID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows2, _ := r.ListByDoc(nil, tenant, docID)
	if len(rows2) != 0 {
		t.Errorf("expected empty after delete, got %d", len(rows2))
	}
}

func TestCollabDocFileRepoValidation(t *testing.T) {
	db := newRepoTestDB(t)
	r := NewCollabDocFileRepository(db)
	// Empty content -> Validate() error
	_, err := r.SaveFile(nil, types.CollabDocFileUpsert{
		TenantID: 1,
		DocID:    "doc",
		Format:   types.CollaborativeDocKindDoc,
		Content:  nil,
		Version:  1,
	})
	if err == nil {
		t.Errorf("expected error for empty content")
	}
}

func TestCollabDocFileRepoVersionConflict(t *testing.T) {
	db := newRepoTestDB(t)
	r := NewCollabDocFileRepository(db)
	_, err := r.SaveFile(nil, types.CollabDocFileUpsert{
		TenantID: 1, DocID: "doc",
		Format: types.CollaborativeDocKindDoc,
		Content: []byte("a"), Version: 1,
	})
	if err != nil {
		t.Fatalf("save 1: %v", err)
	}
	// Re-save at version=1 should fail because of UNIQUE(doc_id, version).
	_, err = r.SaveFile(nil, types.CollabDocFileUpsert{
		TenantID: 1, DocID: "doc",
		Format: types.CollaborativeDocKindDoc,
		Content: []byte("b"), Version: 1,
	})
	if err == nil {
		t.Errorf("expected UNIQUE conflict on duplicate version")
	}
}

func TestCollabDocFileRepoPurgeFilesOlderThan(t *testing.T) {
	db := newRepoTestDB(t)
	r := NewCollabDocFileRepository(db)
	tenant := uint64(1)
	docID := "doc-purge"

	// Seed 5 versions for one doc.
	for v := 1; v <= 5; v++ {
		_, err := r.SaveFile(nil, types.CollabDocFileUpsert{
			TenantID: tenant,
			DocID:    docID,
			Format:   types.CollaborativeDocKindDoc,
			Content:  []byte("content-v" + string(rune('0'+v))),
			Version:  v,
		})
		if err != nil {
			t.Fatalf("save v%d: %v", v, err)
		}
	}
	// Sanity: latest = v5
	latest, err := r.GetLatestFile(nil, tenant, docID)
	if err != nil || latest.Version != 5 {
		t.Fatalf("expected latest v5, got %+v err=%v", latest, err)
	}

	// Purge rows older than now (everything) keeping latest 2.
	cutoff := time.Now().Add(1 * time.Hour)
	n, err := r.PurgeFilesOlderThan(nil, cutoff, 2)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 rows purged (v1/v2/v3), got %d", n)
	}

	// Surviving versions: v4 + v5 only.
	remaining, err := r.ListByDoc(nil, tenant, docID)
	if err != nil {
		t.Fatalf("list after purge: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining rows, got %d", len(remaining))
	}
	got := map[int]bool{}
	for _, row := range remaining {
		got[row.Version] = true
	}
	if !got[4] || !got[5] {
		t.Errorf("expected v4+v5 to remain, got %v", got)
	}

	// Latest is still served.
	latest, err = r.GetLatestFile(nil, tenant, docID)
	if err != nil || latest.Version != 5 {
		t.Errorf("latest broken after purge: %+v err=%v", latest, err)
	}
}
