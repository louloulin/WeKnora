package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/authz"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestValidateTupleRequest_AcceptsValidUserGrant(t *testing.T) {
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "kb",
		ObjectID:    "kb-1",
		Relation:    "viewer",
		SubjectType: "user",
		SubjectID:   "u-1",
	}
	if err := validateTupleRequest(req); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestValidateTupleRequest_RejectsUnknownObjectType(t *testing.T) {
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "spaceship",
		ObjectID:    "x",
		Relation:    "viewer",
		SubjectType: "user",
		SubjectID:   "u-1",
	}
	if err := validateTupleRequest(req); err == nil {
		t.Fatal("expected error for unknown object_type")
	}
}

func TestValidateTupleRequest_RejectsUnknownRelation(t *testing.T) {
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "kb",
		ObjectID:    "kb-1",
		Relation:    "destroyer",
		SubjectType: "user",
		SubjectID:   "u-1",
	}
	if err := validateTupleRequest(req); err == nil {
		t.Fatal("expected error for unknown relation")
	}
}

func TestValidateTupleRequest_RejectsBadSubjectType(t *testing.T) {
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "kb",
		ObjectID:    "kb-1",
		Relation:    "viewer",
		SubjectType: "robot",
		SubjectID:   "r-1",
	}
	if err := validateTupleRequest(req); err == nil {
		t.Fatal("expected error for subject_type robot")
	}
}

func TestValidateTupleRequest_RejectsEmptySubjectID(t *testing.T) {
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "kb",
		ObjectID:    "kb-1",
		Relation:    "viewer",
		SubjectType: "user",
		SubjectID:   "",
	}
	if err := validateTupleRequest(req); err == nil {
		t.Fatal("expected error for empty subject_id")
	}
}

func TestValidateTupleRequest_RejectsPastExpires(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	req := types.AuthZTupleCreateRequest{
		ObjectType:  "kb",
		ObjectID:    "kb-1",
		Relation:    "viewer",
		SubjectType: "user",
		SubjectID:   "u-1",
		ExpiresAt:   &past,
	}
	if err := validateTupleRequest(req); err == nil {
		t.Fatal("expected error for expires_at in the past")
	}
}

func TestIsKnownObjectType_CoversPhase3Set(t *testing.T) {
	want := []string{"tenant", "kb", "wiki_page", "agent", "datasource", "notification", "chat_message"}
	for _, w := range want {
		if !isKnownObjectType(w) {
			t.Errorf("isKnownObjectType(%q) = false, want true", w)
		}
	}
	if isKnownObjectType("not_a_type") {
		t.Errorf("isKnownObjectType must reject unknown types")
	}
}

func TestIsKnownRelation_CoversRuntimeSet(t *testing.T) {
	want := []string{"owner", "editor", "viewer", "admin", "mention", "comment", "share", "delete", "read"}
	for _, w := range want {
		if !isKnownRelation(w) {
			t.Errorf("isKnownRelation(%q) = false, want true", w)
		}
	}
	if isKnownRelation("frobnicate") {
		t.Errorf("isKnownRelation must reject unknown relations")
	}
}

func TestClampLimit(t *testing.T) {
	cases := []struct {
		in, def, max, want int
	}{
		{0, 100, 500, 100},
		{-1, 100, 500, 100},
		{50, 100, 500, 50},
		{500, 100, 500, 500},
		{1000, 100, 500, 500},
	}
	for _, c := range cases {
		got := clampLimit(c.in, c.def, c.max)
		if got != c.want {
			t.Errorf("clampLimit(%d, %d, %d) = %d, want %d", c.in, c.def, c.max, got, c.want)
		}
	}
}

// TestTupleAdapter_NoLookupFailsClosed asserts that registering
// the tuple adapter with a nil lookup degrades to "no relation"
// rather than silently allowing everything.
func TestTupleAdapter_NoLookupFailsClosed(t *testing.T) {
	a := NewTupleAdapter(nil)
	d := a.Check(context.Background(), authz.CheckRequest{
		User:     authz.User{Type: authz.UserTypeUser, ID: "u-1", TenantID: 1, Role: "owner"},
		Object:   authz.Object{Type: authz.ObjectTypeKB, ID: "kb-1"},
		Relation: authz.RelationViewer,
	})
	if d.Allowed || d.Code != authz.CodeNoRelation {
		t.Fatalf("nil lookup must deny with CodeNoRelation, got %+v", d)
	}
}

// TestTupleAdapter_ObjectTypeReturnsTenant verifies the adapter
// uses the ObjectTypeTenant sentinel so it gets consulted for every
// object via the composite fallthrough.
func TestTupleAdapter_ObjectTypeReturnsTenant(t *testing.T) {
	a := NewTupleAdapter(nil)
	if a.ObjectType() != authz.ObjectTypeTenant {
		t.Fatalf("ObjectType() = %q, want %q", a.ObjectType(), authz.ObjectTypeTenant)
	}
}
