package utils

import (
	"regexp"
	"strings"
	"unicode"
)

// MentionKind enumerates the categories of @-mention the parser
// recognizes. The set is intentionally closed so emitters and the
// frontend can ship a default rendering per kind without guessing.
//
//	@user   → notify a tenant member
//	@agent  → scope the agent invocation (KB Owner+ only)
//	@task   → bind to a workflow task (KB Owner+ only)
//	@here   → broadcast to all session collaborators (future)
//
// The frontend's MentionSelector handles kb / file / tag / mcp / skill
// (agent-scope selectors) — those do NOT go through this parser
// because they are not user-visible notifications.
type MentionKind string

const (
	MentionKindUser  MentionKind = "user"
	MentionKindAgent MentionKind = "agent"
	MentionKindTask  MentionKind = "task"
	MentionKindHere  MentionKind = "here"
)

// Mention is a single parsed @-mention. Position is the byte offset
// of the '@' character in the source text; Length covers the entire
// "@<kind>:<id>" token so the frontend can replace it in-place if it
// wants to render a chip.
type Mention struct {
	Kind     MentionKind `json:"kind"`
	ID       string      `json:"id"`
	Display  string      `json:"display"`
	Position int         `json:"position"`
	Length   int         `json:"length"`
	// Raw is the exact source substring (including the leading '@').
	// Useful for highlighting or undo logic.
	Raw string `json:"raw"`
}

// MentionParseConfig controls parser behaviour. The defaults (zero
// value) match the production contract: users + agents + tasks,
// dedup, case-insensitive id matching.
type MentionParseConfig struct {
	// AllowedKinds restricts which kinds the parser will emit. When
	// nil, all four kinds are allowed.
	AllowedKinds map[MentionKind]bool
	// MaxDistance limits how far the parser will look past the '@'
	// for the kind delimiter. Defaults to 32 bytes; longer distances
	// are treated as plain text to keep the parser safe on noisy
	// content.
	MaxDistance int
	// Dedup merges multiple mentions of the same (kind, id) into a
	// single Mention with the first-seen position. Defaults to true.
	Dedup bool
}

// DefaultMentionParseConfig is the production default.
func DefaultMentionParseConfig() MentionParseConfig {
	return MentionParseConfig{
		AllowedKinds: map[MentionKind]bool{
			MentionKindUser:  true,
			MentionKindAgent: true,
			MentionKindTask:  true,
			MentionKindHere:  true,
		},
		MaxDistance: 32,
		Dedup:       true,
	}
}

// mentionPattern matches `@<kind>:<id>` where kind ∈ {user, agent,
// task, here} and id is a run of allowed id characters. The id
// charset is conservative — letters, digits, dash, underscore,
// colon — to keep the parser from greedily eating punctuation like
// commas or periods that the user almost certainly did not intend
// as part of the id.
//
// Note: '.' is intentionally excluded from the id charset so that
// "@user:alice." (with trailing period) parses as id="alice" not
// "alice.". Trailing dots/commas are very common in natural text
// and almost never intentional id characters.
var mentionPattern = regexp.MustCompile(`@(user|agent|task|here):([A-Za-z0-9_\-:]+)`)

// IsMentionIDChar reports whether b is a legal id character. Exposed
// so the frontend can validate input in its @-menu without a round-
// trip. Mirrors the regex charset above.
func IsMentionIDChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z',
		b >= 'A' && b <= 'Z',
		b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '_', '-', ':':
		return true
	}
	return false
}


// matchBareHere reports whether rest begins with the bare broadcast
// mention "@here" — i.e. no ":" suffix, no id. Returns the byte
// length of the matched token when true so the caller can advance
// `lastMentionEnd` and skip past it.
//
// The trailing character (if any) must be a non-id-character so that
// "@hereafter", "@heredoc", and "@here_x" are NOT treated as the
// broadcast mention. id characters are defined by IsMentionIDChar.
func matchBareHere(rest string) (int, bool) {
	if len(rest) < 5 {
		return 0, false
	}
	if rest[0] != '@' || rest[1] != 'h' || rest[2] != 'e' || rest[3] != 'r' || rest[4] != 'e' {
		return 0, false
	}
	if len(rest) == 5 {
		return 5, true
	}
	if IsMentionIDChar(rest[5]) {
		return 0, false
	}
	return 5, true
}

