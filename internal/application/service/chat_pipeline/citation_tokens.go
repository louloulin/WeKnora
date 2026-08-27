package chatpipeline

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
)

// Build #30 — citation token rendering.
//
// After the model emits `<ref id="cN"/>` private handles and the citation
// expander promotes them to public `<kb doc="..." chunk_id="..." kb_id="..."
// />` tags, this pass rewrites every `<kb>` tag into the user-visible
// `[[cite:N]]` token, where N is the 1-indexed position in the answer's
// citation list. CitationIndex[N-1] carries the chunk metadata that the
// frontend uses to render the clickable chip and to call back into the
// audit linkage (Build #30 B4 citation_log handler).
//
// Design choices:
//   - The numeric position is assigned left-to-right in the order each
//     chunk_id first appears. The same chunk referenced twice keeps the
//     same number; this matches user intuition (numbering tracks the
//     evidence, not the call sites).
//   - Citations are gated by CitationsEnabled() — when disabled, the
//     function is a no-op and CitationIndex stays nil.
//   - The pass does NOT validate that cited chunks are in MergeResult:
//     a hallucinated `<kb>` tag with an unknown chunk_id still produces a
//     citation entry so the chip renders, but the audit handler in B4
//     can flag the mismatch separately.
//   - attachCitations is pure (single text in/out). The streaming
//     goroutine uses citationBuilder so the de-dup table persists across
//     chunk Feed/Flush calls and the same chunk_id seen in chunk N still
//     resolves to position 1 in chunk N+1.

var (
	// kbTagRE matches the public citation tags emitted by modelcontext
	// ExpandText: `<kb doc="..." chunk_id="..." [kb_id="..."] />` with
	// attributes in any order. The captured group is the attribute string,
	// and the per-attribute regexes below extract each value independently.
	kbTagRE = regexp.MustCompile(`(?is)<kb\b([^>]*?)/>`)

	// chunkIDRE extracts the chunk_id value. Required — a `<kb>` tag
	// without chunk_id is dropped (no citation entry).
	chunkIDRE = regexp.MustCompile(`(?is)\bchunk_id\s*=\s*"([^"]+)"`)
	// titleRE accepts either `doc` or `title` as the human-readable label
	// the chip tooltip uses.
	titleRE = regexp.MustCompile(`(?is)\b(?:doc|title)\s*=\s*"([^"]*)"`)
	// kbIDRE is optional; absent kb_id is fine (the index row leaves it empty).
	kbIDRE = regexp.MustCompile(`(?is)\bkb_id\s*=\s*"([^"]*)"`)
)

// attachCitations rewrites every <kb chunk_id="..." /> tag in `text`
// into a [[cite:N]] token and returns the rewritten text alongside the
// citation index. When citations are disabled on the request, the
// original text is returned unchanged with a nil index.
//
// The function is pure — it does not consult any external state. The
// streaming path uses citationBuilder instead so the de-dup table can
// span multiple chunks.
func attachCitations(chatManage *types.ChatManage, text string) (string, []types.CitationEntry) {
	if chatManage == nil || !chatManage.CitationsEnabled() {
		return text, nil
	}
	if text == "" {
		return text, nil
	}

	positions := make(map[string]int)
	var index []types.CitationEntry
	out := substituteKBTags(text, &positions, &index)
	return out, index
}

// substituteKBTags walks `text` and replaces every `<kb>` tag with a
// `[[cite:N]]` token, filling the shared positions/index slice in lock-
// step. Pure helper so it can be reused by both attachCitations (single
// text in/out) and citationBuilder (stateful across chunks) without
// duplicating the regex logic.
func substituteKBTags(text string, positions *map[string]int, index *[]types.CitationEntry) string {
	return kbTagRE.ReplaceAllStringFunc(text, func(tag string) string {
		tagMatch := kbTagRE.FindStringSubmatch(tag)
		if len(tagMatch) < 2 {
			return ""
		}
		attrs := tagMatch[1]

		cm := chunkIDRE.FindStringSubmatch(attrs)
		if len(cm) < 2 {
			return ""
		}
		chunkID := strings.TrimSpace(cm[1])
		if chunkID == "" {
			return ""
		}

		title := ""
		if tm := titleRE.FindStringSubmatch(attrs); len(tm) >= 2 {
			title = strings.TrimSpace(tm[1])
		}
		kbID := ""
		if km := kbIDRE.FindStringSubmatch(attrs); len(km) >= 2 {
			kbID = strings.TrimSpace(km[1])
		}

		pos, ok := (*positions)[chunkID]
		if !ok {
			pos = len(*index) + 1
			(*positions)[chunkID] = pos
			*index = append(*index, types.CitationEntry{
				ChunkID:         chunkID,
				KnowledgeBaseID: kbID,
				Title:           title,
			})
		}
		return "[[cite:" + itoa(pos) + "]]"
	})
}

// citationBuilder is the streaming-aware variant of attachCitations.
// Each Rewrite call substitutes `<kb>` tags → `[[cite:N]]` tokens while
// sharing a single de-dup table across calls so a chunk referenced in
// chunk 1 keeps the same position when seen again in chunk 5.
//
// The zero value is ready to use. Methods are safe for concurrent use
// because the goroutine is single-threaded per stream, but we lock
// anyway so callers can fan out safely if they ever want to.
type citationBuilder struct {
	mu        sync.Mutex
	positions map[string]int
	index     []types.CitationEntry
	enabled   bool
}

// newCitationBuilder returns a builder gated by chatManage's citation
// setting. When citations are disabled the builder acts as identity
// (Rewrite returns text unchanged, Index returns nil).
func newCitationBuilder(chatManage *types.ChatManage) *citationBuilder {
	return &citationBuilder{
		positions: make(map[string]int),
		enabled:   chatManage != nil && chatManage.CitationsEnabled(),
	}
}

// Rewrite substitutes `<kb>` tags in `text` with `[[cite:N]]` tokens and
// updates the running citation index. The text argument should already
// be the post-decode output from the citation stream expander — i.e.,
// every `<kb>` tag is complete (not split mid-stream).
//
// Empty input is a no-op.
func (b *citationBuilder) Rewrite(text string) string {
	if b == nil || !b.enabled || text == "" {
		return text
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return substituteKBTags(text, &b.positions, &b.index)
}

// Index returns a copy of the citation entries accumulated so far.
// Returns nil when citations are disabled or no chunks were cited.
func (b *citationBuilder) Index() []types.CitationEntry {
	if b == nil || !b.enabled || len(b.index) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]types.CitationEntry, len(b.index))
	copy(out, b.index)
	return out
}

// itoa is a tiny integer-to-string helper to avoid pulling strconv just
// for citation numbering. Position values are bounded by the number of
// cited chunks (typically <50 in production traffic).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}