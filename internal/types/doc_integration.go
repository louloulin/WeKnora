package types

import (
	"encoding/json"
	"time"
)

// DocKgRelationKind enumerates how a wiki page / doc chunk is tied
// to a Knowledge Graph entity or relation. The kind drives the
// graph UI (visual style + grouping).
type DocKgRelationKind string

const (
	// DocKgMentionsEntity: the doc references the entity in free text.
	DocKgMentionsEntity DocKgRelationKind = "mentions_entity"
	// DocKgDefinesEntity: the doc is the canonical definition of the entity.
	DocKgDefinesEntity DocKgRelationKind = "defines_entity"
	// DocKgAssertsRelation: the doc asserts the relation between two entities.
	DocKgAssertsRelation DocKgRelationKind = "asserts_relation"
	// DocKgCitesKG: the doc cites the KG as a whole.
	DocKgCitesKG DocKgRelationKind = "cites_kg"
)

// DocKgRelation is the bridge record between a document (wiki page
// chunk or KB chunk) and the Knowledge Graph. It is written by the
// build42 DocKgLinker whenever a wiki page is saved.
type DocKgRelation struct {
	ID             uint64           `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID       uint64           `json:"tenant_id" gorm:"index"`
	SourceType     string           `json:"source_type" gorm:"type:varchar(32);index"` // "wiki_page" or "kb_chunk"
	SourceID       string           `json:"source_id" gorm:"type:varchar(64);index"`
	TargetType     string           `json:"target_type" gorm:"type:varchar(32);index"` // "kg_entity" or "kg_relation"
	TargetID       string           `json:"target_id" gorm:"type:varchar(64);index"`
	Kind           DocKgRelationKind `json:"kind" gorm:"type:varchar(32);index"`
	Confidence     float64          `json:"confidence" gorm:"default:1.0"`
	Anchor         string           `json:"anchor" gorm:"type:varchar(255)"` // optional snippet / hash
	CreatedAt      time.Time        `json:"created_at"`
}

// TableName tells GORM to use doc_kg_relations table.
func (DocKgRelation) TableName() string { return "doc_kg_relations" }

// KbWikiReference is the reverse-direction link from a KB chunk back
// to every wiki page that cites it. Written whenever a wiki page
// saves and re-resolves its KB citations; used by the KB answer to
// jump straight to the relevant wiki page in one click.
type KbWikiReference struct {
	ID          uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64    `json:"tenant_id" gorm:"index"`
	KBChunkID   string    `json:"kb_chunk_id" gorm:"type:varchar(64);index"`
	WikiPageID  string    `json:"wiki_page_id" gorm:"type:varchar(64);index"`
	Anchor      string    `json:"anchor" gorm:"type:varchar(255)"`
	CitationCtx string    `json:"citation_ctx" gorm:"type:text"` // a short snippet showing how the wiki page uses the chunk
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName tells GORM to use kb_wiki_references table.
func (KbWikiReference) TableName() string { return "kb_wiki_references" }

// InlineKBRefKind enumerates the shapes an inline KB citation can
// take. The kind drives how the editor renders the chip (text vs
// block vs callout).
type InlineKBRefKind string

const (
	InlineKBRefText     InlineKBRefKind = "text"     // inline text chip
	InlineKBRefBlock    InlineKBRefKind = "block"    // block-level reference panel
	InlineKBRefCallout  InlineKBRefKind = "callout"  // callout box
	InlineKBRefQuote    InlineKBRefKind = "quote"    // quoted block with cite link
)

// InlineKBRef records a wiki page's inline citation of a KB chunk.
// Each chip / block in the editor gets one row so the "open in KB"
// reverse link works on a per-citation basis.
type InlineKBRef struct {
	ID          uint64         `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    uint64         `json:"tenant_id" gorm:"index"`
	WikiPageID  string         `json:"wiki_page_id" gorm:"type:varchar(64);index"`
	KBChunkID   string         `json:"kb_chunk_id" gorm:"type:varchar(64);index"`
	Kind        InlineKBRefKind `json:"kind" gorm:"type:varchar(16);index"`
	Anchor      string         `json:"anchor" gorm:"type:varchar(255)"` // text snippet near the chip
	Position    int            `json:"position"`                        // ordering within the page
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TableName tells GORM to use inline_kb_refs table.
func (InlineKBRef) TableName() string { return "inline_kb_refs" }

// DocAssistantMode enumerates the three modes of the AI Assistant
// Panel (Build #42). They are intentionally separate so the front-end
// can render the right UI per mode while sharing the same backend.
type DocAssistantMode string

const (
	AssistantModeChat    DocAssistantMode = "chat"    // multi-turn Q&A against KB + Wiki + KG
	AssistantModeSearch  DocAssistantMode = "search"  // cross-source unified search
	AssistantModeCreate  DocAssistantMode = "create"  // AI-assisted content creation
)

// DocAssistantRequest is the unified input envelope accepted by the
// AssistantPanel handler. Mode drives which fields are required.
type DocAssistantRequest struct {
	TenantID  uint64          `json:"tenant_id"`
	UserID    string          `json:"user_id"`
	Mode      DocAssistantMode   `json:"mode" binding:"required"`
	Prompt    string          `json:"prompt" binding:"required"`
	ContextIDs []string       `json:"context_ids,omitempty"` // wiki pages, KB chunks, KG entities to pin
	Stream    bool            `json:"stream,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
}

// DocAssistantResponse is the unified output envelope. Citations and
// Created entities/relations are optional depending on mode.
type DocAssistantResponse struct {
	Mode      DocAssistantMode   `json:"mode"`
	Answer    string          `json:"answer"`
	Citations []DocAssistantCitation `json:"citations,omitempty"`
	Created   []DocAssistantCreated  `json:"created,omitempty"`
	Usage     DocAssistantUsage      `json:"usage"`
}

// DocAssistantCitation is a single citation emitted by Chat / Search.
type DocAssistantCitation struct {
	SourceType string  `json:"source_type"` // "wiki_page" | "kb_chunk" | "kg_entity"
	SourceID   string  `json:"source_id"`
	Title      string  `json:"title"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

// DocAssistantCreated is an entity / relation / page created by Create mode.
type DocAssistantCreated struct {
	Kind     string `json:"kind"` // "wiki_page" | "kg_entity" | "kg_relation"
	ID       string `json:"id"`
	Title    string `json:"title"`
	ParentID string `json:"parent_id,omitempty"`
}

// DocAssistantUsage reports the rough token spend so the UI can surface
// "you saved N hours" ROI metrics (Build #34.x follow-up).
type DocAssistantUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
