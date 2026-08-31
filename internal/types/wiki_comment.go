package types

import (
	"time"

	"gorm.io/gorm"
)

// WikiCommentMaxBodyBytes caps the body length at 4 KB. Larger payloads
// are rejected at the request level so a runaway client cannot flood the
// wiki comment table with megabyte blobs.
const WikiCommentMaxBodyBytes = 4096

// WikiComment represents a single wiki page comment. The thread tree is
// materialised by parent_id + replies; the API flattens replies when
// serving the list view so the frontend renders a tree without extra
// calls. Mentions are stored as a JSON array of {user_id, display_name,
// handle?, avatar_url?} so a comment author can mention several users
// inline.
type WikiComment struct {
	ID              string `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64 `json:"tenant_id" gorm:"index"`
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	PageSlug       string `json:"page_slug" gorm:"type:text;index"`
	ParentID       string `json:"parent_id,omitempty" gorm:"type:varchar(36);index"`
	Body           string `json:"body" gorm:"type:text"`
	Mentions       string `json:"mentions" gorm:"type:text"` // JSON-encoded array
	AnchorBlockID  string `json:"anchor_block_id,omitempty" gorm:"type:varchar(64)"`
	AuthorID       string `json:"author_id" gorm:"type:varchar(64);index"`
	AuthorName     string `json:"author_name" gorm:"type:text"`
	AuthorAvatarURL string `json:"author_avatar_url,omitempty" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy     string `json:"resolved_by,omitempty" gorm:"type:varchar(64)"`
}

// TableName returns the underlying GORM table name.
func (WikiComment) TableName() string { return "wiki_page_comments" }

// BeforeCreate ensures the timestamps are populated before insert.
func (c *WikiComment) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate refreshes UpdatedAt on every save.
func (c *WikiComment) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// WikiCommentMention is the embedded mention payload. The JSON shape is
// shared with the frontend client (api/wiki/comments.ts).
type WikiCommentMention struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Handle      string `json:"handle,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// WikiCommentCreateRequest is the body accepted by POST /comments.
type WikiCommentCreateRequest struct {
	Body          string                `json:"body" binding:"required"`
	ParentID      string                `json:"parent_id,omitempty"`
	AnchorBlockID string                `json:"anchor_block_id,omitempty"`
	Mentions      []WikiCommentMention  `json:"mentions,omitempty"`
}

// WikiCommentUpdateRequest is the body accepted by PUT /comments/:id.
// Edits may rewrite the body and mentions; parent/anchor/resolve are
// updated through dedicated endpoints so an edit can never accidentally
// change a comment's threading.
type WikiCommentUpdateRequest struct {
	Body     string                `json:"body" binding:"required"`
	Mentions []WikiCommentMention  `json:"mentions,omitempty"`
}

// WikiCommentResolveRequest is the body accepted by POST /comments/:id/resolve.
type WikiCommentResolveRequest struct {
	Resolved bool `json:"resolved"`
}

// WikiCommentListResponse is the API response for the list endpoint.
// `comments` is the flattened thread (parents + replies interleaved by
// created_at), and `stats` summarises open / resolved counts.
type WikiCommentListResponse struct {
	Comments []WikiComment        `json:"comments"`
	Stats    WikiCommentListStats `json:"stats"`
}

// WikiCommentListStats is the meta panel the front-end renders above the
// thread (e.g. "12 open · 4 resolved").
type WikiCommentListStats struct {
	TotalOpen     int `json:"total_open"`
	TotalResolved int `json:"total_resolved"`
	TotalReplies  int `json:"total_replies"`
}
