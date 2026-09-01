// Package types — v0.7.25 collaborative_docs
//
// CollaborativeDoc is the multi-format editing surface that mirrors the
// Feishu / Tencent document UX. It lives next to the Wiki (markdown-rich-text)
// surface but is type-tagged (doc_kind) so the same Yjs WebSocket fan-out
// can carry TipTap (doc), Univer (sheet) and pptxgenjs (slide) updates.
//
// Design note: we deliberately do not overload wiki_doc_snapshots. The Wiki
// surface has its own share / history / template model; the collab doc
// surface uses a separate set of tables (collaborative_docs,
// collab_doc_snapshots, collab_doc_sessions) so the two surfaces can evolve
// independently. The wire protocol (y-websocket binary frames) is shared
// because both surfaces drive the same Yjs CRDT.
package types

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// CollaborativeDocKind is the doc_kind enum that drives which client
// renderer is mounted and which export path the KB ingestion uses.
type CollaborativeDocKind string

const (
	// CollaborativeDocKindDoc is the rich-text document equivalent to
	// Feishu doc / Tencent doc / Notion page. Editor: TipTap + Yjs.
	CollaborativeDocKindDoc CollaborativeDocKind = "doc"
	// CollaborativeDocKindSheet is the spreadsheet equivalent to Tencent
	// doc sheet / Feishu sheet. Editor: Univer Sheets (Yjs-native).
	CollaborativeDocKindSheet CollaborativeDocKind = "sheet"
	// CollaborativeDocKindSlide is the presentation equivalent to Feishu
	// PPT / Tencent slide. Editor: pptxgenjs (server-rendered pptx blob,
	// previewed client-side via vue-office-pptx). v0.7.25 ships a
	// read+offline-export MVP; full inline collaborative editing is
	// tracked as a follow-up.
	CollaborativeDocKindSlide CollaborativeDocKind = "slide"
	// CollaborativeDocKindForm is the form-builder equivalent to Tencent
	// Docs 收集表 / Feishu Base forms. Editor: CollabFormEditor.vue
	// (Y.Array<Question> + simple text/choice/rating widgets). Storage:
	// plain JSON in the existing Yjs doc; bytes import/export round-trips
	// through .form.json. Responses are kept alongside the doc under
	// collab_doc_responses (added in a follow-up if/when response
	// collection is wired).
	CollaborativeDocKindForm CollaborativeDocKind = "form"
)

// ValidCollaborativeDocKinds is the closed set accepted by handlers; an
// unknown kind is rejected at the API edge so a typo never reaches the DB.
var ValidCollaborativeDocKinds = map[CollaborativeDocKind]bool{
	CollaborativeDocKindDoc:   true,
	CollaborativeDocKindSheet: true,
	CollaborativeDocKindSlide: true,
	CollaborativeDocKindForm:  true,
}

// ErrInvalidCollabDocKind is returned when the request carries a doc_kind
// outside the closed set.
var ErrInvalidCollabDocKind = errors.New("invalid collaborative doc_kind")

// ParseCollaborativeDocKind returns the canonical kind or an error.
func ParseCollaborativeDocKind(raw string) (CollaborativeDocKind, error) {
	k := CollaborativeDocKind(strings.ToLower(strings.TrimSpace(raw)))
	if !ValidCollaborativeDocKinds[k] {
		return "", ErrInvalidCollabDocKind
	}
	return k, nil
}

// TableName returns the GORM table name.
func (CollaborativeDoc) TableName() string { return "collaborative_docs" }

