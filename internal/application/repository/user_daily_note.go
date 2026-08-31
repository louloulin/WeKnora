package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrUserDailyNoteNotFound is returned when GetByUserDate / Update find
// no matching row.
var ErrUserDailyNoteNotFound = errors.New("user daily note not found")

// ErrUserDailyNoteConflict is returned when Create hits the unique
// constraint on (user_id, kb_id, note_date). The service layer
// converts this into "the row already exists" so the handler can
// return the existing note instead of 500-ing.
var ErrUserDailyNoteConflict = errors.New("user daily note already exists for that date")

// userDailyNoteRepository is the GORM-backed implementation of
// interfaces.UserDailyNoteRepository. Dialect-aware: the date
// truncation / comparison is handled at the SQL layer via the
// `note_date` DATE column, so callers can pass any time.Time on the
// day boundary.
type userDailyNoteRepository struct {
	db *gorm.DB
}

// NewUserDailyNoteRepository wires a repository against a GORM DB.
func NewUserDailyNoteRepository(db *gorm.DB) interfaces.UserDailyNoteRepository {
	return &userDailyNoteRepository{db: db}
}

func (r *userDailyNoteRepository) GetByID(ctx context.Context, id string) (*types.UserDailyNote, error) {
	var note types.UserDailyNote
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserDailyNoteNotFound
		}
		return nil, err
	}
	return &note, nil
}

func (r *userDailyNoteRepository) GetByUserDate(ctx context.Context, userID string, kbID string, date time.Time) (*types.UserDailyNote, error) {
	day := truncateDay(date)
	var note types.UserDailyNote
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND knowledge_base_id = ? AND note_date = ?", userID, kbID, day).
		First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserDailyNoteNotFound
		}
		return nil, err
	}
	return &note, nil
}

func (r *userDailyNoteRepository) Create(ctx context.Context, note *types.UserDailyNote) error {
	note.NoteDate = truncateDay(note.NoteDate)
	if err := r.db.WithContext(ctx).Create(note).Error; err != nil {
		// GORM surfaces the unique-constraint violation as a generic
		// error; the service layer treats ErrUserDailyNoteConflict as
		// the canonical signal and ignores everything else.
		if isUniqueViolationForDailyNote(err) {
			return ErrUserDailyNoteConflict
		}
		return err
	}
	return nil
}

func (r *userDailyNoteRepository) Update(ctx context.Context, note *types.UserDailyNote) error {
	// Map update — GORM's struct Updates would skip empty strings and
	// accidentally drop a cleared title, so we set the four mutable
	// columns explicitly.
	res := r.db.WithContext(ctx).
		Model(&types.UserDailyNote{}).
		Where("id = ?", note.ID).
		Updates(map[string]interface{}{
			"title":      note.Title,
			"content":    note.Content,
			"summary":    note.Summary,
			"page_id":    note.PageID,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserDailyNoteNotFound
	}
	return nil
}

func (r *userDailyNoteRepository) SetPageID(ctx context.Context, noteID string, pageID string) error {
	res := r.db.WithContext(ctx).
		Model(&types.UserDailyNote{}).
		Where("id = ?", noteID).
		Updates(map[string]interface{}{
			"page_id":    pageID,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserDailyNoteNotFound
	}
	return nil
}

func (r *userDailyNoteRepository) ListRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time, limit int) ([]*types.UserDailyNote, error) {
	if limit <= 0 || limit > 365 {
		limit = 30
	}
	fromDay := truncateDay(from)
	toDay := truncateDay(to)
	var notes []*types.UserDailyNote
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND knowledge_base_id = ? AND note_date BETWEEN ? AND ?", userID, kbID, fromDay, toDay).
		Order("note_date DESC").
		Limit(limit).
		Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

func (r *userDailyNoteRepository) CountRange(ctx context.Context, userID string, kbID string, from time.Time, to time.Time) (int64, error) {
	fromDay := truncateDay(from)
	toDay := truncateDay(to)
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&types.UserDailyNote{}).
		Where("user_id = ? AND knowledge_base_id = ? AND note_date BETWEEN ? AND ?", userID, kbID, fromDay, toDay).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// truncateDay strips the time-of-day component, anchoring the
// timestamp at UTC midnight. The DATE column stores the value
// timezone-naively, so anchoring at UTC keeps the unique constraint
// deterministic regardless of the server's local TZ.
func truncateDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// isUniqueViolationForDailyNote reports whether err looks like a UNIQUE
// constraint failure. Both sqlite and mysql return distinct messages
// ("UNIQUE constraint failed" / "Duplicate entry") so we delegate to
// the shared isUniqueViolation helper in authz_tuple.go (which handles
// both drivers and is unit-tested in isolation).
func isUniqueViolationForDailyNote(err error) bool {
	return isUniqueViolation(err)
}

// containsStringForDailyNote avoids pulling in strings just for one substring
// check — also used by other repos in this package.
func containsStringForDailyNote(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
