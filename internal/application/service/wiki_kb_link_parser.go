package service

import (
	"regexp"
	"strings"
)

// kbReferenceRegex matches the [[kb:<id>]] inline mention syntax
// inside a wiki page body. The label (between pipes) is captured
// separately so the backfill can record it for audit.
//
// Examples it matches:
//
//	[[kb:abc-123]]
//	[[kb:abc-123|Foo release notes]]
//	[[KB:ABC-123]]          (case-insensitive prefix)
//
// Examples it does NOT match:
//
//	[[page-slug]]           (no kb: prefix)
//	[[ kb:abc-123]]         (leading whitespace)
//	[kb:abc-123]             (missing outer brackets)
//	![kb:abc-123]            (this is an image alt text — kept literal)
//
// We deliberately keep this regex tight rather than parsing full
// markdown: the markdown parser already extracts plain-text positions,
// and a permissive regex here would catch image alts / link text.
var kbReferenceRegex = regexp.MustCompile(`\[\[(?i)kb:([A-Za-z0-9_\-]+)(?:\|([^\]]+))?\]\]`)

// KBReferenceSpan is one inline mention found in the page body.
// Label is empty if the author wrote the bare [[kb:id]] form.
type KBReferenceSpan struct {
	KnowledgeID string
	Label       string
}

// ParseKBReferences walks the page body and returns every inline
// mention in source order, deduplicating by KnowledgeID so a backfill
// can call AddReference once per unique KB id.
//
// The function is intentionally pure: no IO, no DB, no allocations
// beyond the result slice. Callers run it on every wiki page save and
// every wiki page render, so it must stay cheap.
func ParseKBReferences(content string) []KBReferenceSpan {
	if content == "" {
		return nil
	}
	matches := kbReferenceRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]KBReferenceSpan, 0, len(matches))
	for _, m := range matches {
		id := strings.TrimSpace(m[1])
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		label := ""
		if len(m) >= 3 {
			label = strings.TrimSpace(m[2])
		}
		out = append(out, KBReferenceSpan{KnowledgeID: id, Label: label})
	}
	return out
}
