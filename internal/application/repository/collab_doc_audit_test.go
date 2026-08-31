// Package repository - v0.7.30 collab_doc_audit_log repo integration test.
package repository

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&types.CollabDocAuditEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAuditRepoRecordAndGet(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	tenant := uint64(7)
	doc := "doc-a1"

	entry, err := r.Record(nil, types.RecordAuditRequest{ //nolint:staticcheck // ctx not needed for in-mem sqlite
		TenantID:    tenant,
		DocID:       doc,
		ActorUserID: 42,
		ActorName:   "Alice",
		Action:      types.AuditActionSave,
		Target:      "v3",
		Payload:     `{"size_bytes":1024}`,
		IP:          "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if entry.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if entry.Action != types.AuditActionSave {
		t.Fatalf("action mismatch: %s", entry.Action)
	}
	if entry.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}

	got, err := r.Get(nil, tenant, entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != entry.ID {
		t.Fatalf("Get returned %+v, want %d", got, entry.ID)
	}
}

func TestAuditRepoInvalidActionRejected(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	_, err := r.Record(nil, types.RecordAuditRequest{ //nolint:staticcheck
		TenantID: 1,
		DocID:    "doc-x",
		Action:   types.CollabDocAuditAction("bogus.action"),
	})
	if err == nil {
		t.Fatal("expected invalid action to be rejected")
	}
}

func TestAuditRepoEmptyDocIDNormalized(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	entry, err := r.Record(nil, types.RecordAuditRequest{ //nolint:staticcheck
		TenantID: 1,
		DocID:    "", // tenant-scoped event
		Action:   types.AuditActionExport,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if entry.DocID != "*" {
		t.Fatalf("expected DocID '*', got %q", entry.DocID)
	}
}

func TestAuditRepoListAndCount(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	tenant := uint64(1)
	doc := "doc-L"

	for i := 0; i < 5; i++ {
		if _, err := r.Record(nil, types.RecordAuditRequest{ //nolint:staticcheck
			TenantID: tenant,
			DocID:    doc,
			Action:   types.AuditActionSave,
			Target:   "v" + string(rune('1'+i)),
		}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond) // ensure distinct created_at
	}

	filter := types.ListCollabDocAuditFilter{DocID: doc}
	rows, err := r.List(nil, tenant, filter)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
	// Newest first
	if rows[0].Target != "v5" {
		t.Fatalf("expected most recent (v5) first, got %s", rows[0].Target)
	}

	n, err := r.Count(nil, tenant, filter)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 5 {
		t.Fatalf("Count: %d, want 5", n)
	}
}

func TestAuditRepoSummary(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	tenant := uint64(1)
	doc := "doc-S"

	// 3 saves, 1 share enable.
	entries := []types.RecordAuditRequest{
		{TenantID: tenant, DocID: doc, Action: types.AuditActionSave, Target: "v1"},
		{TenantID: tenant, DocID: doc, Action: types.AuditActionSave, Target: "v2"},
		{TenantID: tenant, DocID: doc, Action: types.AuditActionSave, Target: "v3"},
		{TenantID: tenant, DocID: doc, Action: types.AuditActionShareEnable},
	}
	for _, e := range entries {
		if _, err := r.Record(nil, types.RecordAuditRequest(e)); err != nil { //nolint:staticcheck
			t.Fatalf("Record: %v", err)
		}
	}

	out, err := r.Summary(nil, tenant, types.ListCollabDocAuditFilter{DocID: doc})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if out.TotalEntries != 4 {
		t.Fatalf("TotalEntries: %d", out.TotalEntries)
	}
	if out.ByAction[types.AuditActionSave] != 3 {
		t.Fatalf("ByAction[save]: %d, want 3", out.ByAction[types.AuditActionSave])
	}
	if out.ByAction[types.AuditActionShareEnable] != 1 {
		t.Fatalf("ByAction[share.enable]: %d, want 1", out.ByAction[types.AuditActionShareEnable])
	}
	if len(out.ByDay) == 0 {
		t.Fatal("ByDay should have at least one row")
	}
}

func TestAuditRepoFilterByActor(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	tenant := uint64(1)

	if _, err := r.Record(nil, types.RecordAuditRequest{TenantID: tenant, DocID: "d1", ActorUserID: 11, Action: types.AuditActionSave}); err != nil { //nolint:staticcheck
		t.Fatal(err)
	}
	if _, err := r.Record(nil, types.RecordAuditRequest{TenantID: tenant, DocID: "d1", ActorUserID: 22, Action: types.AuditActionSave}); err != nil { //nolint:staticcheck
		t.Fatal(err)
	}

	rows, err := r.List(nil, tenant, types.ListCollabDocAuditFilter{DocID: "d1", ActorUserID: 11})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for actor=11, got %d", len(rows))
	}
	if rows[0].ActorUserID != 11 {
		t.Fatalf("actor mismatch: %d", rows[0].ActorUserID)
	}
}

func TestAuditRepoDeleteByDoc(t *testing.T) {
	db := newAuditTestDB(t)
	r := NewCollabDocAuditRepository(db)
	tenant := uint64(1)

	if _, err := r.Record(nil, types.RecordAuditRequest{TenantID: tenant, DocID: "d1", Action: types.AuditActionSave}); err != nil { //nolint:staticcheck
		t.Fatal(err)
	}
	if _, err := r.Record(nil, types.RecordAuditRequest{TenantID: tenant, DocID: "d2", Action: types.AuditActionSave}); err != nil { //nolint:staticcheck
		t.Fatal(err)
	}

	deleted, err := r.DeleteByDoc(nil, tenant, "d1")
	if err != nil {
		t.Fatalf("DeleteByDoc: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	remaining, _ := r.Count(nil, tenant, types.ListCollabDocAuditFilter{DocID: "d2"})
	if remaining != 1 {
		t.Fatalf("d2 should still have 1 row, got %d", remaining)
	}
}