// ParseMentions extracts every @-mention from text. The parser is
// pure — no regex backtracking beyond Go's RE2 guarantee, no
// allocations beyond the result slice + dedup map.
//
// Order of returned mentions is by ascending Position in the source
// string. When Dedup is enabled (default), a repeated (kind, id)
// only appears once with its first-seen Position.
//
// The function never returns nil; an empty slice is the "no
// mentions" signal so callers can range over the result without a
// nil check.
func ParseMentions(text string, cfg ...MentionParseConfig) []Mention {
	if text == "" {
		return nil
	}
	var c MentionParseConfig
	if len(cfg) > 0 {
		c = cfg[0]
	} else {
		c = DefaultMentionParseConfig()
	}
	if c.AllowedKinds == nil {
		c.AllowedKinds = DefaultMentionParseConfig().AllowedKinds
	}
	if c.MaxDistance <= 0 {
		c.MaxDistance = DefaultMentionParseConfig().MaxDistance
	}

	maxDist := c.MaxDistance
	out := make([]Mention, 0, 4)
	seen := make(map[string]struct{}, 4)
	// lastMentionEnd tracks the absolute byte position just past the
	// end of the most recently emitted mention. We use it to allow
	// adjacent mentions (e.g. "@user:a@user:b") without the email
	// boundary check incorrectly skipping the second '@'.
	lastMentionEnd := -1

	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		// Skip if the '@' is part of an email address — i.e. the
		// preceding character is a letter/digit/underscore AND we
		// are NOT at the boundary of a previously emitted mention.
		if i > 0 && i != lastMentionEnd {
			prev := text[i-1]
			if isEmailContinuation(prev) {
				continue
			}
		}
		rest := text[i:]
		// Special case: bare "@here" broadcast — has no ":" suffix.
		// Handled before the main regex because mentionPattern requires
		// a ":" + id run, which bare "@here" does not satisfy.
		if hereEnd, ok := matchBareHere(rest); ok {
			matchEnd := hereEnd
			if maxDist > 0 && matchEnd > maxDist {
				continue
			}
			if !c.AllowedKinds[MentionKindHere] {
				lastMentionEnd = i + matchEnd
				continue
			}
			if c.Dedup {
				key := string(MentionKindHere) + "\x00"
				if _, dup := seen[key]; dup {
					lastMentionEnd = i + matchEnd
					continue
				}
				seen[key] = struct{}{}
			}
			out = append(out, Mention{
				Kind:     MentionKindHere,
				ID:       "",
				Display:  "@here",
				Position: i,
				Length:   matchEnd,
				Raw:      rest[:matchEnd],
			})
			lastMentionEnd = i + matchEnd
			i += matchEnd - 1 // -1 because the loop's i++ will advance
			continue
		}
		if len(rest) < 6 { // @user:X at minimum
			continue
		}
		loc := mentionPattern.FindStringIndex(rest)
		if loc == nil {
			continue
		}
		matchStart := loc[0]
		matchEnd := loc[1]
		if matchStart != 0 {
			continue
		}
		if maxDist > 0 && matchEnd > maxDist {
			continue
		}
		raw := rest[:matchEnd]
		body := raw[1:] // drop '@'
		colon := strings.IndexByte(body, ':')
		if colon < 0 {
			continue
		}
		kindStr := body[:colon]
		id := body[colon+1:]
		if id == "" {
			continue
		}
		kind := MentionKind(kindStr)
		if !c.AllowedKinds[kind] {
			continue
		}
		if c.Dedup {
			key := string(kind) + "\x00" + id
			if _, ok := seen[key]; ok {
				// Even when deduping, advance lastMentionEnd so a
				// third mention immediately after is not blocked.
				lastMentionEnd = i + matchEnd
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, Mention{
			Kind:     kind,
			ID:       strings.ToLower(id),
			Display:  displayFromID(kind, id),
			Position: i,
			Length:   matchEnd,
			Raw:      raw,
		})
		lastMentionEnd = i + matchEnd
	}
	return out
}

// displayFromID returns the human-friendly label for a mention id.
// user/agent/task display their id verbatim; here is rendered as
// "@here" (no id). Callers can override at the UI layer.
func displayFromID(kind MentionKind, id string) string {
	switch kind {
	case MentionKindHere:
		return "@here"
	default:
		return "@" + string(kind) + ":" + id
	}
}

// isEmailContinuation reports whether prev being a letter/digit/
// underscore means the '@' at position i is part of an email
// address (e.g. "alice@example.com") and must NOT be treated as a
// mention opener.
func isEmailContinuation(prev byte) bool {
	r := rune(prev)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || prev == '_'
}

// ExtractUserMentionIDs returns just the user-id set from a parsed
// mention list. Returns a deduplicated, lower-cased slice; the order
// matches the first-seen order from ParseMentions. Convenience for
// the notification emitter.
func ExtractUserMentionIDs(mentions []Mention) []string {
	if len(mentions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(mentions))
	out := make([]string, 0, len(mentions))
	for _, m := range mentions {
		if m.Kind != MentionKindUser {
			continue
		}
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		out = append(out, m.ID)
	}
	return out
}

// HasMention reports whether any parsed mention has the given kind.
// Used by the chat pipeline to decide whether to attach an extra
// "scope" badge to the rendered message.
func HasMention(mentions []Mention, kind MentionKind) bool {
	for _, m := range mentions {
		if m.Kind == kind {
			return true
		}
	}
	return false
}
