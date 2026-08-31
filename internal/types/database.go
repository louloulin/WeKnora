package types

import (
	"encoding/json"
	"time"
)

// DatabaseFieldType enumerates the column types a Database supports.
// Adding a new type is a code-only change; the data column is JSONB.
type DatabaseFieldType string

const (
	DatabaseFieldText        DatabaseFieldType = "text"
	DatabaseFieldNumber      DatabaseFieldType = "number"
	DatabaseFieldSelect      DatabaseFieldType = "select"
	DatabaseFieldMultiSelect DatabaseFieldType = "multi_select"
	DatabaseFieldDate        DatabaseFieldType = "date"
	DatabaseFieldPerson      DatabaseFieldType = "person"
	DatabaseFieldCheckbox    DatabaseFieldType = "checkbox"
	DatabaseFieldURL         DatabaseFieldType = "url"
	DatabaseFieldEmail       DatabaseFieldType = "email"
	DatabaseFieldPhone       DatabaseFieldType = "phone"
	DatabaseFieldFormula     DatabaseFieldType = "formula"
	DatabaseFieldRelation    DatabaseFieldType = "relation"
	DatabaseFieldRollup      DatabaseFieldType = "rollup"
)

// AllDatabaseFieldTypes lists every registered field type. Used by
// the frontend field picker.
var AllDatabaseFieldTypes = []DatabaseFieldType{
	DatabaseFieldText,
	DatabaseFieldNumber,
	DatabaseFieldSelect,
	DatabaseFieldMultiSelect,
	DatabaseFieldDate,
	DatabaseFieldPerson,
	DatabaseFieldCheckbox,
	DatabaseFieldURL,
	DatabaseFieldEmail,
	DatabaseFieldPhone,
	DatabaseFieldFormula,
	DatabaseFieldRelation,
	DatabaseFieldRollup,
}

// DatabaseViewType enumerates the six view shapes a Database can render.
// New view types are added by extending this enum and adding a renderer
// in frontend/src/components/database/views/.
type DatabaseViewType string

const (
	DatabaseViewTable    DatabaseViewType = "table"
	DatabaseViewBoard    DatabaseViewType = "board"
	DatabaseViewGallery  DatabaseViewType = "gallery"
	DatabaseViewCalendar DatabaseViewType = "calendar"
	DatabaseViewTimeline DatabaseViewType = "timeline"
	DatabaseViewList     DatabaseViewType = "list"
)

// AllDatabaseViewTypes lists every registered view type. Used by
// the view switcher.
var AllDatabaseViewTypes = []DatabaseViewType{
	DatabaseViewTable,
	DatabaseViewBoard,
	DatabaseViewGallery,
	DatabaseViewCalendar,
	DatabaseViewTimeline,
	DatabaseViewList,
}

// Database is the top-level container. Belongs to one knowledge base.
// Deletion is soft (deleted_at) so audit + rollback can recover rows.
type Database struct {
	ID              string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64    `json:"tenant_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	Name            string    `json:"name" gorm:"type:varchar(255)"`
	Description     string    `json:"description" gorm:"type:text"`
	Icon            string    `json:"icon" gorm:"type:varchar(64)"`
	CreatedBy       string    `json:"created_by" gorm:"type:varchar(64)"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

func (Database) TableName() string { return "databases" }

// DatabaseField is one typed column. Options hold type-specific config:
//   - select/multi_select: {"choices": [{"id":"...","name":"...","color":"..."}]}
//   - number: {"precision":2}
//   - formula: {"expression":"price * quantity"}
//   - relation: {"database_id":"...","database_field_id":"..."}
//   - rollup:  {"relation_field_id":"...","target_field_id":"...","fn":"sum|avg|count|min|max"}
type DatabaseField struct {
	ID         string            `json:"id" gorm:"type:varchar(36);primaryKey"`
	DatabaseID string            `json:"database_id" gorm:"type:varchar(36);index"`
	Name       string            `json:"name" gorm:"type:varchar(255)"`
	Type       DatabaseFieldType `json:"type" gorm:"type:varchar(32)"`
	Options    JSON              `json:"options" gorm:"type:json"`
	Width      int               `json:"width"`
	SortOrder  int               `json:"sort_order"`
	IsPrimary  bool              `json:"is_primary"`
	CreatedAt  time.Time         `json:"created_at"`
}

func (DatabaseField) TableName() string { return "database_fields" }

// DatabaseRow is one entry in a Database. Values are stored as JSONB keyed
// by field_id so we can evolve the field schema without rewriting rows.
type DatabaseRow struct {
	ID         string          `json:"id" gorm:"type:varchar(36);primaryKey"`
	DatabaseID string          `json:"database_id" gorm:"type:varchar(36);index"`
	Data       json.RawMessage `json:"data" gorm:"type:json"`
	SortOrder  int             `json:"sort_order"`
	CreatedBy  string          `json:"created_by" gorm:"type:varchar(64)"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	DeletedAt  *time.Time      `json:"deleted_at,omitempty"`
}

func (DatabaseRow) TableName() string { return "database_rows" }

// DatabaseView persists per-user (or shared) view configuration. The
// shape of Config is governed by Type; see viewConfigFor.
type DatabaseView struct {
	ID         string           `json:"id" gorm:"type:varchar(36);primaryKey"`
	DatabaseID string           `json:"database_id" gorm:"type:varchar(36);index"`
	Type       DatabaseViewType `json:"type" gorm:"type:varchar(32)"`
	Name       string           `json:"name" gorm:"type:varchar(255)"`
	Config     JSON             `json:"config" gorm:"type:json"`
	SortOrder  int              `json:"sort_order"`
	IsDefault  bool             `json:"is_default"`
	CreatedBy  string           `json:"created_by" gorm:"type:varchar(64)"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

func (DatabaseView) TableName() string { return "database_views" }

// ViewFilter is a single {field_id, op, value} predicate used to narrow
// rows in a view. Op is one of: equals / not_equals / contains /
// greater_than / less_than / is_empty / is_not_empty / in (multi).
type ViewFilter struct {
	FieldID string `json:"field_id"`
	Op      string `json:"op"`
	Value   any    `json:"value,omitempty"`
}

// ViewSort is a single {field_id, direction} ordering directive.
type ViewSort struct {
	FieldID   string `json:"field_id"`
	Direction string `json:"direction"` // asc | desc
}

// ViewGroup is a single {field_id} grouping directive (board / list).
type ViewGroup struct {
	FieldID string `json:"field_id"`
}

// ViewConfig is the JSON shape stored in database_views.config. Each
// view type consumes only the subset it cares about; the renderer is
// responsible for ignoring foreign keys.
type ViewConfig struct {
	Filters            []ViewFilter `json:"filters,omitempty"`
	Sorts              []ViewSort   `json:"sorts,omitempty"`
	Groups             []ViewGroup  `json:"groups,omitempty"`
	HiddenFields       []string     `json:"hidden_fields,omitempty"`
	BoardGroupFieldID  string       `json:"board_group_field_id,omitempty"`
	CalendarDateFieldID string      `json:"calendar_date_field_id,omitempty"`
	TimelineStartFieldID string     `json:"timeline_start_field_id,omitempty"`
	TimelineEndFieldID   string     `json:"timeline_end_field_id,omitempty"`
}
