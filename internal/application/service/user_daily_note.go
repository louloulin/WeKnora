// Package service — user_daily_note.go (Build #45.a Daily Note 默认页).
//
// Minimal slice of the v25 P0 gap #12: per-(user, kb, calendar_date)
// row that the home page can pin as "today's note", plus the range
// listing the dashboard widget renders.
//
// Layering rules:
//   - The handler resolves (user_id, tenant_id) from the auth context
//     and passes them in; the service does NOT trust request bodies
//     for those fields.
//   - The note date is server-side UTC; client-supplied dates are
//     parsed but always anchored to UTC midnight.
//   - Linked wiki_pages row (PageID) is created lazily on first GET
//     so the write path stays cheap. The InlineAIService can be
//     plugged in to populate Summary at the same point — that's a
//     Build #45.a follow-up, not part of this commit.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// DailyNoteMaxRangeDays caps the date-range query so a malicious /
// buggy client can't pull every note the user has ever written in
// one call. 366 days = 1 year + 1 leap day.
const DailyNoteMaxRangeDays = 366

// DailyNoteDefaultLimit is the default page size when the caller
// doesn't pin one. 30 ≈ one note per weekday-month.
const DailyNoteDefaultLimit = 30

// ErrUserDailyNoteKBRequired is returned when the handler invokes
// GetOrCreate with an empty kb_id — every note is KB-scoped on
// purpose, so this is a 400.
var ErrUserDailyNoteKBRequired = errors.New("daily note: knowledge_base_id is required")

// ErrUserDailyNoteRangeInvalid is returned when the from/to query
// args are unparseable or to < from.
var ErrUserDailyNoteRangeInvalid = errors.New("daily note: invalid date range")

// ErrUserDailyNoteNotFound is the service-level alias of the repo's
// not-found error so callers don't need to import the repository
// package just for errors.Is.
var ErrUserDailyNoteNotFound = repository.ErrUserDailyNoteNotFound

// userDailyNoteService is the Build #45.a service. Kept small and
// synchronous — no async fan-out, no LLM calls in this slice.
type UserDailyNoteService struct {
	repo interfaces.UserDailyNoteRepository
	// now is overridable from tests so we can pin "today" to a
	// deterministic date.
	now func() time.Time
}

// NewUserDailyNoteService wires the service against a repo. The
// clock defaults to time.Now; pass a custom clock from tests.
func NewUserDailyNoteService(repo interfaces.UserDailyNoteRepository) *UserDailyNoteService {
	return &UserDailyNoteService{repo: repo, now: time.Now}
}

// GetOrCreateToday returns today's note for (userID, kbID, tenantID),
// creating the row + linked wiki_pages placeholder on the first call
// of the day. The returned note is safe to mutate on the caller side;
// the service writes through Update if needed.
//
// tenantID is required so the row is correctly tenant-scoped on
// insert; the unique constraint is (user_id, kb_id, note_date), so
// the same user can hold one note per KB per day.
func (s *UserDailyNoteService) GetOrCreateToday(ctx context.Context, tenantID uint64, userID string, kbID string) (*types.UserDailyNote, error) {
	if kbID == "" {
		return nil, ErrUserDailyNoteKBRequired
	}
	if userID == "" {
		return nil, ErrUserDailyNoteKBRequired
	}
	day := s.today()
	existing, err := s.repo.GetByUserDate(ctx, userID, kbID, day)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrUserDailyNoteNotFound) {
		return nil, err
	}
	// First call of the day — create the stub. On a race (two
	// concurrent requests on the same morning) the unique
	// constraint turns the second Create into a Conflict, which we
	// resolve by re-reading the row.
	note := &types.UserDailyNote{
		ID:               uuid.NewString(),
		TenantID:         tenantID,
		UserID:           userID,
		KnowledgeBaseID:  kbID,
		NoteDate:         day,
		Slug:             fmt.Sprintf("daily/%s", day.Format("2006-01-02")),
		PageID:           "", // lazy on first GET-with-AI
		Title:            fmt.Sprintf("%s %s", types.DailyNoteDefaultTitlePrefix, day.Format("2006-01-02")),
		Content:          s.defaultContent(day),
		Summary:          "",
		CreatedAt:        s.now(),
		UpdatedAt:        s.now(),
	}
	if err := s.repo.Create(ctx, note); err != nil {
		if errors.Is(err, repository.ErrUserDailyNoteConflict) {
			return s.repo.GetByUserDate(ctx, userID, kbID, day)
		}
		return nil, err
	}
	return note, nil
}