// CollaborativeDoc is the metadata row for one multi-format editing
// surface document.
type CollaborativeDoc struct {
	ID            string               `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID      uint64               `json:"tenant_id" gorm:"index"`
	KBID          string               `json:"kb_id" gorm:"type:varchar(36);index"`
	Title         string               `json:"title" gorm:"type:varchar(256)"`
	DocKind       CollaborativeDocKind `json:"doc_kind" gorm:"type:varchar(16)"`
	SchemaVersion int                  `json:"schema_version"`
	OwnerUserID   uint64               `json:"owner_user_id"`
	Visibility    string               `json:"visibility" gorm:"type:varchar(16)"`
	ShareToken         string     `json:"share_token" gorm:"type:varchar(64)"`
	SharePasswordHash string     `json:"-" gorm:"type:varchar(128);default:''"`
	ShareExpiresAt    *time.Time `json:"share_expires_at,omitempty" gorm:""`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
}

// CollabDocSnapshotUpsert is the shape used by the snapshot service when
// saving a freshly compacted Yjs document. The service computes size_bytes
// and the new vector clock before calling the repo.
type CollabDocSnapshotUpsert struct {
	TenantID      uint64
	DocID         string
	DocKind       CollaborativeDocKind
	SchemaVersion int
	YDocState     []byte
	VectorClock   []byte
	SizeBytes     int
}

// CollabDocSnapshot is the durable Yjs state for one collab document.
type CollabDocSnapshot struct {
	ID            uint64               `json:"id"`
	TenantID      uint64               `json:"tenant_id" gorm:"index"`
	DocID         string               `json:"doc_id" gorm:"type:varchar(36);index"`
	DocKind       CollaborativeDocKind `json:"doc_kind" gorm:"type:varchar(16)"`
	SchemaVersion int                  `json:"schema_version"`
	YDocState     []byte               `json:"-" gorm:"column:ydoc_state;type:bytea"`
	VectorClock   []byte               `json:"-" gorm:"column:vector_clock;type:bytea"`
	Version       int64                `json:"version"`
	SizeBytes     int                  `json:"size_bytes"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// MarshalJSON encodes the binary fields as base64 so JSON callers can
// round-trip without extra plumbing. The shape mirrors WikiRealtimeSnapshot.
func (s CollabDocSnapshot) MarshalJSON() ([]byte, error) {
	type alias CollabDocSnapshot
	ydoc := base64.StdEncoding.EncodeToString(s.YDocState)
	vc := base64.StdEncoding.EncodeToString(s.VectorClock)
	return json.Marshal(struct {
		alias
		YDocStateB64 string `json:"ydoc_state"`
		VectorClockB64 string `json:"vector_clock"`
	}{
		alias:          alias(s),
		YDocStateB64:   ydoc,
		VectorClockB64: vc,
	})
}

// TableName returns the GORM table name.
func (CollabDocSnapshot) TableName() string { return "collab_doc_snapshots" }

// CollabDocSession is one live presence row.
type CollabDocSession struct {
	ID            string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID      uint64    `json:"tenant_id" gorm:"index"`
	DocID         string    `json:"doc_id" gorm:"type:varchar(36);index"`
	UserID        uint64    `json:"user_id" gorm:"index"`
	ClientID      uint64    `json:"client_id"`
	Color         string    `json:"color" gorm:"type:varchar(16)"`
	DisplayName   string    `json:"display_name" gorm:"type:varchar(128)"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	JoinedAt      time.Time `json:"joined_at"`
}

// TableName returns the GORM table name.
func (CollabDocSession) TableName() string { return "collab_doc_sessions" }

// ErrCollabDocInvalid is the typed sentinel for surface-level validation
// failures (similar to ErrWikiRealtimeInvalid).
type ErrCollabDocInvalid string

func (e ErrCollabDocInvalid) Error() string { return string(e) }

// Validate enforces the non-empty invariants the snapshot repo relies on.
func (u CollabDocSnapshotUpsert) Validate() error {
	if u.TenantID == 0 {
		return ErrCollabDocInvalid("tenant_id is required")
	}
	if u.DocID == "" {
		return ErrCollabDocInvalid("doc_id is required")
	}
	if !ValidCollaborativeDocKinds[u.DocKind] {
		return ErrCollabDocInvalid("doc_kind is invalid")
	}
	if len(u.YDocState) == 0 {
		return ErrCollabDocInvalid("ydoc_state is empty")
	}
	if u.SizeBytes <= 0 {
		return ErrCollabDocInvalid("size_bytes must be positive")
	}
	return nil
}

// CreateCollaborativeDocRequest is the body for POST /collaborative-docs.
type CreateCollaborativeDocRequest struct {
	KBID    string               `json:"kb_id" binding:"required"`
	Title   string               `json:"title" binding:"required"`
	DocKind CollaborativeDocKind `json:"doc_kind"`
}

// UpdateCollaborativeDocRequest is the body for PATCH /collaborative-docs/:id.
type UpdateCollaborativeDocRequest struct {
	Title      *string `json:"title,omitempty"`
	Visibility *string `json:"visibility,omitempty"`
}

// ListCollaborativeDocsFilter narrows list queries.
type ListCollaborativeDocsFilter struct {
	KBID     string
	DocKind  CollaborativeDocKind
	Archived bool
	Limit    int
	Offset   int
}

// CollabDocFile is one persisted binary payload of a collab doc. Each save
// of the underlying Office file (.docx/.pptx/.xlsx) creates a new row with
// version+1 so history can be inspected and previous bytes can be
// downloaded for rollback.
type CollabDocFile struct {
	ID          uint64                `json:"id"`
	TenantID    uint64                `json:"tenant_id" gorm:"index"`
	DocID       string                `json:"doc_id" gorm:"type:varchar(36);uniqueIndex:uk_collab_doc_file_doc_version,priority:1"`
	Format      CollaborativeDocKind  `json:"format" gorm:"type:varchar(16)"`
	Content     []byte                `json:"-" gorm:"column:content;type:longblob"`
	SizeBytes   int                   `json:"size_bytes"`
	SHA256      string                `json:"sha256" gorm:"type:varchar(64)"`
	Version     int                   `json:"version" gorm:"uniqueIndex:uk_collab_doc_file_doc_version,priority:2"`
	CreatedAt   time.Time             `json:"created_at"`
}

// TableName returns the GORM table name.
func (CollabDocFile) TableName() string { return "collab_doc_files" }

// MarshalJSON encodes content as base64 so JSON callers can round-trip
// without extra plumbing. Mirrors the snapshot MarshalJSON strategy.
func (f CollabDocFile) MarshalJSON() ([]byte, error) {
	type alias CollabDocFile
	content := base64.StdEncoding.EncodeToString(f.Content)
	return json.Marshal(struct {
		alias
		ContentB64 string `json:"content"`
	}{
		alias:     alias(f),
		ContentB64: content,
	})
}

// CollabDocFileUpsert is the input for SaveFile: tenant, doc, format, raw
// bytes and the caller-computed version (current_version+1).
type CollabDocFileUpsert struct {
	TenantID  uint64
	DocID     string
	Format    CollaborativeDocKind
	Content   []byte
	Version   int
}

// Validate enforces non-empty invariants for SaveFile.
func (u CollabDocFileUpsert) Validate() error {
	if u.TenantID == 0 {
		return ErrCollabDocInvalid("tenant_id is required")
	}
	if u.DocID == "" {
		return ErrCollabDocInvalid("doc_id is required")
	}
	if !ValidCollaborativeDocKinds[u.Format] {
		return ErrCollabDocInvalid("format is invalid")
	}
	if len(u.Content) == 0 {
		return ErrCollabDocInvalid("content is empty")
	}
	if u.Version < 0 {
		return ErrCollabDocInvalid("version must be non-negative (0 = auto)")
	}
	return nil
}

// GetLatestFileFilter narrows the latest-file query. When Format is set, it
// overrides the doc_kind for the lookup (today they're 1:1, but the doc
// engine doesn't enforce that).
type GetLatestFileFilter struct {
	Format CollaborativeDocKind
}

// CommentAnchorType is the surface kind the comment is anchored to.
type CommentAnchorType string

const (
	CommentAnchorDoc   CommentAnchorType = "doc"   // paragraph-range inside a DOC
	CommentAnchorSlide CommentAnchorType = "slide" // shape / region inside a PPT slide
	CommentAnchorSheet CommentAnchorType = "sheet" // cell or range inside a SHEET
)

// ValidCommentAnchorTypes is the closed set enforced at the API edge.
var ValidCommentAnchorTypes = map[CommentAnchorType]bool{
	CommentAnchorDoc:   true,
	CommentAnchorSlide: true,
	CommentAnchorSheet: true,
}

// CollabDocComment is a single message in a comment thread. Threads are
// flattened into the table; `parent_id` points at the message being
// replied to (NULL for top-level thread anchors). `anchor_ref` carries a
// JSON blob the editor knows how to render (paragraph range, shape id,
// A1:B10 cell range, …).
type CollabDocComment struct {
	ID            uint64           `json:"id" gorm:"primaryKey"`
	TenantID      uint64           `json:"tenant_id" gorm:"index"`
	DocID         string           `json:"doc_id" gorm:"type:varchar(36);index"`
	ThreadID      string           `json:"thread_id" gorm:"type:varchar(36);index"`
	ParentID      *uint64          `json:"parent_id,omitempty"`
	AuthorUserID  uint64           `json:"author_user_id"`
	AuthorName    string           `json:"author_name" gorm:"type:varchar(128)"`
	AuthorColor   string           `json:"author_color" gorm:"type:varchar(16)"`
	AnchorType    CommentAnchorType `json:"anchor_type" gorm:"type:varchar(16)"`
	AnchorRef     string           `json:"anchor_ref" gorm:"column:anchor_ref;type:text"`
	Body          string           `json:"body" gorm:"type:text"`
	Resolved      bool             `json:"resolved" gorm:"column:resolved"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// TableName returns the GORM table name.
func (CollabDocComment) TableName() string { return "collab_doc_comments" }

// Validate enforces non-empty invariants the comment repo relies on.
func (c CollabDocComment) Validate() error {
	if c.TenantID == 0 {
		return ErrCollabDocInvalid("tenant_id is required")
	}
	if c.DocID == "" {
		return ErrCollabDocInvalid("doc_id is required")
	}
	if c.ThreadID == "" {
		return ErrCollabDocInvalid("thread_id is required")
	}
	if !ValidCommentAnchorTypes[c.AnchorType] {
		return ErrCollabDocInvalid("anchor_type is invalid")
	}
	if c.Body == "" {
		return ErrCollabDocInvalid("body is empty")
	}
	return nil
}

// CreateCollabDocCommentRequest is the body for POST /collaborative-docs/:id/comments.
type CreateCollabDocCommentRequest struct {
	ThreadID   string            `json:"thread_id"`
	ParentID   *uint64           `json:"parent_id,omitempty"`
	AnchorType CommentAnchorType `json:"anchor_type" binding:"required"`
	AnchorRef  string            `json:"anchor_ref"`
	Body       string            `json:"body" binding:"required"`
}

// UpdateCollabDocCommentRequest is the body for PATCH .../comments/:commentID.
type UpdateCollabDocCommentRequest struct {
	Body     *string `json:"body,omitempty"`
	Resolved *bool   `json:"resolved,omitempty"`
}

// ListCollabDocCommentsFilter narrows comment queries.
type ListCollabDocCommentsFilter struct {
	ThreadID  string
	Resolved  *bool
	Limit     int
	Offset    int
}

// ----------------------------------------------------------------------------
// v0.7.30 — collab_doc_audit_log (immutable operation history)
// ----------------------------------------------------------------------------

// CollabDocAuditAction is the closed enum of operations recorded to the audit log.
// Adding a new action requires a corresponding Writer call in the service
// layer (search for `RecordAudit` in
// internal/application/service/collaborative_doc.go).
type CollabDocAuditAction string

const (
	AuditActionCreate       CollabDocAuditAction = "create"        // doc created
	AuditActionRename       CollabDocAuditAction = "rename"        // doc.title updated
	AuditActionUpload       CollabDocAuditAction = "upload"        // initial .docx/.pptx/.xlsx upload
	AuditActionSave         CollabDocAuditAction = "save"          // session snapshot saved (.docx/.pptx/.xlsx bytes)
	AuditActionShareEnable  CollabDocAuditAction = "share.enable"  // share_token turned on
	AuditActionShareDisable CollabDocAuditAction = "share.disable" // share_token turned off
	AuditActionShareAccess  CollabDocAuditAction = "share.access"  // anonymous reader fetched share URL
	AuditActionArchive      CollabDocAuditAction = "archive"       // soft-delete
	AuditActionRestore      CollabDocAuditAction = "restore"       // undelete
	AuditActionDelete       CollabDocAuditAction = "delete"        // hard-delete
	AuditActionCommentAdd   CollabDocAuditAction = "comment.add"   // comment posted
	AuditActionCommentReply CollabDocAuditAction = "comment.reply" // comment reply posted
	AuditActionCommentSolve CollabDocAuditAction = "comment.solve" // thread resolved/unresolved
	AuditActionCommentDel   CollabDocAuditAction = "comment.delete"
	AuditActionPolish       CollabDocAuditAction = "polish"        // AI 段落润色
	AuditActionSyncToKB     CollabDocAuditAction = "sync_to_kb"    // KB ingestion pipeline
	AuditActionExport       CollabDocAuditAction = "export"        // markdown / pdf export
)

// ValidCollabDocAuditActions is the closed set enforced at the application layer.
var ValidCollabDocAuditActions = map[CollabDocAuditAction]bool{
	AuditActionCreate:       true,
	AuditActionRename:       true,
	AuditActionUpload:       true,
	AuditActionSave:         true,
	AuditActionShareEnable:  true,
	AuditActionShareDisable: true,
	AuditActionShareAccess:  true,
	AuditActionArchive:      true,
	AuditActionRestore:      true,
	AuditActionDelete:       true,
	AuditActionCommentAdd:   true,
	AuditActionCommentReply: true,
	AuditActionCommentSolve: true,
	AuditActionCommentDel:   true,
	AuditActionPolish:       true,
	AuditActionSyncToKB:     true,
	AuditActionExport:       true,
}

// CollabDocAuditEntry is one row of the immutable operation history. Rows
// are never updated — every meaningful user action produces a new entry so
// the timeline is tamper-evident.
type CollabDocAuditEntry struct {
	ID           uint64     `json:"id" gorm:"primaryKey"`
	TenantID     uint64     `json:"tenant_id" gorm:"index"`
	DocID        string     `json:"doc_id" gorm:"type:varchar(36);index"`
	ActorUserID  uint64     `json:"actor_user_id"`
	ActorName    string     `json:"actor_name" gorm:"type:varchar(128)"`
	ActorColor   string     `json:"actor_color" gorm:"type:varchar(16)"`
	Action       CollabDocAuditAction `json:"action" gorm:"type:varchar(32)"`
	Target       string     `json:"target" gorm:"type:varchar(64)"`
	Payload      string     `json:"payload" gorm:"type:text"`
	IP           string     `json:"ip" gorm:"type:varchar(64)"`
	UserAgent    string     `json:"user_agent" gorm:"type:varchar(256)"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TableName returns the GORM table name.
func (CollabDocAuditEntry) TableName() string { return "collab_doc_audit_log" }

// Validate enforces non-empty invariants the audit repo relies on. DocID
// is a sentinel here (Use "*" for tenant-scoped events not tied to a
// single doc, e.g. a tenant-wide export). The application layer
// normalizes empty DocID to "*" before calling Validate.
func (e CollabDocAuditEntry) Validate() error {
	if e.TenantID == 0 {
		return ErrCollabDocInvalid("tenant_id is required")
	}
	if !ValidCollabDocAuditActions[e.Action] {
		return ErrCollabDocInvalid("action is invalid")
	}
	if len(string(e.Action)) == 0 {
		return ErrCollabDocInvalid("action is empty")
	}
	return nil
}

// RecordAuditRequest is the body used by service callers (e.g. middleware
// wrapping a handler, or explicit calls inside the service). All string
// fields default to ""; the application layer fills in IP / User-Agent
// from gin.Context when available.
type RecordAuditRequest struct {
	TenantID    uint64               `json:"tenant_id"`
	DocID       string               `json:"doc_id"`
	ActorUserID uint64               `json:"actor_user_id"`
	ActorName   string               `json:"actor_name"`
	ActorColor  string               `json:"actor_color"`
	Action      CollabDocAuditAction `json:"action" binding:"required"`
	Target      string               `json:"target"`
	Payload     string               `json:"payload"`
	IP          string               `json:"ip"`
	UserAgent   string               `json:"user_agent"`
}

// ListCollabDocAuditFilter narrows audit queries for the history panel.
// All fields are optional; an empty Filter lists every entry for the
// tenant (paginated by Limit/Offset).
type ListCollabDocAuditFilter struct {
	DocID       string
	ActorUserID uint64
	Action      CollabDocAuditAction
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int
}

// CollabDocAuditSummary is the rolled-up view returned to the frontend
// history panel ("12 saves, 3 comments this week" etc.).
type CollabDocAuditSummary struct {
	TotalEntries uint64                   `json:"total_entries"`
	ByAction     map[CollabDocAuditAction]uint64   `json:"by_action"`
	ByDay        []CollabDocAuditDayCount `json:"by_day"`
}

// CollabDocAuditDayCount is one row in the daily roll-up. Days with no
// entries are skipped (not zero-filled) to keep payloads small.
type CollabDocAuditDayCount struct {
	Day    string `json:"day"`    // YYYY-MM-DD
	Count  uint64 `json:"count"`
}
