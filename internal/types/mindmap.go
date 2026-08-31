// Package types — Build #43 MindMap data model.
//
// A MindMap is a tree-of-nodes graph used as the "rich expression" layer
// inside the collaborative document editor. The model supports 5 layouts
// (tree / free / fishbone / timeline / radial), 6 node types (text / image /
// link / doc-ref / task / formula), bidirectional links to Wiki / KB Docs,
// and an export surface (PNG / SVG / Markdown / OPML / .xmind).
//
// Storage layer: GORM-friendly struct + JSON-friendly struct in one file.
// Migration: 000048_mindmaps.sql (sqlite + postgres + mysql).
package types

import (
	"errors"
	"time"
)

// MindMapLayout enumerates the supported layouts.
type MindMapLayout string

const (
	MindMapLayoutTree     MindMapLayout = "tree"
	MindMapLayoutFree     MindMapLayout = "free"
	MindMapLayoutFishbone MindMapLayout = "fishbone"
	MindMapLayoutTimeline MindMapLayout = "timeline"
	MindMapLayoutRadial   MindMapLayout = "radial"
)

// ValidMindMapLayouts is the closed set enforced at the API edge.
var ValidMindMapLayouts = map[MindMapLayout]bool{
	MindMapLayoutTree:     true,
	MindMapLayoutFree:     true,
	MindMapLayoutFishbone: true,
	MindMapLayoutTimeline: true,
	MindMapLayoutRadial:   true,
}

// MindMapNodeType enumerates the supported node types.
type MindMapNodeType string

const (
	MindMapNodeText    MindMapNodeType = "text"
	MindMapNodeImage   MindMapNodeType = "image"
	MindMapNodeLink    MindMapNodeType = "link"
	MindMapNodeDocRef  MindMapNodeType = "doc_ref"
	MindMapNodeTask    MindMapNodeType = "task"
	MindMapNodeFormula MindMapNodeType = "formula"
)

// ValidMindMapNodeTypes is the closed set enforced at the API edge.
var ValidMindMapNodeTypes = map[MindMapNodeType]bool{
	MindMapNodeText:    true,
	MindMapNodeImage:   true,
	MindMapNodeLink:    true,
	MindMapNodeDocRef:  true,
	MindMapNodeTask:    true,
	MindMapNodeFormula: true,
}

