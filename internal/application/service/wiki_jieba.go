package service

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// JiebaSegmentForSearch produces the space-joined token string that backs
// Build #19.x's `content_ts_zh` column. It runs `CutForSearch` (search-engine
// mode) on the combined title + content so the multi-character Chinese words
// get broken into both full and sub-word tokens. The result is what gets
// stored verbatim — PostgreSQL re-tokenizes via `to_tsvector('simple', ...)`
// at index time, but the input is already word-aligned so the lexeme set is
// stable and deterministic.
//
// Returns "" if both inputs are empty (no rows are ever inserted in that
// case but defensive: nil pointer math elsewhere).
func JiebaSegmentForSearch(title, content string) string {
	combined := strings.TrimSpace(title) + "\n" + strings.TrimSpace(content)
	if combined == "" {
		return ""
	}
	tokens := types.Jieba.CutForSearch(combined, true)
	// Filter out pure punctuation / single-char stop tokens that the
	// `simple` regconfig would otherwise treat as noise. Keeps the index
	// lean without losing any meaningful Chinese word.
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// Drop pure ASCII whitespace; everything else (including single-char
		// Chinese tokens like "的" "了") is left alone for the indexer to
		// decide on.
		out = append(out, t)
	}
	return strings.Join(out, " ")
}
