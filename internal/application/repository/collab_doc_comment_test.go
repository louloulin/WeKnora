// Package repository - v0.7.29 collab_doc_comment repo integration test.
package repository

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCommentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&types.CollabDocComment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCollabDocCommentRepoCreateAndGet(t *testing.T) {
	db := newCommentTestDB(t)
	r := NewCollabDocCommentRepository(db)
	tenant := uint64(1)
	docID := "doc-c1"
	threadID := "thread-1"

	// Create a thread anchor (no parent)
	anchor := &types.CollabDocComment{
		TenantID:     tenant,
		DocID:        docID,
		ThreadID:     threadID,
		AuthorUserID: 100,
		AuthorName:   "Alice",
		AuthorColor:   "#58a6ff",
		AnchorType:   types.CommentAnchorDoc,
		AnchorRef:    `{"from":3,"to":5}`,
		Body:         "Could you reword this?",
	}
	if err := r.Create(nil, anchor); err != nil {
		t.Fatalf("create anchor: %v", err)
	}
	if anchor.ID == 0 {
		t.Fatalf("expected ID to be set")
	}

	// Reply (parent_id set to anchor.ID)
	reply := &types.CollabDocComment{
		TenantID:     tenant,
		DocID:        docID,
		ThreadID:     threadID,
		ParentID:     &anchor.ID,
		AuthorUserID: 200,
		AuthorName:   "Bob",
		AnchorType:   types.CommentAnchorDoc,
		Body:         "Sure, how about X?",
	}
	if err := r.Create(nil, reply); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	rows, err := r.ListByDoc(nil, tenant, docID, types.ListCollabDocCommentsFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Body != anchor.Body {
		t.Errorf("expected first row to be the anchor")
	}
}

func TestCollabDocCommentRepoUpdate(t *testing.T) {
	db := newCommentTestDB(t)
	r := NewCollabDocCommentRepository(db)
	c := &types.CollabDocComment{
		TenantID:     1,
		DocID:        "doc",
		ThreadID:     "t1",
		AuthorUserID: 100,
		AnchorType:   types.CommentAnchorSlide,
		AnchorRef:    `{"slide":2,"shapeId":"x"}`,
		Body:         "first body",
	}
	if err := r.Create(nil, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	newBody := "edited body"
	resolved := true
	updated, err := r.Update(nil, 1, c.ID, types.UpdateCollabDocCommentRequest{
		Body:     &newBody,
		Resolved: &resolved,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Body != newBody {
		t.Errorf("body not updated: %q", updated.Body)
	}
	if !updated.Resolved {
		t.Errorf("resolved not updated")
	}
}

func TestCollabDocCommentRepoValidationErrors(t *testing.T) {
	db := newCommentTestDB(t)
	r := NewCollabDocCommentRepository(db)

	// Empty body
	err := r.Create(nil, &types.CollabDocComment{
		TenantID:   1,
		DocID:      "doc",
		ThreadID:   "t1",
		AnchorType: types.CommentAnchorDoc,
		Body:       "",
	})
	if err == nil {
		t.Errorf("expected validation error on empty body")
	}

	// Invalid anchor type
	err = r.Create(nil, &types.CollabDocComment{
		TenantID:   1,
		DocID:      "doc",
		ThreadID:   "t1",
		AnchorType: "bogus",
		Body:       "x",
	})
	if err == nil {
		t.Errorf("expected validation error on invalid anchor_type")
	}
}
