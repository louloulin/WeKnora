package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newVerificationDB returns an isolated in-memory sqlite with the
// wiki_pages schema (incl. the new Build #48 verification columns)
// migrated, and a wikiPageRepository bound to it. Sharing pattern
// with TestPruneEmptyFolderChainsDeletesOnlyEmptyCandidateAncestors
// keeps the tests close to the real storage path.
func newVerificationDB(t *testing.T) (*gorm.DB, interfaces.WikiPageRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WikiPage{}, &types.WikiPageRevision{}, &types.WikiFolder{}))
	repo := repository.NewWikiPageRepository(db)
	return db, repo
}

// fixedClock returns a *time.Time-aware clock function for tests so we
// can deterministically exercise the ReviewDueAt advance logic.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func seedPage(t *testing.T, repo interfaces.WikiPageRepository, page *types.WikiPage) {
	t.Helper()
	require.NoError(t, repo.Create(context.Background(), page))
}

func TestMarkVerified_SetsFieldsAndAdvancesDue(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	_, repo := newVerificationDB(t)
	past := now.Add(-30 * 24 * time.Hour)
	seedPage(t, repo, &types.WikiPage{
		ID:              "p1",
		TenantID:        1,
		KnowledgeBaseID: "kb",
		Slug:            "x",
		Title:           "X",
		PageType:        types.WikiPageTypeEntity,
		Status:          types.WikiPageStatusPublished,
		ReviewDueAt:     &past,
		CreatedAt:       past,
		UpdatedAt:       past,
	})

	svc := NewWikiVerificationService(repo)
	svc.now = fixedClock(now)

	require.NoError(t, svc.MarkVerified(context.Background(), "p1", "alice"))

	got, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, got.VerifiedAt)
	require.True(t, got.VerifiedAt.Equal(now))
	require.Equal(t, "alice", got.VerifiedBy)
	require.NotNil(t, got.ReviewDueAt)
	require.True(t, got.ReviewDueAt.Equal(now.Add(DefaultReviewInterval)),
		"due should advance by DefaultReviewInterval, got %v", got.ReviewDueAt)
}

func TestMarkVerified_LeavesFarFutureDueAlone(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	_, repo := newVerificationDB(t)
	farFuture := now.Add(365 * 24 * time.Hour)
	seedPage(t, repo, &types.WikiPage{
		ID:              "p1",
		TenantID:        1,
		KnowledgeBaseID: "kb",
		Slug:            "y",
		Title:           "Y",
		PageType:        types.WikiPageTypeEntity,
		Status:          types.WikiPageStatusPublished,
		ReviewDueAt:     &farFuture,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	svc := NewWikiVerificationService(repo)
	svc.now = fixedClock(now)

	require.NoError(t, svc.MarkVerified(context.Background(), "p1", "alice"))
	got, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.True(t, got.ReviewDueAt.Equal(farFuture),
		"manager-set far-future due should not be reset")
}

func TestMarkVerified_RejectsEmptyInputs(t *testing.T) {
	_, repo := newVerificationDB(t)
	svc := NewWikiVerificationService(repo)
	require.ErrorIs(t, svc.MarkVerified(context.Background(), "", "alice"), ErrWikiPageVerificationInvalidInput)
	require.ErrorIs(t, svc.MarkVerified(context.Background(), "p1", ""), ErrWikiPageVerificationInvalidInput)
}

func TestMarkVerified_NotFound(t *testing.T) {
	_, repo := newVerificationDB(t)
	svc := NewWikiVerificationService(repo)
	err := svc.MarkVerified(context.Background(), "missing", "alice")
	require.ErrorIs(t, err, repository.ErrWikiPageNotFound)
}

func TestSetReviewSchedule_WritesOwnerAndDue(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	_, repo := newVerificationDB(t)
	seedPage(t, repo, &types.WikiPage{
		ID:              "p1",
		TenantID:        1,
		KnowledgeBaseID: "kb",
		Slug:            "z",
		Title:           "Z",
		PageType:        types.WikiPageTypeEntity,
		Status:          types.WikiPageStatusPublished,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	svc := NewWikiVerificationService(repo)

	due := now.Add(7 * 24 * time.Hour)
	require.NoError(t, svc.SetReviewSchedule(context.Background(), "p1", "bob", due, "manager-1"))

	got, err := repo.GetByID(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, "bob", got.ReviewOwner)
	require.NotNil(t, got.ReviewDueAt)
	require.True(t, got.ReviewDueAt.Equal(due))
	require.Equal(t, "manager-1", got.VerifiedBy,
		"re-pinning a stale page should stamp VerifiedBy so audit trail is preserved")
}

func TestSetReviewSchedule_RejectsLongOwner(t *testing.T) {
	_, repo := newVerificationDB(t)
	svc := NewWikiVerificationService(repo)
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	err := svc.SetReviewSchedule(context.Background(), "p1", string(long), time.Now(), "")
	require.ErrorIs(t, err, ErrWikiPageVerificationInvalidInput)
}

func TestComputeVerificationStatus_Table(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc := NewWikiVerificationService(nil)
	svc.now = fixedClock(now)
	verified := now.Add(-30 * 24 * time.Hour)
	past := now.Add(-1 * time.Hour)

	cases := []struct {
		name string
		page *types.WikiPage
		want types.VerificationStatus
	}{
		{"nil page → missing", nil, types.VerificationStatusMissing},
		{
			"never verified → warning (first verification pending)",
			&types.WikiPage{UpdatedAt: now},
			types.VerificationStatusWarning,
		},
		{
			"verified but due passed → bad",
			&types.WikiPage{VerifiedAt: &verified, ReviewDueAt: &past, UpdatedAt: verified},
			types.VerificationStatusBad,
		},
		{
			"edited after verification → warning",
			&types.WikiPage{
				VerifiedAt: &verified,
				UpdatedAt:  now.Add(time.Hour),
			},
			types.VerificationStatusWarning,
		},
		{
			"verified, not due, not edited after → ok",
			&types.WikiPage{
				VerifiedAt: &verified,
				UpdatedAt:  verified,
			},
			types.VerificationStatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, svc.ComputeVerificationStatus(tc.page))
		})
	}
}

func TestIsPageStale(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc := NewWikiVerificationService(nil)
	svc.now = fixedClock(now)
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	require.False(t, svc.IsPageStale(nil))
	require.False(t, svc.IsPageStale(&types.WikiPage{}))
	require.False(t, svc.IsPageStale(&types.WikiPage{ReviewDueAt: &future}))
	require.True(t, svc.IsPageStale(&types.WikiPage{ReviewDueAt: &past}))
}