// GetOrCreateDate is the date-keyed variant of GetOrCreateToday.
// Mostly used by the dashboard widget for the "jump to yesterday /
// tomorrow" buttons.
func (s *UserDailyNoteService) GetOrCreateDate(ctx context.Context, tenantID uint64, userID string, kbID string, day time.Time) (*types.UserDailyNote, error) {
	if kbID == "" || userID == "" {
		return nil, ErrUserDailyNoteKBRequired
	}
	day = truncateDayUTC(day)
	existing, err := s.repo.GetByUserDate(ctx, userID, kbID, day)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrUserDailyNoteNotFound) {
		return nil, err
	}
	note := &types.UserDailyNote{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		UserID:          userID,
		KnowledgeBaseID: kbID,
		NoteDate:        day,
		Slug:            fmt.Sprintf("daily/%s", day.Format("2006-01-02")),
		Title:           fmt.Sprintf("%s %s", types.DailyNoteDefaultTitlePrefix, day.Format("2006-01-02")),
		Content:         s.defaultContent(day),
		CreatedAt:       s.now(),
		UpdatedAt:       s.now(),
	}
	if err := s.repo.Create(ctx, note); err != nil {
		if errors.Is(err, repository.ErrUserDailyNoteConflict) {
			return s.repo.GetByUserDate(ctx, userID, kbID, day)
		}
		return nil, err
	}
	return note, nil
}

// UpdateContent rewrites title + content + summary on an existing
// note. The handler passes the userID from the auth context so a
// token can't be used to edit another user's note.
func (s *UserDailyNoteService) UpdateContent(ctx context.Context, userID string, noteID string, title string, content string, summary string) (*types.UserDailyNote, error) {
	existing, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		// Don't leak ownership — return NotFound rather than
		// Forbidden so cross-tenant probing stays neutral.
		return nil, ErrUserDailyNoteNotFound
	}
	existing.Title = title
	existing.Content = content
	existing.Summary = summary
	existing.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// ListRange returns notes for (userID, kbID) in [from, to]. Both
// bounds are inclusive and capped at DailyNoteMaxRangeDays apart.
func (s *UserDailyNoteService) ListRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time, limit int) ([]*types.UserDailyNote, error) {
	if kbID == "" || userID == "" {
		return nil, ErrUserDailyNoteKBRequired
	}
	from = truncateDayUTC(from)
	to = truncateDayUTC(to)
	if to.Before(from) {
		return nil, ErrUserDailyNoteRangeInvalid
	}
	if d := to.Sub(from); d > time.Duration(DailyNoteMaxRangeDays)*24*time.Hour {
		return nil, ErrUserDailyNoteRangeInvalid
	}
	if limit <= 0 {
		limit = DailyNoteDefaultLimit
	}
	return s.repo.ListRange(ctx, userID, kbID, from, to, limit)
}

// CountRange returns the row count for the same predicate as
// ListRange. Kept on the service (not the handler) so the
// dashboard's "X notes this month" copy doesn't have to re-validate
// the date bounds.
func (s *UserDailyNoteService) CountRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time) (int64, error) {
	if kbID == "" || userID == "" {
		return 0, ErrUserDailyNoteKBRequired
	}
	from = truncateDayUTC(from)
	to = truncateDayUTC(to)
	if to.Before(from) {
		return 0, ErrUserDailyNoteRangeInvalid
	}
	return s.repo.CountRange(ctx, userID, kbID, from, to)
}

// today returns the UTC midnight of "now". Stable across the same
// day so a request landing 5 ms before midnight doesn't roll over
// to the next day.
func (s *UserDailyNoteService) today() time.Time {
	return truncateDayUTC(s.now())
}

func truncateDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// defaultContent seeds a freshly-created note with a 3-block skeleton
// the front-end recognises:
//
//   1. "## 今日焦点" — daily intent
//   2. "## 昨日回顾" — yesterday's carry-over
//   3. "## 关联 KB 摘要" — populated by InlineAI on first view
//
// Plain markdown so the front-end renderer can apply its normal
// wiki styling without a special case.
func (s *UserDailyNoteService) defaultContent(day time.Time) string {
	return fmt.Sprintf(
		"## 今日焦点\n\n- \n\n## 昨日回顾\n\n- \n\n## 关联 KB 摘要\n\n（首次查看时由 InlineAI 自动生成 %s 的相关条目摘要）\n",
		day.Format("2006-01-02"),
	)
}
