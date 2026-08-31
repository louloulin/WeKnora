package types

import "time"

// WikiPageComment is a single comment on a wiki page.
//
// Comments are flat: a parent_comment_id can chain to create a thread,
// but the schema stays simple. Soft-delete preserves audit trail. Resolved
// comments stay visible; resolved_at / resolved_by are nullable so the
// UI can show "Mark resolved" / "Reopen" affordances.
type WikiPageComment struct {
	ID                string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID          uint64     `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID   string     `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	WikiPageID        string     `json:"wiki_page_id" gorm:"type:varchar(36);index"`
	ParentCommentID   *string    `json:"parent_comment_id,omitempty" gorm:"type:varchar(36);index"`
	AuthorID          string     `json:"author_id" gorm:"type:varchar(36);index"`
	Body              string     `json:"body" gorm:"type:text"`
	Mentions          StringArray `json:"mentions" gorm:"type:json"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty" gorm:"index"`
	ResolvedBy        *string    `json:"resolved_by,omitempty" gorm:"type:varchar(36)"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty" gorm:"index"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`

	// Optional join fields populated by ListByPage; never persisted.
	AuthorName    string `json:"author_name,omitempty" gorm:"-"`
	AuthorAvatar  string `json:"author_avatar,omitempty" gorm:"-"`
}

// TableName overrides the default pluralized table so we keep the
// singular noun for readability ("wiki_page_comments" reads better than
// "wiki_page_commentses" that GORM would otherwise emit).
func (WikiPageComment) TableName() string {
	return "wiki_page_comments"
}

// CreateWikiCommentRequest is the POST body for creating a comment.
type CreateWikiCommentRequest struct {
	Body            string      `json:"body" binding:"required,min=1,max=10000"`
	ParentCommentID *string     `json:"parent_comment_id,omitempty"`
	Mentions        StringArray `json:"mentions,omitempty"`
}

// UpdateWikiCommentRequest is the PATCH body for editing a comment.
// Only the author (or KB admin) may edit; the handler enforces this.
type UpdateWikiCommentRequest struct {
	Body string `json:"body" binding:"required,min=1,max=10000"`
}

// ResolveWikiCommentRequest is the PATCH body for the resolve toggle.
// Reopen sets Resolved=false to undo.
type ResolveWikiCommentRequest struct {
	Resolved bool `json:"resolved"`
}
