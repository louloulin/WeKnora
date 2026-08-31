package utils

import (
	"reflect"
	"testing"
)

func TestParseMentions_Empty(t *testing.T) {
	if got := ParseMentions(""); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestParseMentions_SingleUserMention(t *testing.T) {
	got := ParseMentions("hello @user:alice how are you?")
	want := []Mention{{
		Kind:     MentionKindUser,
		ID:       "alice",
		Display:  "@user:alice",
		Position: 6,
		Length:   11,
		Raw:      "@user:alice",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseMentions_MultipleKinds(t *testing.T) {
	got := ParseMentions("ping @user:bob and @agent:helper for @task:triage-42 please")
	if len(got) != 3 {
		t.Fatalf("expected 3 mentions, got %d: %+v", len(got), got)
	}
	if got[0].Kind != MentionKindUser || got[0].ID != "bob" {
		t.Errorf("first mention wrong: %+v", got[0])
	}
	if got[1].Kind != MentionKindAgent || got[1].ID != "helper" {
		t.Errorf("second mention wrong: %+v", got[1])
	}
	if got[2].Kind != MentionKindTask || got[2].ID != "triage-42" {
		t.Errorf("third mention wrong: %+v", got[2])
	}
}

func TestParseMentions_HereIsSpecial(t *testing.T) {
	got := ParseMentions("@here stand up everyone")
	if len(got) != 1 || got[0].Kind != MentionKindHere || got[0].ID != "" {
		t.Errorf("here mention wrong: %+v", got)
	}
}

func TestParseMentions_CaseInsensitiveID(t *testing.T) {
	got := ParseMentions("hi @user:AliceBob")
	if len(got) != 1 || got[0].ID != "alicebob" {
		t.Errorf("id should be lower-cased, got %+v", got)
	}
}

func TestParseMentions_DedupByDefault(t *testing.T) {
	got := ParseMentions("@user:alice and again @user:alice but only once")
	if len(got) != 1 {
		t.Errorf("dedup should produce 1 mention, got %d: %+v", len(got), got)
	}
	if got[0].Position != 0 {
		t.Errorf("first-seen position should be 0, got %d", got[0].Position)
	}
}

func TestParseMentions_DedupDisabledKeepsAll(t *testing.T) {
	cfg := DefaultMentionParseConfig()
	cfg.Dedup = false
	got := ParseMentions("@user:alice and @user:alice", cfg)
	if len(got) != 2 {
		t.Errorf("dedup=false should keep both, got %d", len(got))
	}
}

func TestParseMentions_SkipsEmailAddresses(t *testing.T) {
	got := ParseMentions("contact alice@example.com for help")
	if len(got) != 0 {
		t.Errorf("email should not parse as mention, got %+v", got)
	}
}

func TestParseMentions_IgnoresUnknownKind(t *testing.T) {
	got := ParseMentions("@unknown:thing and @user:alice")
	if len(got) != 1 || got[0].ID != "alice" {
		t.Errorf("unknown kind must be ignored, got %+v", got)
	}
}

func TestParseMentions_RespectsAllowedKinds(t *testing.T) {
	cfg := DefaultMentionParseConfig()
	cfg.AllowedKinds = map[MentionKind]bool{
		MentionKindUser: true,
		// agent / task / here disabled
	}
	got := ParseMentions("@user:alice @agent:helper @task:t1 @here", cfg)
	if len(got) != 1 || got[0].Kind != MentionKindUser {
		t.Errorf("only user should pass, got %+v", got)
	}
}

func TestParseMentions_MaxDistanceTruncates(t *testing.T) {
	cfg := DefaultMentionParseConfig()
	cfg.MaxDistance = 10
	got := ParseMentions("@user:thisisaverylongidentifier", cfg)
	if len(got) != 0 {
		t.Errorf("over-distance mention should be ignored, got %+v", got)
	}
}

func TestParseMentions_EmptyIDRejected(t *testing.T) {
	got := ParseMentions("@user: then nothing")
	if len(got) != 0 {
		t.Errorf("empty id must be rejected, got %+v", got)
	}
}

func TestParseMentions_PreservesPositionOrder(t *testing.T) {
	got := ParseMentions("@user:later first @user:early")
	if len(got) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(got))
	}
	if got[0].ID != "later" || got[0].Position != 0 {
		t.Errorf("first should be 'later' at pos 0, got %+v", got[0])
	}
	if got[1].ID != "early" || got[1].Position != 18 {
		t.Errorf("second should be 'early' at pos 18, got %+v", got[1])
	}
}

func TestParseMentions_IDCharsetVariations(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"@user:user_123", "user_123"},
		{"@user:kebab-case", "kebab-case"},
		{"@user:ns:id", "ns:id"},
		{"@user:bad space", "bad"}, // stops at space
	}
	for _, c := range cases {
		got := ParseMentions(c.in)
		if len(got) != 1 {
			t.Errorf("%q: expected 1 mention, got %d", c.in, len(got))
			continue
		}
		if got[0].ID != c.want {
			t.Errorf("%q: id = %q, want %q", c.in, got[0].ID, c.want)
		}
	}
}

