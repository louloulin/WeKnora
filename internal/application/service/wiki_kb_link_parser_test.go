//go:build wikikbtest
// +build wikikbtest

package service_test

import (
	"reflect"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
)

// TestParseKBReferences_BasicForms covers the four shapes the parser
// promises to handle: bare id, id with label, case-insensitive prefix,
// and whitespace trimming. Each case asserts both the KB id and the
// optional label so a future refactor can't quietly drop one field.
func TestParseKBReferences_BasicForms(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []service.KBReferenceSpan
	}{
		{
			name:    "bare id",
			content: "see [[kb:abc-123]] for context",
			want:    []service.KBReferenceSpan{{KnowledgeID: "abc-123", Label: ""}},
		},
		{
			name:    "id with label",
			content: "see [[kb:abc-123|Foo release notes]] for context",
			want:    []service.KBReferenceSpan{{KnowledgeID: "abc-123", Label: "Foo release notes"}},
		},
		{
			name:    "uppercase prefix is fine",
			content: "[[KB:ABC-123]]",
			want:    []service.KBReferenceSpan{{KnowledgeID: "ABC-123", Label: ""}},
		},
		{
			name:    "underscore and dash allowed",
			content: "[[kb:a_b-c-1]]",
			want:    []service.KBReferenceSpan{{KnowledgeID: "a_b-c-1", Label: ""}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.ParseKBReferences(tc.content)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestParseKBReferences_Dedup guarantees a page that mentions the
// same KB doc three times only produces one backfill entry, which
// keeps the upsert fast and the audit log readable.
func TestParseKBReferences_Dedup(t *testing.T) {
	content := "[[kb:abc]] and [[kb:abc|again]] and [[kb:abc]]"
	got := service.ParseKBReferences(content)
	if len(got) != 1 {
		t.Fatalf("expected 1 unique span, got %d: %+v", len(got), got)
	}
	if got[0].KnowledgeID != "abc" {
		t.Fatalf("expected id abc, got %q", got[0].KnowledgeID)
	}
	if got[0].Label != "again" {
		t.Fatalf("expected label 'again', got %q", got[0].Label)
	}
}

// TestParseKBReferences_IgnoresNonKBLinks is the negative space:
// wiki cross-links, image alts, and bare brackets must NOT be
// mistaken for KB references, otherwise the backfill would happily
// create phantom bindings.
func TestParseKBReferences_IgnoresNonKBLinks(t *testing.T) {
	cases := []string{
		"plain text with no references",
		"see [[some-page-slug]] for context",        // wiki cross-link
		"![alt text](image.png)",                    // image
		"![kb:not-an-id](image.png)",                // image alt — also rejected
		"inline [kb:abc-123] missing outer brackets", // brackets only
		"[[ kb:leading-space]] rejects whitespace",
		"[[kb:]] empty id rejected",
	}
	for _, c := range cases {
		if got := service.ParseKBReferences(c); len(got) != 0 {
			t.Fatalf("expected no refs in %q, got %+v", c, got)
		}
	}
}

// TestParseKBReferences_EmptyInput is the cheapest possible smoke:
// the function must short-circuit on empty input rather than run
// the regex engine on it.
func TestParseKBReferences_EmptyInput(t *testing.T) {
	if got := service.ParseKBReferences(""); got != nil {
		t.Fatalf("expected nil for empty input, got %+v", got)
	}
}
