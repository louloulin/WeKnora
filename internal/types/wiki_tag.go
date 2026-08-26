package types

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// WikiTagPalette enumerates the 8-color hard-coded palette the frontend
// exposes. The service validates user-supplied color values against this
// set; values outside the palette are rejected at the request level.
// Centralizing the constant here guarantees the API contract and the
// validation logic agree on what counts as a valid color.
var WikiTagPalette = []string{
	"blue", "green", "orange", "red",
	"purple", "teal", "gray", "gold",
}

// IsValidWikiTagColor reports whether c is one of the 8 palette entries.
// Used by Create / Update handlers and by the service layer to refuse
// arbitrary color strings that would survive persistence but confuse the
// frontend renderer.
func IsValidWikiTagColor(c string) bool {
	for _, p := range WikiTagPalette {
		if p == c {
			return true
		}
	}
	return false
}

// WikiTag is a user-defined label that can be attached to any wiki page
// within the same KB. Mirrors internal/types/tag.go:KnowledgeTag, except
// the KB identifier is the wiki's UUID (KnowledgeTag uses the older
// knowledge_base_id shape).
type WikiTag struct {
	// Unique identifier of the tag (UUID).
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// Workspace ID — used for tenant scoping at the service layer.
	TenantID uint64 `json:"tenant_id"`
	// Knowledge base ID this tag belongs to.
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	// Display name. Unique within the KB.
	Name string `json:"name" gorm:"type:varchar(64);not null"`
	// One of WikiTagPalette.
	Color string `json:"color" gorm:"type:varchar(16);not null;default:'blue'"`
	// Display order inside the WikiTagPanel.
	SortOrder int `json:"sort_order" gorm:"type:int;default:0"`
	// Standard GORM timestamps.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName overrides GORM's default pluralization so the table name
// stays singular ("wiki_tags", not "wiki_tages").
func (WikiTag) TableName() string { return "wiki_tags" }

// BeforeCreate stamps the standard GORM v2 lifecycle hook so the
// CreatedAt / UpdatedAt fields are populated automatically. Matches
// the convention used by KnowledgeTag in this package.
func (t *WikiTag) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	return nil
}

func (t *WikiTag) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedAt = time.Now()
	return nil
}

// WikiTagWithCount is the List response shape. PageCount is the number
// of pages currently associated with this tag, computed via a single
// LEFT JOIN + GROUP BY in the repository.
type WikiTagWithCount struct {
	WikiTag
	PageCount int64 `json:"page_count"`
}

// WikiPageTag is the join-table row between a tag and a page. The pair
// (wiki_tag_id, wiki_page_id) is the primary key; duplicates are
// rejected by the schema.
type WikiPageTag struct {
	WikiTagID  string    `json:"wiki_tag_id"  gorm:"type:varchar(36);primaryKey"`
	WikiPageID string    `json:"wiki_page_id" gorm:"type:varchar(36);primaryKey"`
	CreatedAt  time.Time `json:"created_at"`
}

func (WikiPageTag) TableName() string { return "wiki_page_tags" }

// WikiTagCreateRequest is the POST /wiki/tags body.
type WikiTagCreateRequest struct {
	Name  string `json:"name"  binding:"required,min=1,max=64"`
	Color string `json:"color"`
}

// WikiTagUpdateRequest is the PUT /wiki/tags/:id body. All fields are
// optional; nil pointers mean "leave unchanged".
type WikiTagUpdateRequest struct {
	Name      *string `json:"name,omitempty"      binding:"omitempty,min=1,max=64"`
	Color     *string `json:"color,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
}

// WikiTagSetPageRequest is the PUT /wiki/pages/:slug/tags body.
// The service replaces the existing associations atomically.
type WikiTagSetPageRequest struct {
	TagIDs []string `json:"tag_ids" binding:"required,max=10,dive,uuid"`
}

// WikiBatchTagBody is the POST /wiki/pages/batch-tag body.
// Op is "add" (insert, ignore duplicates) or "remove" (delete,
// silently no-op when the page doesn't carry the tag).
type WikiBatchTagBody struct {
	Slugs []string `json:"slugs"   binding:"required,min=1,max=200,dive,required"`
	TagID string   `json:"tag_id"  binding:"required,uuid"`
	Op    string   `json:"op"      binding:"required,oneof=add remove"`
}

// WikiBatchTagOpAdd / WikiBatchTagOpRemove are the two valid values for
// WikiBatchTagBody.Op. Exposed as constants so callers don't sprinkle
// string literals.
const (
	WikiBatchTagOpAdd    = "add"
	WikiBatchTagOpRemove = "remove"
)

// WikiTagMaxPerPage is the per-page cap enforced by SetPageTags. Values
// above this are rejected at the request level with code tag_limit_exceeded.
// Mirrors spec.md section 1.5 D4.
const WikiTagMaxPerPage = 10

// WikiTagNameMaxLength caps user-supplied tag names. The DB schema also
// enforces 64 chars via VARCHAR(64). Centralizing here keeps the request
// validator and the service-layer guard in lockstep.
const WikiTagNameMaxLength = 64

// Sentinel errors for WikiTagService. The HTTP handler maps each to a
// stable status code (see internal/handler/wiki_tag.go). Wrapped where
// extra context is needed; callers should errors.Is on these.
var (
	// ErrWikiTagNotFound — tag does not exist in the requested KB (or
	// lives in a different KB; the handler returns 404 for both to
	// avoid leaking existence across KBs).
	ErrWikiTagNotFound = errors.New("wiki tag not found")
	// ErrWikiTagConflict — another tag with the same (kb, name) already
	// exists. Handler returns 409.
	ErrWikiTagConflict = errors.New("wiki tag name conflict")
	// ErrWikiTagLimitExceeded — SetPageTags was asked to attach more
	// than WikiTagMaxPerPage tags to a single page. Handler returns 400.
	ErrWikiTagLimitExceeded = errors.New("wiki tag per-page limit exceeded")
	// ErrWikiTagInvalidName — name is empty, whitespace-only, or longer
	// than WikiTagNameMaxLength after trim. Handler returns 400.
	ErrWikiTagInvalidName = errors.New("wiki tag name invalid")
	// ErrWikiTagInvalidColor — color is not in WikiTagPalette. Handler
	// returns 400.
	ErrWikiTagInvalidColor = errors.New("wiki tag color invalid")
)

// IsWikiTagNotFound reports whether err originated from a not-found
// path on the tag service (mirrors IsWikiBatchKBMismatch).
func IsWikiTagNotFound(err error) bool {
	return errors.Is(err, ErrWikiTagNotFound)
}

// IsWikiTagConflict reports whether err is the name-conflict sentinel.
func IsWikiTagConflict(err error) bool {
	return errors.Is(err, ErrWikiTagConflict)
}

// IsWikiTagLimitExceeded reports whether err is the per-page-limit sentinel.
func IsWikiTagLimitExceeded(err error) bool {
	return errors.Is(err, ErrWikiTagLimitExceeded)
}