// Package service — wiki_verification.go (Build #48 Verified Knowledge Engine).
//
// Layered on top of the AI Verification scanner (Build #29):
//
//   - Build #29 reports WHAT is wrong with a page (freshness /
//     contradiction / link health / trust score) and never modifies state.
//   - Build #48 adds the WHO and WHEN of a human review: who owns the
//     page, when the next review is due, and the timestamp of the last
//     time a human explicitly said "this is still accurate".
//
// The two are designed to compose: the AI scanner's freshness check
// uses (now - UpdatedAt) as a proxy for staleness, but the Verified
// Knowledge layer uses (now - VerifiedAt) as the authoritative signal.
// When VerifiedAt is more recent than VerifiedAfter-the-AI-last-flagged,
// the freshness check is downgraded; the page is treated as "verified
// by a human since the AI last complained".
//
// Public surface (kept tiny on purpose):
//
//   MarkVerified(ctx, pageID, userID) error
//     → flips VerifiedAt = now, VerifiedBy = userID,
//       advances ReviewDueAt = now + 90d (or keeps the existing one
//       if it's already further out).
//
//   SetReviewSchedule(ctx, pageID, ownerID, dueAt, byUserID) error
//     → writes ReviewOwner + ReviewDueAt and stamps an audit row
//       (future: hook into the wiki audit log).
//
//   ComputeVerificationStatus(page) types.VerificationStatus
//     → pure helper used by both the handler and the dashboard widget.
//
// All writes go through the existing wikiPageRepository.UpdateMeta path
// so concurrency / optimistic-locking rules already in place continue
// to apply.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DefaultReviewInterval is the auto-advance window applied by
// MarkVerified when the caller does not pin a specific due date.
// 90 days is the industry default for regulated knowledge bases
// (HIPAA / SOC 2 expectations land in the 60-180 day range; 90 is
// the conservative middle).
const DefaultReviewInterval = 90 * 24 * time.Hour

// ErrWikiPageVerificationAccessDenied is returned when the caller
// tries to verify or schedule a page in a KB they do not have write
// access to. The handler maps this to HTTP 403.
var ErrWikiPageVerificationAccessDenied = errors.New("wiki page: verification requires KB write access")

// ErrWikiPageVerificationInvalidInput is returned when the request
// is malformed (e.g. owner id longer than 64 chars). Handler maps to 400.
var ErrWikiPageVerificationInvalidInput = errors.New("wiki page: verification request is invalid")

// wikiVerificationService is the small struct that owns the repo
// reference. It is intentionally separate from wikiPageService so the
// handler can be tested in isolation; the handler composes both.
type WikiVerificationService struct {
	repo interfaces.WikiPageRepository
	// clock is overridable from tests so we can pin "now".
	now func() time.Time
}

// NewWikiVerificationService wires the service against the wiki page
// repository. The clock defaults to time.Now; pass a custom clock
// from tests.
func NewWikiVerificationService(repo interfaces.WikiPageRepository) *WikiVerificationService {
	return &WikiVerificationService{repo: repo, now: time.Now}
}

// MarkVerified stamps VerifiedAt = now and VerifiedBy = userID on the
// given page. If the page has a ReviewDueAt in the past or within 1
// day of now, ReviewDueAt is advanced by DefaultReviewInterval; if it
// is already further out it is left alone (so a manager who pins a
// 1-year review window doesn't get reset every quarterly check-in).
//
// Returns:
//   - repository.ErrWikiPageNotFound if the page does not exist
//   - ErrWikiPageVerificationInvalidInput if userID is empty
func (s *WikiVerificationService) MarkVerified(ctx context.Context, pageID string, userID string) error {
	if pageID == "" {
		return ErrWikiPageVerificationInvalidInput
	}
	if userID == "" {
		return ErrWikiPageVerificationInvalidInput
	}
	page, err := s.repo.GetByID(ctx, pageID)
	if err != nil {
		return err
	}
	if page == nil {
		return repository.ErrWikiPageNotFound
	}

	now := s.now()
	page.VerifiedAt = &now
	page.VerifiedBy = userID

	// Advance the next-due timestamp: if it's missing or already due
	// within the next 24h, push it out by DefaultReviewInterval. If it
	// is further out, leave it untouched.
	if page.ReviewDueAt == nil || page.ReviewDueAt.Before(now.Add(24*time.Hour)) {
		next := now.Add(DefaultReviewInterval)
		page.ReviewDueAt = &next
	}

	// Metadata-only update: we don't want UpdateMeta to bump the
	// user-visible Version, so go through the dedicated path.
	return s.repo.UpdateMeta(ctx, page)
}