// MindMap is the top-level container. Each KB / Doc can have many mind maps;
// the owning user sets visibility (private / team / kb-wide).
type MindMap struct {
	ID          string        `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID    uint64        `json:"tenant_id" gorm:"index"`
	Title       string        `json:"title" gorm:"type:varchar(255)"`
	Layout      MindMapLayout `json:"layout" gorm:"type:varchar(16);default:'tree'"`
	Theme       string        `json:"theme" gorm:"type:varchar(32);default:'feishu'"`
	RootNodeID  string        `json:"root_node_id" gorm:"type:varchar(36)"`
	KBID        string        `json:"kb_id" gorm:"type:varchar(36);index"`
	OwnerUserID uint64        `json:"owner_user_id"`
	Visibility  string        `json:"visibility" gorm:"type:varchar(16);default:'private'"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// TableName returns the GORM table name.
func (MindMap) TableName() string { return "mindmaps" }

// Validate enforces non-empty invariants the repo relies on.
func (m MindMap) Validate() error {
	if m.TenantID == 0 {
		return ErrMindMapInvalid("tenant_id is required")
	}
	if m.Title == "" {
		return ErrMindMapInvalid("title is required")
	}
	if m.Layout != "" && !ValidMindMapLayouts[m.Layout] {
		return ErrMindMapInvalid("layout is invalid")
	}
	if m.RootNodeID == "" {
		return ErrMindMapInvalid("root_node_id is required")
	}
	if m.OwnerUserID == 0 {
		return ErrMindMapInvalid("owner_user_id is required")
	}
	return nil
}

// MindMapNode is a single node in the graph. ParentID is nil for the root.
// The X / Y / Width / Height fields are absolute canvas coordinates in pixels.
// DocRef / KBRef / TaskRef are optional cross-resource links.
type MindMapNode struct {
	ID        string          `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID  uint64          `json:"tenant_id" gorm:"index"`
	MapID     string          `json:"map_id" gorm:"type:varchar(36);index"`
	ParentID  *string         `json:"parent_id,omitempty" gorm:"type:varchar(36);index"`
	NodeType  MindMapNodeType `json:"node_type" gorm:"type:varchar(16);default:'text'"`
	Label     string          `json:"label" gorm:"type:varchar(512)"`
	Body      string          `json:"body" gorm:"type:text"`
	X         float64         `json:"x"`
	Y         float64         `json:"y"`
	Width     float64         `json:"width" gorm:"default:160"`
	Height    float64         `json:"height" gorm:"default:48"`
	Color     string          `json:"color" gorm:"type:varchar(16)"`
	Icon      string          `json:"icon" gorm:"type:varchar(64)"`
	DocRef    *string         `json:"doc_ref,omitempty" gorm:"type:varchar(36);index"`
	KBRef     *string         `json:"kb_ref,omitempty" gorm:"type:varchar(36);index"`
	TaskRef   *uint64         `json:"task_ref,omitempty"`
	Formula   string          `json:"formula,omitempty" gorm:"type:text"`
	OrderHint int             `json:"order_hint" gorm:"default:0"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// TableName returns the GORM table name.
func (MindMapNode) TableName() string { return "mindmap_nodes" }

// Validate enforces non-empty invariants.
func (n MindMapNode) Validate() error {
	if n.TenantID == 0 {
		return ErrMindMapInvalid("tenant_id is required")
	}
	if n.MapID == "" {
		return ErrMindMapInvalid("map_id is required")
	}
	if n.Label == "" {
		return ErrMindMapInvalid("label is required")
	}
	if !ValidMindMapNodeTypes[n.NodeType] {
		return ErrMindMapInvalid("node_type is invalid")
	}
	if n.Width <= 0 {
		n.Width = 160
	}
	if n.Height <= 0 {
		n.Height = 48
	}
	return nil
}

// CreateMindMapRequest is the body for POST /mindmaps.
type CreateMindMapRequest struct {
	Title       string        `json:"title" binding:"required"`
	Layout      MindMapLayout `json:"layout"`
	Theme       string        `json:"theme"`
	KBID        string        `json:"kb_id"`
	Visibility  string        `json:"visibility"`
	RootLabel   string        `json:"root_label"`
	RootBody    string        `json:"root_body"`
	RootColor   string        `json:"root_color"`
	RootIcon    string        `json:"root_icon"`
}

// UpdateMindMapRequest is the body for PATCH /mindmaps/:id.
type UpdateMindMapRequest struct {
	Title      *string        `json:"title,omitempty"`
	Layout     *MindMapLayout `json:"layout,omitempty"`
	Theme      *string        `json:"theme,omitempty"`
	Visibility *string        `json:"visibility,omitempty"`
	RootNodeID *string        `json:"root_node_id,omitempty"`
}

// CreateMindMapNodeRequest is the body for POST /mindmaps/:id/nodes.
type CreateMindMapNodeRequest struct {
	ParentID  *string         `json:"parent_id,omitempty"`
	NodeType  MindMapNodeType `json:"node_type" binding:"required"`
	Label     string          `json:"label" binding:"required"`
	Body      string          `json:"body"`
	X         float64         `json:"x"`
	Y         float64         `json:"y"`
	Width     float64         `json:"width"`
	Height    float64         `json:"height"`
	Color     string          `json:"color"`
	Icon      string          `json:"icon"`
	DocRef    *string         `json:"doc_ref,omitempty"`
	KBRef     *string         `json:"kb_ref,omitempty"`
	TaskRef   *uint64         `json:"task_ref,omitempty"`
	Formula   string          `json:"formula"`
	OrderHint int             `json:"order_hint"`
}

// UpdateMindMapNodeRequest is the body for PATCH /mindmaps/:id/nodes/:nodeID.
type UpdateMindMapNodeRequest struct {
	ParentID  *string         `json:"parent_id,omitempty"`
	NodeType  *MindMapNodeType `json:"node_type,omitempty"`
	Label     *string         `json:"label,omitempty"`
	Body      *string         `json:"body,omitempty"`
	X         *float64        `json:"x,omitempty"`
	Y         *float64        `json:"y,omitempty"`
	Width     *float64        `json:"width,omitempty"`
	Height    *float64        `json:"height,omitempty"`
	Color     *string         `json:"color,omitempty"`
	Icon      *string         `json:"icon,omitempty"`
	DocRef    *string         `json:"doc_ref,omitempty"`
	KBRef     *string         `json:"kb_ref,omitempty"`
	TaskRef   *uint64         `json:"task_ref,omitempty"`
	Formula   *string         `json:"formula,omitempty"`
	OrderHint *int            `json:"order_hint,omitempty"`
}

// ListMindMapsFilter narrows map queries.
type ListMindMapsFilter struct {
	KBID        string
	OwnerUserID uint64
	Visibility  string
	Limit       int
	Offset      int
}

// AutoLayoutRequest is the body for POST /mindmaps/:id/auto-layout.
type AutoLayoutRequest struct {
	Layout MindMapLayout `json:"layout" binding:"required"`
	// Spacing controls the gap between siblings (in pixels).
	Spacing int `json:"spacing"`
}

// ExportFormat enumerates export targets.
type ExportFormat string

const (
	ExportFormatPNG    ExportFormat = "png"
	ExportFormatSVG    ExportFormat = "svg"
	ExportFormatMD     ExportFormat = "markdown"
	ExportFormatOPML   ExportFormat = "opml"
	ExportFormatXMIND  ExportFormat = "xmind"
)

// ValidExportFormats is the closed set enforced at the API edge.
var ValidExportFormats = map[ExportFormat]bool{
	ExportFormatPNG:   true,
	ExportFormatSVG:   true,
	ExportFormatMD:    true,
	ExportFormatOPML:  true,
	ExportFormatXMIND: true,
}

// ErrMindMapInvalid is a typed error for validation failures.
type ErrMindMapInvalid string

func (e ErrMindMapInvalid) Error() string { return string(e) }

// IsMindMapInvalid reports whether err is a validation failure.
func IsMindMapInvalid(err error) bool {
	var t ErrMindMapInvalid
	return errors.As(err, &t)
}
