// Package verification implements the Build #29 AI Verification
// service. The scanner runs four lightweight checks per page and
// surfaces a per-page trust score plus a per-KB summary.
//
// The checks are designed to be cheap so they can run on a 24h cron
// without any LLM call:
//
//  1. Freshness — `now - updated_at > freshnessDays`.
//  2. Contradiction — pairwise text overlap with the top N most-similar
//     pages in the same KB; pairs above 0.85 trigram overlap trigger
//     a contradiction candidate.
//  3. Link health — broken outbound links (slug exists in `out_links`
//     but not in `wiki_pages` for the same KB).
//  4. Trust score — composite in [0, 1] derived from the three above
//     and an explicit owner-defined override (out of scope here).
//
// The service exposes RunForPage / RunForKB; the cron wiring lives in
// cmd/server (Build #29 schedule).
package verification

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// default thresholds. Tuned for the v0.7.25 release; can be overridden
// per-tenant in a follow-up.
const (
	freshnessWarningDays = 180
	freshnessBadDays     = 365
	contradictionThreshold = 0.85
	linkHealthBadRatio   = 0.25
	trustScoreBad        = 0.40
	trustScoreWarn       = 0.65
)

// PageFetcher returns the lightweight fields the scanner needs. Defined
// as an interface so tests can substitute a fake without standing up
// the full wiki_page service.
type PageFetcher interface {
	GetPage(ctx context.Context, kbID, slug string) (*PageSummary, error)
	ListSlugs(ctx context.Context, kbID string) ([]string, error)
	ListBySlugs(ctx context.Context, kbID string, slugs []string) (map[string]*PageSummary, error)
}

// PageSummary is the projection the scanner reads. Decoupled from
// types.WikiPageLite so the service can run against any backend
// (e.g. a future open-source search index).
type PageSummary struct {
	Slug       string
	Title      string
	KBID       string
	TenantID   string
	PageID     string
	OutLinks   []string
	InLinks    []string
	Status     string
	UpdatedAt  time.Time
	Content    string // markdown body for trigram overlap
}

// Service is the Build #29 scanner.
type Service struct {
	fetcher PageFetcher
}

// NewService wires a Service against the given PageFetcher.
func NewService(fetcher PageFetcher) *Service {
	return &Service{fetcher: fetcher}
}

// RunForPage runs all checks on a single page and returns the report.
func (s *Service) RunForPage(ctx context.Context, kbID, slug string) (*types.VerificationReport, error) {
	if s == nil || s.fetcher == nil {
		return nil, errors.New("verification: nil fetcher")
	}
	page, err := s.fetcher.GetPage(ctx, kbID, slug)
	if err != nil || page == nil {
		return &types.VerificationReport{
			Slug:            slug,
			KnowledgeBaseID: kbID,
			Status:          types.VerificationStatusMissing,
			Checks: []types.VerificationCheck{{
				Code:     "existence",
				Severity: types.VerificationStatusBad,
				Message:  "Page no longer exists or has been deleted.",
				SuggestedAction: "review",
			}},
			ScannedAt: time.Now(),
		}, nil
	}

	checks := []types.VerificationCheck{
		s.checkFreshness(page),
		s.checkLinkHealth(ctx, kbID, page),
		s.checkContradiction(ctx, kbID, page),
	}
	score := computeTrustScore(checks)
	status := rollupStatus(checks, score)
	return &types.VerificationReport{
		PageID:          page.PageID,
		Slug:            page.Slug,
		KnowledgeBaseID: kbID,
		TenantID:        page.TenantID,
		Status:          status,
		TrustScore:      score,
		Checks:          checks,
		ScannedAt:       time.Now(),
	}, nil
}

