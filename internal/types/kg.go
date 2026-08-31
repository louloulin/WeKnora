package types

import (
	"encoding/json"
	"time"
)

// KGSupertag is a typed schema-tag applied to entities (Tana supertag
// parity). Each KGSupertag carries a JSON schema describing the fields it
// owns; the schema is enforced at the application layer when entities
// adopt the tag.
type KGSupertag struct {
	ID            string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID      uint64          `json:"tenant_id" gorm:"index;type:varchar(36)"`
	KBID          string          `json:"kb_id" gorm:"index;type:varchar(36)"`
	Name          string          `json:"name" gorm:"type:varchar(128)"`
	Color         string          `json:"color" gorm:"type:varchar(16)"`
	Schema        json.RawMessage `json:"schema" gorm:"type:jsonb"`
	Icon          string          `json:"icon" gorm:"type:varchar(64)"`
	ChildKGSupertag bool            `json:"child_supertag"`
	AutofillModel string          `json:"autofill_model" gorm:"type:varchar(64)"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// TableName tells GORM to use supertags table.
func (KGSupertag) TableName() string { return "supertags" }

// KGSupertagField is one entry in a KGSupertag.Schema JSON array.
type KGSupertagField struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // text / number / date / link / multi_link
	Required bool   `json:"required"`
}

// KGEntity is a node in the knowledge graph. It may be bound to a KGSupertag
// and carries arbitrary typed fields in Properties.
type KGEntity struct {
	ID            string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID      uint64          `json:"tenant_id" gorm:"index;type:varchar(36)"`
	KBID          string          `json:"kb_id" gorm:"index;type:varchar(36)"`
	SupertagID    *string         `json:"supertag_id,omitempty" gorm:"index;type:varchar(36)"`
	Name          string          `json:"name" gorm:"type:varchar(256);index"`
	Properties    json.RawMessage `json:"properties" gorm:"type:jsonb"`
	Embeddings    []byte          `json:"embeddings,omitempty" gorm:"type:bytea"`
	FirstSeenDoc  *string         `json:"first_seen_doc,omitempty" gorm:"type:varchar(36)"`
	LastSeenDoc   *string         `json:"last_seen_doc,omitempty" gorm:"type:varchar(36)"`
	Occurrence    int             `json:"occurrence"`
	TrustScore    float64         `json:"trust_score"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// TableName tells GORM to use entities table.
func (Entity) TableName() string { return "entities" }

// KGEntityRelation is a directed edge between two Entities. Evidence lists
// the document IDs that supported the relation during extraction.
type KGEntityRelation struct {
	ID           string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	SrcEntityID  string          `json:"src_entity_id" gorm:"index;type:varchar(36)"`
	DstEntityID  string          `json:"dst_entity_id" gorm:"index;type:varchar(36)"`
	Relation     string          `json:"relation" gorm:"type:varchar(64)"`
	Weight       float64         `json:"weight"`
	EvidenceDocs json.RawMessage `json:"evidence_docs" gorm:"type:jsonb"`
	Confidence   float64         `json:"confidence"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TableName tells GORM to use entity_relations table.
func (KGEntityRelation) TableName() string { return "entity_relations" }

// KGSupertagCommand wires a KGSupertag event to a Build #33 Automation. When
// the event fires (e.g. on_add), the Automation is triggered.
type KGSupertagCommand struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	SupertagID  string    `json:"supertag_id" gorm:"index;type:varchar(36)"`
	Event       string    `json:"event" gorm:"type:varchar(32);index"` // on_add / on_remove / on_update
	AutomationID string    `json:"automation_id" gorm:"type:varchar(36)"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName tells GORM to use supertag_commands table.
func (KGSupertagCommand) TableName() string { return "supertag_commands" }

//  KGEntityDraft is the intermediate shape returned by the NER pipeline
// before entities are persisted. Confidence is between 0 and 1.
type  KGEntityDraft struct {
	TmpID     string                 `json:"tmp_id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Span      string                 `json:"span"`
	Confidence float64                `json:"confidence"`
	Properties map[string]any         `json:"properties,omitempty"`
}

// KGRelationDraft is the intermediate shape returned by the RE pipeline.
type KGRelationDraft struct {
	SrcTmpID   string  `json:"src_tmp_id"`
	DstTmpID   string  `json:"dst_tmp_id"`
	Relation   string  `json:"relation"`
	Confidence float64 `json:"confidence"`
}

// KGExtractionResult bundles the NER + RE output for one document.
type KGExtractionResult struct {
	DocumentID string          `json:"document_id"`
	Entities   [] KGEntityDraft   `json:"entities"`
	Relations  []KGRelationDraft `json:"relations"`
}
