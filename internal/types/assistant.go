package types

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// AssistantCitation is one source backing an assistant response.
// Both KB and Wiki citations share this wire shape so the renderer
// can switch on CitationType without changing the rest of the UI.
type AssistantCitation struct {
	// Type is either "kb" or "wiki".
	Type string `json:"type"`
	// ID is the underlying resource id (knowledge_id or page_id).
	ID string `json:"id"`
	// Title is the human-readable label — knowledge title for KB
	// citations, page title for wiki citations.
	Title string `json:"title"`
	// Slug is the wiki slug (empty for KB citations).
	Slug string `json:"slug,omitempty"`
	// KBID is the knowledge base id (KB citations only).
	KBID string `json:"kb_id,omitempty"`
	// Snippet is the short text excerpt that justified the match.
	Snippet string `json:"snippet"`
	// Score is the normalised relevance score in [0, 1]. KB and
	// Wiki searches normalise to this range independently, so
	// scores are comparable across sources within a single Ask.
	Score float64 `json:"score"`
}

// AssistantAskRequest is the wire body for POST /assistant/ask.
//
// SourceKBIDs scopes the KB-side retrieval to specific knowledge
// bases. An empty list means "search all KBs the user has access
// to" — the service resolves the visible KB list from the
// KnowledgeBaseService. IncludeWiki toggles the wiki-side retrieval;
// turning it off is useful when the assistant panel is configured
// to be KB-only.
type AssistantAskRequest struct {
	Query               string   `json:"query" binding:"required"`
	SourceKBIDs         []string `json:"source_kb_ids"`
	IncludeWiki         bool     `json:"include_wiki"`
	MaxResultsPerSource int      `json:"max_results_per_source"`
	ConversationID      string   `json:"conversation_id"`
}

// AssistantAskResponse is the wire body the handler renders. The
// client renders AnswerText as the user-visible answer and the two
// citation slices as clickable footnotes that open the source.
type AssistantAskResponse struct {
	AnswerID       string              `json:"answer_id"`
	ConversationID string              `json:"conversation_id"`
	AnswerText     string              `json:"answer_text"`
	KBCitations    []AssistantCitation `json:"kb_citations"`
	WikiCitations  []AssistantCitation `json:"wiki_citations"`
	ModelName      string              `json:"model_name"`
	LatencyMS      int                 `json:"latency_ms"`
	ResultCount    int                 `json:"result_count"`
	CreatedAt      time.Time           `json:"created_at"`
}

// AssistantConversation is one Q&A turn stored in
// assistant_conversations. KBCitations / WikiCitations are stored
// verbatim so the admin audit view can replay the retrieval exactly
// as it happened without re-running the search.
type AssistantConversation struct {
	ID             uint64              `gorm:"primaryKey"`
	TenantID       string              `gorm:"type:varchar(36);not null;index"`
	UserID         string              `gorm:"type:varchar(36);not null"`
	ConversationID string              `gorm:"type:varchar(36);not null;index"`
	QueryText      string              `gorm:"type:text;not null"`
	KBCitations    []AssistantCitation `gorm:"type:jsonb;serializer:json"`
	WikiCitations  []AssistantCitation `gorm:"type:jsonb;serializer:json"`
	SourceKBIDs    StringArray         `gorm:"type:jsonb;serializer:json"`
	IncludeWiki    bool                `gorm:"not null;default:true"`
	ResultCount    int                 `gorm:"not null;default:0"`
	ModelName      string              `gorm:"type:varchar(64);not null;default:''"`
	LatencyMS      int                 `gorm:"not null;default:0"`
	CreatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (AssistantConversation) TableName() string { return "assistant_conversations" }

// MarshalCitations serialises both citation slices for the audit log.
// Kept on the type so callers do not have to remember which column
// holds what.
func (c *AssistantConversation) MarshalCitations() ([]byte, error) {
	return json.Marshal(struct {
		KBCitations   []AssistantCitation `json:"kb_citations"`
		WikiCitations []AssistantCitation `json:"wiki_citations"`
	}{KBCitations: c.KBCitations, WikiCitations: c.WikiCitations})
}
