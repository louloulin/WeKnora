package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newDailyNoteDB returns an isolated in-memory sqlite + the Build #45.a
// repo bound to it. Pattern mirrors TestPruneEmptyFolderChains so the
// test surface stays close to the real storage path.
func newDailyNoteDB(t *testing.T) (*gorm.DB, interfaces.UserDailyNoteRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.UserDailyNote{}))
	repo := repository.NewUserDailyNoteRepository(db)
	return db, repo
}

// fixedClock pins "now" so we can deterministically exercise the
// (now → note_date) anchor logic.
func fixedClockDaily(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestGetOrCreateToday_FirstCallCreates(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	svc.now = fixedClockDaily(now)

	note, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)
	require.NotEmpty(t, note.ID)
	require.Equal(t, "alice", note.UserID)
	require.Equal(t, "kb-1", note.KnowledgeBaseID)
	require.Equal(t, uint64(1), note.TenantID)
	require.Equal(t, "daily/2026-09-01", note.Slug)
	require.Contains(t, note.Title, "2026-09-01")
	require.Contains(t, note.Content, "今日焦点")
	require.Contains(t, note.Content, "昨日回顾")
	require.Contains(t, note.Content, "关联 KB 摘要")
	require.True(t, note.NoteDate.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		"note_date must be UTC midnight, got %v", note.NoteDate)
}

func TestGetOrCreateToday_SecondCallReturnsExisting(t *testing.T) {
	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	svc.now = fixedClockDaily(now)

	first, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)

	// advance the clock by 5 hours within the same UTC day
	svc.now = fixedClockDaily(now.Add(5 * time.Hour))
	second, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "same-day second call must return the same note")

	// advance to next UTC day → new row
	svc.now = fixedClockDaily(now.Add(25 * time.Hour))
	third, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, third.ID, "next-day call must create a new note")
}

func TestGetOrCreateToday_DifferentKBDifferentNote(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	svc.now = fixedClockDaily(now)

	a, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)
	b, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-2")
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
	require.NotEqual(t, a.Slug, b.Slug)
}

func TestGetOrCreateToday_RejectsEmptyKB(t *testing.T) {
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	_, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "")
	require.ErrorIs(t, err, ErrUserDailyNoteKBRequired)
}

func TestGetOrCreateToday_RejectsEmptyUser(t *testing.T) {
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	_, err := svc.GetOrCreateToday(context.Background(), 1, "", "kb-1")
	require.ErrorIs(t, err, ErrUserDailyNoteKBRequired)
}

func TestUpdateContent_OnlyOwner(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	svc.now = fixedClockDaily(now)

	note, err := svc.GetOrCreateToday(context.Background(), 1, "alice", "kb-1")
	require.NoError(t, err)

	// Owner can update
	updated, err := svc.UpdateContent(context.Background(), "alice", note.ID, "New title", "New body", "summary")
	require.NoError(t, err)
	require.Equal(t, "New title", updated.Title)
	require.Equal(t, "New body", updated.Content)
	require.Equal(t, "summary", updated.Summary)

	// Another user can't
	_, err = svc.UpdateContent(context.Background(), "mallory", note.ID, "hijack", "hijack", "")
	require.ErrorIs(t, err, ErrUserDailyNoteNotFound, "non-owner must get NotFound, not Forbidden, to avoid leaking ownership")
}

func TestListRange_OrderingAndRange(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)

	// Create notes across 5 days.
	for d := 0; d < 5; d++ {
		day := now.AddDate(0, 0, -d)
		svc.now = fixedClockDaily(day)
		_, err := svc.GetOrCreateDate(context.Background(), 1, "alice", "kb-1", day)
		require.NoError(t, err)
	}

	from := now.AddDate(0, 0, -10)
	to := now
	notes, err := svc.ListRange(context.Background(), "alice", "kb-1", from, to, 0)
	require.NoError(t, err)
	require.Len(t, notes, 5)
	// Newest-first
	require.True(t, notes[0].NoteDate.After(notes[4].NoteDate),
		"ListRange must return newest-first, got %v", notes)
}

func TestListRange_RejectsInvertedRange(t *testing.T) {
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	now := time.Now()
	_, err := svc.ListRange(context.Background(), "alice", "kb-1", now, now.AddDate(0, 0, -1), 0)
	require.ErrorIs(t, err, ErrUserDailyNoteRangeInvalid)
}

func TestListRange_RejectsOverYear(t *testing.T) {
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	now := time.Now()
	from := now.AddDate(-2, 0, 0) // 2 years
	_, err := svc.ListRange(context.Background(), "alice", "kb-1", from, now, 0)
	require.ErrorIs(t, err, ErrUserDailyNoteRangeInvalid)
}

func TestCountRange(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)
	_, repo := newDailyNoteDB(t)
	svc := NewUserDailyNoteService(repo)
	for d := 0; d < 3; d++ {
		day := now.AddDate(0, 0, -d)
		svc.now = fixedClockDaily(day)
		_, err := svc.GetOrCreateDate(context.Background(), 1, "alice", "kb-1", day)
		require.NoError(t, err)
	}
	n, err := svc.CountRange(context.Background(), "alice", "kb-1",
		now.AddDate(0, 0, -10), now)
	require.NoError(t, err)
	require.Equal(t, int64(3), n)
}

func TestRepository_UniqueConstraintConflict(t *testing.T) {
	db, _ := newDailyNoteDB(t)
	repo := repository.NewUserDailyNoteRepository(db)
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	first := &types.UserDailyNote{
		ID: uuid.NewString(), TenantID: 1, UserID: "alice", KnowledgeBaseID: "kb-1",
		NoteDate: day, Slug: "daily/x", Title: "T", Content: "C",
	}
	require.NoError(t, repo.Create(context.Background(), first))

	second := &types.UserDailyNote{
		ID: uuid.NewString(), TenantID: 1, UserID: "alice", KnowledgeBaseID: "kb-1",
		NoteDate: day, Slug: "daily/x", Title: "T", Content: "C",
	}
	err := repo.Create(context.Background(), second)
	require.ErrorIs(t, err, repository.ErrUserDailyNoteConflict)
}