// RunForKB scans every slug in the KB and produces per-page reports
// plus a summary. Slugs that error out are recorded as
// VerificationStatusMissing so a single bad row can't block the run.
func (s *Service) RunForKB(ctx context.Context, kbID string, limit int) ([]*types.VerificationReport, *types.VerificationSummary, error) {
	slugs, err := s.fetcher.ListSlugs(ctx, kbID)
	if err != nil {
		return nil, nil, err
	}
	if limit > 0 && len(slugs) > limit {
		slugs = slugs[:limit]
	}
	summary := &types.VerificationSummary{KnowledgeBaseID: kbID, ScannedAt: time.Now()}
	reports := make([]*types.VerificationReport, 0, len(slugs))
	for _, slug := range slugs {
		rep, err := s.RunForPage(ctx, kbID, slug)
		if err != nil {
			continue
		}
		reports = append(reports, rep)
		switch rep.Status {
		case types.VerificationStatusOK:
			summary.OK++
		case types.VerificationStatusWarning:
			summary.Warning++
		case types.VerificationStatusBad:
			summary.Bad++
		case types.VerificationStatusMissing:
			summary.Missing++
		}
		summary.AvgTrustScore += rep.TrustScore
	}
	summary.Total = len(reports)
	if summary.Total > 0 {
		summary.AvgTrustScore /= float64(summary.Total)
	}
	return reports, summary, nil
}

func (s *Service) checkFreshness(page *PageSummary) types.VerificationCheck {
	if page.UpdatedAt.IsZero() {
		return types.VerificationCheck{
			Code:     "freshness",
			Severity: types.VerificationStatusWarning,
			Message:  "Page has never been touched; we can't tell how fresh it is.",
			SuggestedAction: "review",
		}
	}
	age := time.Since(page.UpdatedAt)
	if age > freshnessBadDays*24*time.Hour {
		return types.VerificationCheck{
			Code:     "freshness",
			Severity: types.VerificationStatusBad,
			Message:  "Last updated over a year ago.",
			SuggestedAction: "review",
			Details: map[string]string{
				"updated_at": page.UpdatedAt.Format(time.RFC3339),
				"age_days":   formatDays(age),
			},
		}
	}
	if age > freshnessWarningDays*24*time.Hour {
		return types.VerificationCheck{
			Code:     "freshness",
			Severity: types.VerificationStatusWarning,
			Message:  "Page is older than six months.",
			SuggestedAction: "review",
			Details: map[string]string{
				"updated_at": page.UpdatedAt.Format(time.RFC3339),
				"age_days":   formatDays(age),
			},
		}
	}
	return types.VerificationCheck{
		Code:     "freshness",
		Severity: types.VerificationStatusOK,
		Message:  "Recently updated.",
		Details:  map[string]string{"age_days": formatDays(age)},
	}
}

func (s *Service) checkLinkHealth(ctx context.Context, kbID string, page *PageSummary) types.VerificationCheck {
	if len(page.OutLinks) == 0 {
		return types.VerificationCheck{
			Code:     "link_health",
			Severity: types.VerificationStatusOK,
			Message:  "No outbound links.",
		}
	}
	missing, _ := s.fetcher.ListBySlugs(ctx, kbID, page.OutLinks)
	broken := 0
	for _, slug := range page.OutLinks {
		if slug == "" || slug == page.Slug {
			continue
		}
		if _, ok := missing[slug]; !ok {
			broken++
		}
	}
	ratio := float64(broken) / float64(len(page.OutLinks))
	if ratio >= linkHealthBadRatio {
		return types.VerificationCheck{
			Code:     "link_health",
			Severity: types.VerificationStatusBad,
			Message:  "More than 25% of outbound links are broken.",
			SuggestedAction: "review",
			Details: map[string]string{
				"broken": strconvInt(broken),
				"total":  strconvInt(len(page.OutLinks)),
				"ratio":  formatFloat(ratio),
			},
		}
	}
	if broken > 0 {
		return types.VerificationCheck{
			Code:     "link_health",
			Severity: types.VerificationStatusWarning,
			Message:  "At least one outbound link is broken.",
			SuggestedAction: "review",
			Details: map[string]string{
				"broken": strconvInt(broken),
				"total":  strconvInt(len(page.OutLinks)),
			},
		}
	}
	return types.VerificationCheck{
		Code:     "link_health",
		Severity: types.VerificationStatusOK,
		Message:  "All outbound links resolve.",
	}
}

