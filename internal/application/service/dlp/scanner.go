package dlp

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Match represents a single DLP hit: which rule fired, what text matched,
// and a context window (up to 32 chars on each side of the match).
type Match struct {
	RuleID         uint64
	PolicyID       uint64
	PatternName    string // builtin name / regex / dictionary key
	MatchedValue   string // up to 256 chars
	Context        string // up to 512 chars (truncated to that cap)
	Severity       string
	Action         string
	CompiledRegex  *regexp.Regexp
	DictEntry      string // non-empty for dictionary hits
}

// scanner wraps the pattern compilation cache. Each scanner is safe for
// concurrent use — the compiled-regex map is read-only after construction.
type scanner struct {
	rules []scannedRule
}

type scannedRule struct {
	Rule         types.DLPRule
	PolicyID     uint64
	Compiled     *regexp.Regexp
	DictEntries  []string // for dictionary rules; pre-lowercased
	Severity     string
	Action       string
}

// newScanner compiles every rule and returns a ready-to-scan bundle.
// Invalid regex rules are logged and skipped — a malformed rule should
// not block all the others from firing.
func newScanner(policyRules []policyRule) *scanner {
	out := &scanner{}
	for _, pr := range policyRules {
		sr := scannedRule{
			Rule:     pr.Rule,
			PolicyID: pr.PolicyID,
			Severity: pr.Rule.Severity,
			Action:   pr.Action,
		}
		if !pr.Rule.Enabled {
			continue
		}
		switch pr.Rule.PatternType {
		case types.DLPPatternBuiltin:
			re, ok := builtinPatterns[pr.Rule.PatternValue]
			if !ok {
				logger.Warnf(context.Background(),
					"[DLP] unknown builtin %q in rule %d — skipped",
					pr.Rule.PatternValue, pr.Rule.ID)
				continue
			}
			sr.Compiled = re
			sr.DictEntries = nil
		case types.DLPPatternRegex:
			re, err := regexp.Compile(pr.Rule.PatternValue)
			if err != nil {
				logger.Warnf(context.Background(),
					"[DLP] regex compile failed for rule %d: %v", pr.Rule.ID, err)
				continue
			}
			sr.Compiled = re
		case types.DLPPatternDictionary:
			// Split on commas; trim whitespace; drop empties.
			for _, e := range strings.Split(pr.Rule.PatternValue, ",") {
				e = strings.TrimSpace(strings.ToLower(e))
				if e != "" {
					sr.DictEntries = append(sr.DictEntries, e)
				}
			}
			if len(sr.DictEntries) == 0 {
				logger.Warnf(context.Background(),
					"[DLP] empty dictionary in rule %d — skipped", pr.Rule.ID)
				continue
			}
		default:
			logger.Warnf(context.Background(),
				"[DLP] unknown pattern_type %q in rule %d", pr.Rule.PatternType, pr.Rule.ID)
			continue
		}
		out.rules = append(out.rules, sr)
	}
	return out
}

// policyRule bundles a rule with its parent policy metadata.
type policyRule struct {
	Rule     types.DLPRule
	PolicyID uint64
	Action   string
	Severity string
}

// scan runs every compiled regex / dictionary entry against text and
// returns the list of matches. The match cap (maxMatches) prevents a
// pathological document with thousands of credit-card mentions from
// producing an unbounded violation list.
func (s *scanner) scan(text string, maxMatches int) []Match {
	if maxMatches <= 0 {
		maxMatches = 200
	}
	out := make([]Match, 0, 8)
	for _, sr := range s.rules {
		if sr.Compiled != nil {
			idx := sr.Compiled.FindAllStringIndex(text, -1)
			for _, m := range idx {
				if len(out) >= maxMatches {
					return out
				}
				value := truncate(text[m[0]:m[1]], 256)
				out = append(out, Match{
					RuleID:       sr.Rule.ID,
					PolicyID:     sr.PolicyID,
					PatternName:  sr.Rule.PatternValue,
					MatchedValue: value,
					Context:      contextWindow(text, m[0], m[1], 32, 512),
					Severity:     sr.Severity,
					Action:       sr.Action,
					CompiledRegex: sr.Compiled,
				})
			}
		}
		// Dictionary hits — case-insensitive substring scan.
		for _, entry := range sr.DictEntries {
			if len(out) >= maxMatches {
				return out
			}
			lower := strings.ToLower(text)
			from := 0
			for {
				idx := strings.Index(lower[from:], entry)
				if idx < 0 {
					break
				}
				start := from + idx
				end := start + len(entry)
				if len(out) >= maxMatches {
					return out
				}
				value := truncate(text[start:end], 256)
				out = append(out, Match{
					RuleID:      sr.Rule.ID,
					PolicyID:    sr.PolicyID,
					PatternName: "dict:" + entry,
					MatchedValue: value,
					Context:     contextWindow(text, start, end, 32, 512),
					Severity:    sr.Severity,
					Action:      sr.Action,
					DictEntry:   entry,
				})
				from = end
			}
		}
	}
	return out
}

// contextWindow returns text[start-contextBefore : end+contextAfter],
// clamped to the document bounds and truncated to maxTotal runes.
func contextWindow(text string, start, end, contextBefore, maxTotal int) string {
	from := start - contextBefore
	if from < 0 {
		from = 0
	}
	to := end + contextBefore
	if to > len(text) {
		to = len(text)
	}
	out := text[from:to]
	if len(out) > maxTotal {
		out = out[:maxTotal]
	}
	return out
}

// truncate caps s at max runes.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// Compile-time sanity: ensure time is used (helpful for future atime /
// time-window filters).
var _ = time.Now
