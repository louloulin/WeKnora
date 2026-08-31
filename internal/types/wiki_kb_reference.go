package types

import (
	"time"

	"gorm.io/gorm"
)

// WikiKBReference binds a Wiki Page to a Knowledge Base document so that
// the doc + KB integrated platform stays coherent in both directions.
//
// The relation is the smallest possible bridge: each row records that the
// wiki page referenced the KB document at least once (the wiki content
// might mention the same KB doc in multiple paragraphs — we keep one row
// per pair, with a fresh reference_label whenever the author re-edits).
//
// Lifecycle rules:
//   - Insertion is upsert by (wiki_page_id, knowledge_id); a second
//     [[kb:id]] mention from the same page only updates reference_label.
//   - Soft-delete on either the wiki page or the KB document does NOT
//     remove the row; the resolver returns a Tombstone status instead so
//     the UI can render a "deleted" badge without losing the link audit.
//   - Hard delete (GDPR purge) on either side cascades via the FK DDL.
//
// All access goes through KnowledgeReferenceService; direct repository
// use from outside the service is reserved for migration scripts.
type WikiKBReference struct {
	ID             uint64 `gorm:"primaryKey"`
	TenantID       string `gorm:"type:varchar(36);not null;index"`
	WikiPageID     string `gorm:"type:varchar(36);not null;uniqueIndex:uq_wiki_kb_reference"`
	KnowledgeID    string `gorm:"type:varchar(36);not null;uniqueIndex:uq_wiki_kb_reference"`
	ReferenceLabel string `gorm:"type:varchar(256);not null;default:''"`
	CreatedBy      string `gorm:"type:varchar(36);not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// TableName fixes the table name for GORM so that the postgres and
// sqlite migrations both address the same physical table.
func (WikiKBReference) TableName() string { return "wiki_kb_references" }

// ReferenceStatus is the resolution status returned by the service.
// It is a wire-level enum so the renderer can pick a badge without
// having to inspect each linked entity itself.
type ReferenceStatus string

const (
	// ReferenceStatusActive means both endpoints still exist.
	ReferenceStatusActive ReferenceStatus = "active"
	// ReferenceStatusWikiDeleted means the wiki page was soft-deleted.
	ReferenceStatusWikiDeleted ReferenceStatus = "wiki_deleted"
	// ReferenceStatusKBDeleted means the KB document was soft-deleted.
	ReferenceStatusKBDeleted ReferenceStatus = "kb_deleted"
	// ReferenceStatusBothDeleted means both endpoints are tombstones.
	ReferenceStatusBothDeleted ReferenceStatus = "both_deleted"
)

// ResolvedWikiKBReference is what the service hands to the renderer.
// It bundles the reference row with the minimal info needed to draw a
// reference card and the resolution status that controls the badge.
type ResolvedWikiKBReference struct {
	WikiKBReference
	Status     ReferenceStatus `json:"status"`
	WikiTitle  string          `json:"wiki_title"`
	KBTitle    string          `json:"kb_title"`
	KBSnippet  string          `json:"kb_snippet"`
	KBFileName string          `json:"kb_file_name"`
}

// WikiKBReferenceListFilter is the input shape for List operations.
// All zero/empty fields are no-filter; the repository ANDs the predicates.
type WikiKBReferenceListFilter struct {
	TenantID          string
	WikiPageID        string
	KnowledgeID       string
	Limit             int
	Offset            int
	IncludeTombstoned bool
}
