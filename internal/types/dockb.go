package types

import (
	"encoding/json"
	"time"
)

// DocKBSummary is the AI-generated summary of a single knowledge chunk
// that bridges the document side (raw chunks) with the AI knowledge
// side (semantic queries). One row per (knowledge, chunk) pair; the
// pair is unique per tenant so re-running the summariser is
// idempotent.
type DocKBSummary struct {
	ID          uint64    `json:"id"`
	TenantID    string    `json:"tenant_id" gorm:"index"`
	KnowledgeID string    `json:"knowledge_id" gorm:"index"`
	ChunkID     string    `json:"chunk_id" gorm:"index"`
	Summary     string    `json:"summary" gorm:"type:text"`
	Keyphrases  []string  `json:"keyphrases" gorm:"type:text"`
	AutoTags    []string  `json:"auto_tags" gorm:"type:text"`
	ModelName   string    `json:"model_name" gorm:"type:varchar(64)"`
	Confidence  float32   `json:"confidence"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName tells GORM to use the doc_kb_summaries table.
func (DocKBSummary) TableName() string { return "doc_kb_summaries" }

// ParseKeyphrases deserialises the stored JSON column.
func (s *DocKBSummary) ParseKeyphrases() ([]string, error) {
	if len(s.Keyphrases) == 0 {
		return []string{}, nil
	}
	// Keyphrases might be []string or []any depending on driver; both
	// round-trip through json.Marshal/Unmarshal safely.
	return s.Keyphrases, nil
}

// StringListField is a small helper that round-trips []string through
// JSON. Used by both repo and handler layers so we keep the format
// consistent (always `[]string` on the wire).
func StringListField(raw []string) string {
	b, _ := json.Marshal(raw)
	if len(b) == 0 {
		return "[]"
	}
	return string(b)
}

// --- Database / 多维表 ---

// DatabaseFieldType enumerates the supported column types. v0.7.23
// ships the five most-requested types; richer types land in v0.7.24
// alongside the formula engine.
type DatabaseFieldType string

const (
	DBFieldText     DatabaseFieldType = "text"
	DBFieldNumber   DatabaseFieldType = "number"
	DBFieldSelect   DatabaseFieldType = "select"
	DBFieldCheckbox DatabaseFieldType = "checkbox"
	DBFieldDate     DatabaseFieldType = "date"
)

// DatabaseField describes one column in a wk_database.
type DatabaseField struct {
	Name    string            `json:"name"`
	Type    DatabaseFieldType `json:"type"`
	Options []string          `json:"options,omitempty"` // for select
	Width   int               `json:"width,omitempty"`   // hint for UI
	Required bool             `json:"required,omitempty"`
}

// WKDatabase is a tenant-scoped schema + row container.
type WKDatabase struct {
	ID          uint64          `json:"id"`
	TenantID    string          `json:"tenant_id" gorm:"index"`
	Name        string          `json:"name" gorm:"type:varchar(255)"`
	Description string          `json:"description" gorm:"type:text"`
	Schema      []DatabaseField `json:"schema" gorm:"type:text"`
	CreatedBy   string          `json:"created_by" gorm:"index"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// TableName tells GORM to use the wk_databases table.
func (WKDatabase) TableName() string { return "wk_databases" }

// WKDatabaseRow is one record in a WKDatabase.
type WKDatabaseRow struct {
	ID         uint64            `json:"id"`
	TenantID   string            `json:"tenant_id" gorm:"index"`
	DatabaseID uint64            `json:"database_id" gorm:"index"`
	Values     map[string]any    `json:"values" gorm:"type:text"`
	CreatedBy  string            `json:"created_by" gorm:"index"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// TableName tells GORM to use the wk_database_rows table.
func (WKDatabaseRow) TableName() string { return "wk_database_rows" }
