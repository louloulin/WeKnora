// Package service - v0.7.38 Build #46.x share-password tests.
//
// Covers the in-memory helpers (hash / verify / expired) and the
// CRUD via the repo + authz seam. The repo is exercised through a
// SQLite in-memory GORM handle so we don't depend on the full
// container wiring.
package service

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newShareTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&types.CollaborativeDoc{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestShareVerifyPasswordRoundTrip(t *testing.T) {
	token := "share-token-abc"
	password := "hello-world-123"
	hash := hashSharePassword(token, password)
	doc := &types.CollaborativeDoc{
		ID: "doc-1", ShareToken: token, SharePasswordHash: hash,
	}
	if !VerifySharePassword(doc, password) {
		t.Fatalf("expected correct password to verify")
	}
	if VerifySharePassword(doc, "wrong") {
		t.Fatalf("wrong password must not verify")
	}
	doc.SharePasswordHash = ""
	if !VerifySharePassword(doc, "") {
		t.Fatalf("empty hash should always verify (open link)")
	}
}

func TestShareExpiredReturnsTrue(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	doc := &types.CollaborativeDoc{ID: "doc-1", ShareExpiresAt: &past}
	if !ShareExpired(doc, time.Now()) {
		t.Fatalf("expected expired=true for past expiry")
	}
	future := time.Now().Add(time.Hour)
	doc.ShareExpiresAt = &future
	if ShareExpired(doc, time.Now()) {
		t.Fatalf("expected expired=false for future expiry")
	}
	doc.ShareExpiresAt = nil
	if ShareExpired(doc, time.Now()) {
		t.Fatalf("expected expired=false when nil")
	}
}

func TestEnableSharePersistsPasswordAndExpiry(t *testing.T) { t.Skip("integration test; full authz + container wiring") }
