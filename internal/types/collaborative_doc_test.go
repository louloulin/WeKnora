package types

import "testing"

func TestParseCollaborativeDocKind(t *testing.T) {
	for _, want := range []CollaborativeDocKind{CollaborativeDocKindDoc, CollaborativeDocKindSheet, CollaborativeDocKindSlide} {
		got, err := ParseCollaborativeDocKind(string(want))
		if err != nil {
			t.Fatalf("parse %q: %v", want, err)
		}
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	if _, err := ParseCollaborativeDocKind("spreadsheet"); err == nil {
		t.Fatalf("expected error for unknown kind")
	}
}

func TestCollabDocSnapshotUpsertValidate(t *testing.T) {
	cases := []struct{
		name string
		in   CollabDocSnapshotUpsert
		ok   bool
	}{
		{"valid", CollabDocSnapshotUpsert{TenantID: 1, DocID: "abc", DocKind: CollaborativeDocKindDoc, YDocState: []byte{1}, SizeBytes: 1}, true},
		{"missing tenant", CollabDocSnapshotUpsert{DocID: "abc", DocKind: CollaborativeDocKindDoc, YDocState: []byte{1}, SizeBytes: 1}, false},
		{"missing doc id", CollabDocSnapshotUpsert{TenantID: 1, DocKind: CollaborativeDocKindDoc, YDocState: []byte{1}, SizeBytes: 1}, false},
		{"bad kind", CollabDocSnapshotUpsert{TenantID: 1, DocID: "abc", DocKind: "spreadsheet", YDocState: []byte{1}, SizeBytes: 1}, false},
		{"empty state", CollabDocSnapshotUpsert{TenantID: 1, DocID: "abc", DocKind: CollaborativeDocKindDoc, SizeBytes: 1}, false},
		{"zero size", CollabDocSnapshotUpsert{TenantID: 1, DocID: "abc", DocKind: CollaborativeDocKindDoc, YDocState: []byte{1}}, false},
	}
	for _, tc := range cases {
		err := tc.in.Validate()
		got := err == nil
		if got != tc.ok {
			t.Errorf("%s: got ok=%v want ok=%v (err=%v)", tc.name, got, tc.ok, err)
		}
	}
}

func TestValidCollaborativeDocKindsClosed(t *testing.T) {
	// Closed set guards against typos landing in the DB.
	want := map[CollaborativeDocKind]bool{
		CollaborativeDocKindDoc: true, CollaborativeDocKindSheet: true, CollaborativeDocKindSlide: true,
	}
	if len(ValidCollaborativeDocKinds) != len(want) {
		t.Fatalf("expected %d kinds, got %d", len(want), len(ValidCollaborativeDocKinds))
	}
	for k := range want {
		if !ValidCollaborativeDocKinds[k] {
			t.Errorf("%q missing from closed set", k)
		}
	}
}
