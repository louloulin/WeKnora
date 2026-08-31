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
)

// ValidCollaborativeDocKinds is the closed set accepted by handlers; an
// unknown kind is rejected at the API edge so a typo never reaches the DB.
var ValidCollaborativeDocKinds = map[CollaborativeDocKind]bool{
	CollaborativeDocKindDoc:   true,
	CollaborativeDocKindSheet: true,
	CollaborativeDocKindSlide: true,
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
	ShareToken    string               `json:"share_token" gorm:"type:varchar(64)"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	ArchivedAt    *time.Time           `json:"archived_at,omitempty"`
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
	DocID       string                `json:"doc_id" gorm:"type:varchar(36);index"`
	Format      CollaborativeDocKind  `json:"format" gorm:"type:varchar(16)"`
	Content     []byte                `json:"-" gorm:"column:content;type:longblob"`
	SizeBytes   int                   `json:"size_bytes"`
	SHA256      string                `json:"sha256" gorm:"type:varchar(64)"`
	Version     int                   `json:"version"`
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
	if u.Version <= 0 {
		return ErrCollabDocInvalid("version must be positive")
	}
	return nil
}

// GetLatestFileFilter narrows the latest-file query. When Format is set, it
// overrides the doc_kind for the lookup (today they're 1:1, but the doc
// engine doesn't enforce that).
type GetLatestFileFilter struct {
	Format CollaborativeDocKind
}