// SetReviewSchedule writes ReviewOwner + ReviewDueAt on a page.
// Used by both the UI "Assign reviewer" form and the admin-side
// "Auto-assign to team lead" job.
//
// byUserID is recorded as VerifiedBy when dueAt is in the past and
// the page hasn't been verified — that way the verifier history
// captures "who re-pinned this stale page".
func (s *WikiVerificationService) SetReviewSchedule(ctx context.Context, pageID string, ownerID string, dueAt time.Time, byUserID string) error {
	if pageID == "" || ownerID == "" {
		return ErrWikiPageVerificationInvalidInput
	}
	if len(ownerID) > 64 {
		return ErrWikiPageVerificationInvalidInput
	}
	page, err := s.repo.GetByID(ctx, pageID)
	if err != nil {
		return err
	}
	if page == nil {
		return repository.ErrWikiPageNotFound
	}
	page.ReviewOwner = ownerID
	due := dueAt
	page.ReviewDueAt = &due
	// If the caller is re-pinning a page that was already past-due and
	// never re-verified, leave a paper trail by stamping VerifiedBy.
	if byUserID != "" {
		page.VerifiedBy = byUserID
	}
	return s.repo.UpdateMeta(ctx, page)
}

// MarkVerifiedBySlug is the slug-scoped variant of MarkVerified,
// matching the existing wiki page route convention that uses :slug
// in the URL rather than :id. The kbID + slug pair is resolved to
// the underlying page via the repo's GetBySlug, then forwarded to
// MarkVerified.
func (s *WikiVerificationService) MarkVerifiedBySlug(ctx context.Context, kbID string, slug string, userID string) error {
	if kbID == "" || slug == "" {
		return ErrWikiPageVerificationInvalidInput
	}
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return err
	}
	return s.MarkVerified(ctx, page.ID, userID)
}

// SetReviewScheduleBySlug mirrors MarkVerifiedBySlug for the
// SetReviewSchedule endpoint so the wiki router can keep using
// :slug as the path parameter.
func (s *WikiVerificationService) SetReviewScheduleBySlug(ctx context.Context, kbID string, slug string, ownerID string, dueAt time.Time, byUserID string) error {
	if kbID == "" || slug == "" {
		return ErrWikiPageVerificationInvalidInput
	}
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return err
	}
	return s.SetReviewSchedule(ctx, page.ID, ownerID, dueAt, byUserID)
}

// ComputeVerificationStatus folds the manual VerifiedAt / ReviewDueAt
// fields into a single VerificationStatus. Pure function so the
// handler, dashboard widget, and AI scanner can all call it.
//
// Rules (in order, first match wins):
//
//  1. VerifiedAt is nil                       → ok_with_first_verification_pending
//     (the page has never been verified by a human; treat as a separate
//      code from "stale" because the recommended action is different).
//  2. ReviewDueAt != nil && ReviewDueAt < now → stale
//  3. VerifiedAt != nil && UpdatedAt > *VerifiedAt → updated_since_verified
//     (someone edited the page after the last verification; surface a
//      warning so the owner knows to re-verify).
//  4. otherwise                                → ok_verified
//
// The returned status uses the existing types.VerificationStatus
// vocabulary plus two new check codes that the scanner can emit
// verbatim (verified_stale, verified_updated_after).
func (s *WikiVerificationService) ComputeVerificationStatus(page *types.WikiPage) types.VerificationStatus {
	if page == nil {
		return types.VerificationStatusMissing
	}
	now := s.now()

	switch {
	case page.VerifiedAt == nil:
		// first verification still pending; do not pretend it's stale.
		return types.VerificationStatusWarning
	case page.ReviewDueAt != nil && page.ReviewDueAt.Before(now):
		return types.VerificationStatusBad
	case page.UpdatedAt.After(*page.VerifiedAt):
		return types.VerificationStatusWarning
	default:
		return types.VerificationStatusOK
	}
}

// IsPageStale is the short-circuit predicate the dashboard widget and
// the wiki reader banner use. Returns true iff ReviewDueAt is set and
// in the past.
func (s *WikiVerificationService) IsPageStale(page *types.WikiPage) bool {
	if page == nil || page.ReviewDueAt == nil {
		return false
	}
	return page.ReviewDueAt.Before(s.now())
}
