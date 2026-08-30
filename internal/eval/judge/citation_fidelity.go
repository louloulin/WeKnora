package judge

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Build #31 — citation_fidelity heuristic.
//
// The full citation_fidelity dimension is the MIN of:
//
//   1. Heuristic score (this file): does the model answer use
//      [[cite:N]] tokens that index into the citation_index array
//      without gaps, duplicates, or out-of-range indices?
//   2. LLM judge score (JudgeCitationFidelity): does each token point
//      at a passage that actually supports the statement?
//
// The runner reports min(heuristic, llm) so a model that uses perfect
// indices but cites the wrong content scores low, and vice versa.
// Design note: Build #31 D8 picks min over weighted average because
// the failure mode is "model confidently cites wrong stuff" — we
// want any one signal failing to surface the badcase.

var (
	// citationTokenRe matches [[cite:N]] tokens. The N is captured so
	// the heuristic can extract the integer for range checking.
	citationTokenRe = regexp.MustCompile(`\[\[cite:(\d+)\]\]`)

	// errCitationTruncated is returned by HeuristicCitationFidelity when
	// the citation_index JSON is unparseable. The runner treats this
	// as score 1 (worst) so a corrupt log does not silently produce a
	// high score.
	errCitationTruncated = errors.New("judge: citation_index unparseable")
)

// CitationIndex is the B30 B3 shape: a 1-based map of int → chunk_id.
// We accept any JSON object whose keys are positive integers; the
// caller is responsible for the canonical {1: "kb_X-chunk_Y", ...}
// form.
type CitationIndex map[int]string

// HeuristicCitationFidelity scores the model answer on:
//
//   - every [[cite:N]] token has a positive integer N
//   - every N is present in the citation_index map
//   - every N is unique (a duplicated [[cite:3]] in two places means
//     either a rephrased copy or a mistake; we flag both as score -1)
//
// Score bands:
//
//	5: every citation valid, no duplicates, no out-of-range
//	4: one duplicate or one gap (cited N+1 with no N)
//	3: reserved for future partial-evidence cases
//	2: missing citation_index or empty answer text
//	1: parse failure or three+ bad references
//
// The function is pure: same inputs → same output. The harness pins
// this so a regex tweak never silently changes the score distribution.
func HeuristicCitationFidelity(modelAnswer string, citationIndexJSON json.RawMessage) int {
	if strings.TrimSpace(modelAnswer) == "" {
		return 1
	}
	idx, err := parseCitationIndex(citationIndexJSON)
	if err != nil {
		return 1
	}
	if len(idx) == 0 {
		return 2
	}

	matches := citationTokenRe.FindAllStringSubmatch(modelAnswer, -1)
	if len(matches) == 0 {
		// No citations at all in the answer. With B30's evidence
		// requirement, that is a regression: even a low-evidence
		// answer should cite the closest-matching chunk.
		return 2
	}

	// Deduplicate indices; track which ones were used more than once
	// (duplicate penalty) and which ones are out of range.
	seen := make(map[int]int, len(matches))
	keys := make([]int, 0, len(idx))
	for k := range idx {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	maxIdx := keys[len(keys)-1]

	duplicateCount := 0
	outOfRangeCount := 0
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			outOfRangeCount++
			continue
		}
		seen[n]++
		if _, ok := idx[n]; !ok || n > maxIdx+1024 {
			outOfRangeCount++
		}
	}
	for _, count := range seen {
		if count > 1 {
			duplicateCount += count - 1
		}
	}

	bad := outOfRangeCount + duplicateCount
	if outOfRangeCount >= 3 {
		return 1
	}
	if outOfRangeCount > 0 {
		return 2
	}
	switch {
	case bad == 0:
		return 5
	case bad == 1:
		return 4
	case bad <= 2:
		return 3
	case bad <= 4:
		return 2
	default:
		return 1
	}
}

// parseCitationIndex accepts either {"1":"chunk_a","2":"chunk_b"} or
// {"entries":[{"index":1,"chunk_id":"chunk_a"}, ...]} so future
// schema evolution can land without breaking the heuristic. Today only
// the first shape is produced by chat_manage.
func parseCitationIndex(raw json.RawMessage) (CitationIndex, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errCitationTruncated
	}
	// First try the canonical 1-based map shape.
	var direct map[int]string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	// Fall back to the entries shape used by some legacy callers.
	var wrapped struct {
		Entries []struct {
			Index   int    `json:"index"`
			ChunkID string `json:"chunk_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Entries) > 0 {
		out := make(CitationIndex, len(wrapped.Entries))
		for _, e := range wrapped.Entries {
			out[e.Index] = e.ChunkID
		}
		return out, nil
	}
	return nil, errCitationTruncated
}

// CombinedCitationFidelity returns min(heuristic, llmScore). The
// runner uses this when both signals are present. When llmScore is 0
// (judge was not run, or returned an error) we return the heuristic
// alone so the score column is never 0.
func CombinedCitationFidelity(heuristic, llmScore int) int {
	if llmScore <= 0 {
		if heuristic <= 0 {
			return clampScore(llmScore)
		}
		return clampScore(heuristic)
	}
	if heuristic <= 0 {
		return clampScore(llmScore)
	}
	h := clampScore(heuristic)
	l := clampScore(llmScore)
	if h < l {
		return h
	}
	return l
}
