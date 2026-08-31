package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// UserDailyNoteRepository is the Build #45.a persistence surface. Kept
// small on purpose so the service layer can wrap each call in a single
// DB transaction if needed; concrete implementation lives in
// internal/application/repository/user_daily_note.go and is dialect-
// aware (sqlite + mysql).
type UserDailyNoteRepository interface {
	// GetByID returns the row by primary key. Used by the
	// UpdateContent path which only has the note id from the URL.
	// Returns ErrUserDailyNoteNotFound if no row exists.
	GetByID(ctx context.Context, id string) (*types.UserDailyNote, error)

	// GetByUserDate returns the row for (userID, kbID, date) or
	// ErrUserDailyNoteNotFound if no row exists yet. The date is
	// truncated to day boundary by the caller.
	GetByUserDate(ctx context.Context, userID string, kbID string, date time.Time) (*types.UserDailyNote, error)

	// Create persists a new row. The repo is responsible for
	// ensuring the unique constraint on (user_id, kb_id, note_date)
	// is honoured — callers should call GetByUserDate first when
	// the caller wants "get or create" semantics, but a Create call
	// on an existing row returns ErrUserDailyNoteConflict so the
	// service can decide how to recover (typically: reload + patch).
	Create(ctx context.Context, note *types.UserDailyNote) error

	// Update rewrites title / content / summary / page_id on an
	// existing row. CreatedAt is preserved; UpdatedAt is set by the
	// GORM layer. Returns ErrUserDailyNoteNotFound if no row exists.
	Update(ctx context.Context, note *types.UserDailyNote) error

	// SetPageID back-fills the lazy wiki_pages link after the first
	// GET that materializes the linked wiki page.
	SetPageID(ctx context.Context, noteID string, pageID string) error

	// ListRange returns rows for (userID, kbID) where note_date is
	// in [from, to], inclusive. Sorted newest-first. Limit caps the
	// page size (default 30, max 365) — callers pass 0 for default.
	ListRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time, limit int) ([]*types.UserDailyNote, error)

	// CountRange returns the total row count for the same predicate
	// (used for the dashboard widget's "X notes this month" copy).
	CountRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time) (int64, error)
}