func TestExtractUserMentionIDs(t *testing.T) {
	mentions := ParseMentions("@user:alice @agent:helper @user:bob @user:alice @task:t1")
	got := ExtractUserMentionIDs(mentions)
	want := []string{"alice", "bob"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractUserMentionIDs_Empty(t *testing.T) {
	if got := ExtractUserMentionIDs(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
	if got := ExtractUserMentionIDs([]Mention{}); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestHasMention(t *testing.T) {
	mentions := ParseMentions("@user:alice @agent:helper")
	if !HasMention(mentions, MentionKindUser) {
		t.Error("should have user mention")
	}
	if !HasMention(mentions, MentionKindAgent) {
		t.Error("should have agent mention")
	}
	if HasMention(mentions, MentionKindTask) {
		t.Error("should not have task mention")
	}
}

func TestIsMentionIDChar(t *testing.T) {
	yes := []byte{'a', 'Z', '0', '9', '_', '-', ':'}
	// '.' intentionally excluded — see comment on IsMentionIDChar / mentionPattern
	for _, b := range yes {
		if !IsMentionIDChar(b) {
			t.Errorf("expected %q (%#x) to be valid id char", b, b)
		}
	}
	no := []byte{' ', '\t', '\n', '@', ',', ';', '/', '\\', '(', ')', '[', ']'}
	for _, b := range no {
		if IsMentionIDChar(b) {
			t.Errorf("expected %q (%#x) to be invalid id char", b, b)
		}
	}
}

func TestParseMentions_AdjacentMentionsNoSpace(t *testing.T) {
	// Two mentions back-to-back should each be found.
	got := ParseMentions("@user:a@user:b")
	if len(got) != 2 {
		t.Errorf("adjacent mentions should both parse, got %d: %+v", len(got), got)
	}
}

func TestParseMentions_MultibyteAndUnicodeSafe(t *testing.T) {
	// Non-ASCII surrounding text must not break parsing of an ASCII
	// mention. Unicode IDs are not supported today (the id charset
	// is [A-Za-z0-9_-:]); user IDs in the system are ASCII so this
	// is acceptable. The parser still handles UTF-8 boundaries
	// correctly via byte-indexed scanning.
	got := ParseMentions("请 @user:alice 看一下这个文档")
	if len(got) != 1 || got[0].ID != "alice" {
		t.Errorf("ascii id in unicode text should parse, got %+v", got)
	}
}

func TestParseMentions_UnicodeIDRejected(t *testing.T) {
	// Unicode-only ids are out of scope today. The parser silently
	// skips them rather than failing.
	got := ParseMentions("@user:张三")
	if len(got) != 0 {
		t.Errorf("unicode id should be rejected today, got %+v", got)
	}
}

func TestParseMentions_WhitespaceBoundaries(t *testing.T) {
	// Line break + tab are valid boundaries; the mention should parse.
	got := ParseMentions("line1\n@user:alice\tline2")
	if len(got) != 1 || got[0].ID != "alice" {
		t.Errorf("whitespace boundaries should parse, got %+v", got)
	}
}

func TestParseMentions_PunctuationBoundary(t *testing.T) {
	// Trailing period is NOT part of the id (the id charset excludes
	// '.' to avoid greedy consumption of natural-text punctuation).
	got := ParseMentions("see, @user:alice. thanks!")
	if len(got) != 1 || got[0].ID != "alice" {
		t.Errorf("punctuation boundary should parse cleanly, got %+v", got)
	}
}

func TestParseMentions_MentionInsideCodeBlock(t *testing.T) {
	// We do NOT special-case code blocks today — the parser treats
	// '@' as a mention opener regardless of context. The frontend
	// is expected to render code blocks verbatim and accept that
	// mentions inside them will also fire. This test locks the
	// current behaviour so a future "smart" parser doesn't silently
	// regress.
	got := ParseMentions("```\n@user:alice\n```")
	if len(got) != 1 || got[0].ID != "alice" {
		t.Errorf("code-block mention should still parse today, got %+v", got)
	}
}