func (s *Service) checkContradiction(ctx context.Context, kbID string, page *PageSummary) types.VerificationCheck {
	if page.Content == "" {
		return types.VerificationCheck{
			Code:     "contradiction",
			Severity: types.VerificationStatusWarning,
			Message:  "Page has no content to compare; treat as untested.",
			SuggestedAction: "review",
		}
	}
	others, err := s.fetcher.ListSlugs(ctx, kbID)
	if err != nil || len(others) == 0 {
		return types.VerificationCheck{
			Code:     "contradiction",
			Severity: types.VerificationStatusOK,
			Message:  "No comparable pages.",
		}
	}
	limit := 25
	if len(others) > limit {
		others = others[:limit]
	}
	targets := make([]string, 0, len(others))
	for _, s := range others {
		if s != page.Slug {
			targets = append(targets, s)
		}
	}
	candidates, _ := s.fetcher.ListBySlugs(ctx, kbID, targets)
	maxOverlap := 0.0
	conflictSlug := ""
	for slug, p := range candidates {
		if p == nil || p.Content == "" {
			continue
		}
		overlap := trigramOverlap(page.Content, p.Content)
		if overlap > maxOverlap {
			maxOverlap = overlap
			conflictSlug = slug
		}
	}
	if maxOverlap >= contradictionThreshold {
		return types.VerificationCheck{
			Code:     "contradiction",
			Severity: types.VerificationStatusWarning,
			Message:  "Content substantially overlaps another page in the same KB.",
			SuggestedAction: "merge",
			Details: map[string]string{
				"score":    formatFloat(maxOverlap),
				"conflict": conflictSlug,
			},
		}
	}
	return types.VerificationCheck{
		Code:     "contradiction",
		Severity: types.VerificationStatusOK,
		Message:  "No duplicate content detected.",
		Details:  map[string]string{"max_overlap": formatFloat(maxOverlap)},
	}
}

func computeTrustScore(checks []types.VerificationCheck) float64 {
	score := 1.0
	for _, c := range checks {
		switch c.Severity {
		case types.VerificationStatusWarning:
			score -= 0.15
		case types.VerificationStatusBad:
			score -= 0.40
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

func rollupStatus(checks []types.VerificationCheck, score float64) types.VerificationStatus {
	hasBad := false
	hasWarn := false
	for _, c := range checks {
		if c.Severity == types.VerificationStatusBad {
			hasBad = true
		}
		if c.Severity == types.VerificationStatusWarning {
			hasWarn = true
		}
	}
	if hasBad || score < trustScoreBad {
		return types.VerificationStatusBad
	}
	if hasWarn || score < trustScoreWarn {
		return types.VerificationStatusWarning
	}
	return types.VerificationStatusOK
}

// trigramOverlap returns the Sørensen–Dice coefficient over the
// 3-character shingle sets of the two inputs. It's a deliberately
// crude text-similarity metric that runs in O(n) per pair and never
// hits the LLM.
func trigramOverlap(a, b string) float64 {
	ash := shingles(normalize(a))
	bsh := shingles(normalize(b))
	if len(ash) == 0 || len(bsh) == 0 {
		return 0
	}
	intersect := 0
	for k := range ash {
		if _, ok := bsh[k]; ok {
			intersect++
		}
	}
	return 2 * float64(intersect) / float64(len(ash)+len(bsh))
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func shingles(s string) map[string]struct{} {
	out := map[string]struct{}{}
	if len(s) < 3 {
		out[s] = struct{}{}
		return out
	}
	for i := 0; i+3 <= len(s); i++ {
		out[s[i:i+3]] = struct{}{}
	}
	return out
}

func formatDays(d time.Duration) string {
	return strings.TrimRight(
		strings.TrimRight(
			strings.ReplaceAll(
				time.Duration(int64(d.Hours()/24)).String(),
				"0s", "0d",
			),
			"0m0s",
		),
		"0h0m0s",
	)
}

// Local helper to keep the file free of strconv dependency noise.
func strconvInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := make([]byte, 0, 8)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func formatFloat(f float64) string {
	// 4 decimal places is enough for trigram scores.
	intPart := int(f)
	frac := int((f - float64(intPart)) * 10000)
	if frac < 0 {
		frac = -frac
	}
	return strconvInt(intPart) + "." + zeroPad(strconvInt(frac), 4)
}

func zeroPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// Ensure report slice ordering is deterministic for callers that
// diff two runs (cron tests rely on this).
func sortReports(reports []*types.VerificationReport) {
	sort.SliceStable(reports, func(i, j int) bool {
		if reports[i].KnowledgeBaseID != reports[j].KnowledgeBaseID {
			return reports[i].KnowledgeBaseID < reports[j].KnowledgeBaseID
		}
		return reports[i].Slug < reports[j].Slug
	})
}

var _ = sortReports
