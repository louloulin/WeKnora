package dockb

import (
	"context"
	"strings"
)

// NoopSummariser is a deterministic stub used when no LLM is
// configured. It produces a short summary by taking the first 200
// characters of the input and synthesising simple keyword tags by
// splitting on whitespace and filtering stopwords.
//
// The stub is intentionally minimal: it must not depend on any
// model server and must produce stable output for tests.
type NoopSummariser struct{}

var noopStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "of": {}, "to": {},
	"in": {}, "on": {}, "for": {}, "is": {}, "are": {}, "be": {}, "by": {},
}

// Summarize implements interfaces.Summariser.
func (NoopSummariser) Summarize(_ context.Context, text string) (string, []string, []string, error) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 200 {
		trimmed = trimmed[:200] + "..."
	}
	tokens := strings.FieldsFunc(strings.ToLower(trimmed), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	seen := map[string]struct{}{}
	keyphrases := []string{}
	tags := []string{}
	for _, t := range tokens {
		if _, skip := noopStopwords[t]; skip {
			continue
		}
		if len(t) < 3 {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if len(keyphrases) < 5 {
			keyphrases = append(keyphrases, t)
		}
		if len(tags) < 3 {
			tags = append(tags, t)
		}
		if len(keyphrases) >= 5 && len(tags) >= 3 {
			break
		}
	}
	return trimmed, keyphrases, tags, nil
}
