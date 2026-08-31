package scimsp

import (
	"strings"
	"testing"
)

func TestParseFilterBasic(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		hasErr bool
	}{
		{"empty", "", false},
		{"userName eq", `userName eq "alice"`, false},
		{"email co", `emails.value co "@corp.example.com"`, false},
		{"displayName pr", `displayName pr`, false},
		{"and chain", `userName eq "alice" and active eq "true"`, false},
		{"or chain", `userName eq "alice" or userName eq "bob"`, false},
		{"unsupported gt", `meta.lastModified gt "2024-01-01T00:00:00Z"`, true},
		{"unterminated", `userName eq "alice`, true},
		{"empty value", `userName eq ""`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFilter(tc.input)
			gotErr := err != nil
			if gotErr != tc.hasErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.hasErr)
			}
			if !tc.hasErr && tc.input != "" && f == nil {
				t.Fatalf("expected non-nil filter")
			}
		})
	}
}

func TestFilterMatch(t *testing.T) {
	doc := map[string]string{
		"username":     "alice",
		"displayname":  "Alice Smith",
		"emails.value": "alice@corp.example.com",
		"active":       "true",
	}
	get := func(attr string) (string, bool) {
		v, ok := doc[attr]
		return v, ok
	}

	cases := []struct {
		name   string
		filter string
		want   bool
	}{
		{"eq match", `userName eq "alice"`, true},
		{"eq no match", `userName eq "bob"`, false},
		{"ne match", `userName ne "bob"`, true},
		{"co match", `emails.value co "@corp"`, true},
		{"sw match", `displayName sw "Alice"`, true},
		{"sw no match", `displayName sw "Bob"`, false},
		{"pr match", `displayName pr`, true},
		{"pr no match", `nickname pr`, false},
		{"and both", `userName eq "alice" and active eq "true"`, true},
		{"and one false", `userName eq "alice" and active eq "false"`, false},
		{"or both false", `userName eq "bob" or userName eq "carol"`, false},
		{"or one true", `userName eq "bob" or userName eq "alice"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := ParseFilter(tc.filter)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := f.Match(get)
			if got != tc.want {
				t.Fatalf("Match=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestTokenizeQuotes(t *testing.T) {
	tokens, err := tokenize(`userName eq "alice bob"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("want 3 tokens, got %d (%v)", len(tokens), tokens)
	}
	if tokens[2] != "alice bob" {
		t.Fatalf("quoted value must preserve internal space, got %q", tokens[2])
	}
}

func TestTokenizeEscape(t *testing.T) {
	tokens, err := tokenize(`userName eq "al\"ice"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if !strings.Contains(tokens[2], "al") {
		t.Fatalf("escape not honoured: %v", tokens)
	}
}
