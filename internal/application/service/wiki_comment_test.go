package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// These tests cover the service-layer validation rules that don't need a
// real gorm.DB. Repository-backed paths (Create / Update / Delete / SetResolved)
// are exercised in the router-level integration tests once the wire-up
// lands; here we lock down the cheap invariants the rest of the code
// depends on.

func TestWikiCommentCreate_RejectsEmptyBody(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	_, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "   "})
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
}

func TestWikiCommentCreate_RejectsTooLongBody(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	long := strings.Repeat("a", 10001)
	_, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: long})
	if err == nil {
		t.Fatal("expected error for too-long body, got nil")
	}
}

func TestWikiCommentCreate_RejectsPageNotFound(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: false}}
	_, err := svc.Create(context.Background(), "kb-1", "missing", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "hi"})
	if err == nil {
		t.Fatal("expected error for missing page, got nil")
	}
}

func TestWikiCommentCreate_RejectsEmptyParentID(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	empty := ""
	_, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "reply", ParentCommentID: &empty})
	if err == nil {
		t.Fatal("expected error for empty parent_comment_id, got nil")
	}
}

func TestWikiCommentCreate_NormalizesNilMentions(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	out, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "hello"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Mentions == nil {
		t.Fatal("Mentions should be empty array, not nil")
	}
	if len(out.Mentions) != 0 {
		t.Errorf("Mentions should be empty, got %v", out.Mentions)
	}
}

func TestWikiCommentCreate_PropagatesMentions(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	out, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{
			Body:     "cc @alice",
			Mentions: types.StringArray{"alice", "bob"},
		})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.Mentions) != 2 {
		t.Errorf("Mentions = %v, want [alice bob]", out.Mentions)
	}
}

func TestWikiCommentCreate_GeneratesIDAndTimestamps(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	out, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID == "" {
		t.Error("ID should be populated")
	}
	if out.AuthorID != "user-1" {
		t.Errorf("AuthorID = %q, want user-1", out.AuthorID)
	}
	if out.TenantID != 7 {
		t.Errorf("TenantID = %d, want 7", out.TenantID)
	}
	if out.WikiPageID != "page-1" {
		t.Errorf("WikiPageID = %q, want page-1", out.WikiPageID)
	}
	if out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		t.Error("timestamps should be populated")
	}
	if out.UpdatedAt.Before(out.CreatedAt) {
		t.Errorf("UpdatedAt %v < CreatedAt %v", out.UpdatedAt, out.CreatedAt)
	}
}

func TestWikiCommentCreate_PreservesParentCommentID(t *testing.T) {
	svc := &WikiPageCommentService{pageLookup: &stubPageLookup{exists: true}}
	parent := "parent-1"
	out, err := svc.Create(context.Background(), "kb-1", "page-1", "user-1", 7,
		&types.CreateWikiCommentRequest{Body: "reply", ParentCommentID: &parent})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.ParentCommentID == nil || *out.ParentCommentID != parent {
		t.Errorf("ParentCommentID = %v, want %q", out.ParentCommentID, parent)
	}
}

func TestWikiCommentErrors_AreDistinct(t *testing.T) {
	if ErrCommentNotFound == nil {
		t.Fatal("ErrCommentNotFound should be exported")
	}
	if ErrCommentForbidden == nil {
		t.Fatal("ErrCommentForbidden should be exported")
	}
	if ErrCommentNotFound.Error() == ErrCommentForbidden.Error() {
		t.Errorf("errors must be distinguishable; both = %q", ErrCommentNotFound.Error())
	}
}

func TestNormalizeMentions(t *testing.T) {
	if got := normalizeMentions(nil); got == nil {
		t.Error("nil in should produce empty array, got nil")
	}
	if got := normalizeMentions(types.StringArray{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("non-nil should pass through unchanged, got %v", got)
	}
}

// stubPageLookup lets us inject page existence behavior without spinning
// up the full WikiPageService.
type stubPageLookup struct {
	exists bool
	err    error
}

func (s *stubPageLookup) PageExists(_ context.Context, _, _ string) (bool, error) {
	return s.exists, s.err
}
