// Package types — Build #45.a Daily Note (P0 gap #12 from v25 capability
// analysis).
//
// A UserDailyNote is one row per (user, kb, calendar_date). The handler
// layer keeps the date dimension server-side (UTC, truncated to the day)
// so client clocks don't accidentally split a note across two rows.
//
// The note itself is a stub: page_id points at a real wiki_pages row
// created lazily on first GET. This keeps the daily-note write path
// fast (no per-day WikiPage creation cost) while still letting the
// Verified Knowledge Engine treat daily notes as reviewable surfaces.
package types

import "time"

// DailyNoteDefaultTitle is the title prefix used for newly-created
// daily notes. Kept server-side so a Chinese-locale client doesn't
// accidentally create "Daily Note 2026-09-01" while an English-locale
// teammate creates "每日笔记 2026-09-01" — the date is the only
// canonical handle, the title is display-only.
const DailyNoteDefaultTitlePrefix = "每日笔记"

// UserDailyNote is the per-(user, kb, date) row. One note per day per
// KB per user; verified-knowledge fields ride on the related wiki_page
// (this row keeps the lightweight date-keyed index).
type UserDailyNote struct {
	// ID is a UUID v4. Stable across edits so the front-end can use it
	// as the PATCH handle.
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// TenantID scopes the note to the active workspace so a user
	// switching workspaces does not see another workspace's notes.
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// UserID is the owner. Derived from the auth context, never from
	// the request body — clients cannot write notes into another
	// user's account.
	UserID string `json:"user_id" gorm:"type:varchar(64);index"`
	// KnowledgeBaseID scopes the note to a single KB. Different KBs
	// in the same tenant get different daily notes (an enterprise
	// team might want one note per client KB).
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	// NoteDate is the UTC calendar date (no time-of-day). Used as part
	// of the unique constraint; the front-end never sees this raw
	// since the handler round-trips through time.Time.UTC() / .Truncate.
	NoteDate time.Time `json:"note_date" gorm:"type:date;index"`
	// Slug is the URL-friendly handle within the KB, e.g. "daily/2026-09-01".
	Slug string `json:"slug" gorm:"type:varchar(255)"`
	// PageID points at the linked wiki_pages row. NULL on the day the
	// note is created; populated lazily on first GET so writes don't
	// have to materialize a full wiki page.
	PageID string `json:"page_id,omitempty" gorm:"type:varchar(36);index"`
	// Title is the display title; defaults to "每日笔记 YYYY-MM-DD".
	Title string `json:"title" gorm:"type:varchar(255)"`
	// Content is the markdown body. Small enough to fit in TEXT.
	// Front-end patches are full-body replacements, not deltas.
	Content string `json:"content" gorm:"type:text"`
	// Summary is the AI-generated one-line summary, populated by
	// InlineAIService on first view. Empty until then.
	Summary string `json:"summary" gorm:"type:varchar(512)"`
	// CreatedAt / UpdatedAt are GORM-managed.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName pins the table name so GORM's pluraliser can't drift.
func (UserDailyNote) TableName() string { return "user_daily_notes" }

// DailyNoteListRequest is the query body for ListRange. Date range is
// inclusive on both ends.
type DailyNoteListRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	From            string `json:"from"` // YYYY-MM-DD
	To              string `json:"to"`   // YYYY-MM-DD
	Limit           int    `json:"limit,omitempty"`
}

// DailyNoteListResponse is the paginated range response. Pages are
// returned newest-first so the dashboard widget can render the most
// recent edit without a separate sort.
type DailyNoteListResponse struct {
	Notes []*UserDailyNote `json:"notes"`
	Total int64            `json:"total"`
}
