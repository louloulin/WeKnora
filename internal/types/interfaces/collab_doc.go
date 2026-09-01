// Package interfaces — v0.7.25 collaborative_docs contracts.
//
// The interface seams are deliberately narrow so the application layer owns
// the CRDT semantics (snapshot compaction triggers, GC of stale presence
// rows, vector-clock encoding) and the repo just stores/loads bytes keyed
// by (tenant, doc).
package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// CollabDocRepository persists metadata for collaborative documents. This
// is separate from the snapshot/session repos because document metadata
// updates (rename, archive) are independent of CRDT state writes and have
// different access patterns (title search, by-kb listings).
type CollabDocRepository interface {
	Create(ctx context.Context, d *types.CollaborativeDoc) error
	Get(ctx context.Context, tenantID uint64, id string) (*types.CollaborativeDoc, error)
	Update(ctx context.Context, d *types.CollaborativeDoc) error
	Archive(ctx context.Context, tenantID uint64, id string) error
	Delete(ctx context.Context, tenantID uint64, id string) error
	List(ctx context.Context, tenantID uint64, filter types.ListCollaborativeDocsFilter) ([]*types.CollaborativeDoc, error)
	Count(ctx context.Context, tenantID uint64, filter types.ListCollaborativeDocsFilter) (int64, error)
	FindByShareToken(ctx context.Context, token string) (*types.CollaborativeDoc, error)
}

// CollabDocSnapshotRepository persists Yjs document snapshots keyed by
// (tenant, doc). Mirrors WikiRealtimeSnapshotRepository but keyed on doc_id
// instead of (kb_id, page_id).
type CollabDocSnapshotRepository interface {
	Upsert(ctx context.Context, in types.CollabDocSnapshotUpsert) (*types.CollabDocSnapshot, error)
	Get(ctx context.Context, tenantID uint64, docID string) (*types.CollabDocSnapshot, error)
	Delete(ctx context.Context, tenantID uint64, docID string) error
}

// CollabDocSessionRepository tracks live presence rows.
type CollabDocSessionRepository interface {
	Upsert(ctx context.Context, s *types.CollabDocSession) error
	ListByDoc(ctx context.Context, tenantID uint64, docID string, since time.Time) ([]*types.CollabDocSession, error)
	DeleteByClient(ctx context.Context, tenantID uint64, docID string, clientID uint64) error
	SweepOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// CollabDocFileRepository persists the binary payload of a collab doc. One
// row per save; the latest row is the canonical "open this doc" target.
type CollabDocFileRepository interface {
	// SaveFile writes a new version row. Returns ErrCollabDocInvalid on
	// version collision (existing row at the same (doc_id, version)).
	SaveFile(ctx context.Context, in types.CollabDocFileUpsert) (*types.CollabDocFile, error)
	// GetLatestFile returns the row with the highest version for a doc.
	GetLatestFile(ctx context.Context, tenantID uint64, docID string) (*types.CollabDocFile, error)
	// GetFileByVersion returns a specific version's row (for rollback).
	GetFileByVersion(ctx context.Context, tenantID uint64, docID string, version int) (*types.CollabDocFile, error)
	// CurrentVersion returns the highest version number for a doc, or 0
	// when no file has been uploaded yet.
	CurrentVersion(ctx context.Context, tenantID uint64, docID string) (int, error)
	// DeleteByDoc removes every row for a doc (hard delete).
	DeleteByDoc(ctx context.Context, tenantID uint64, docID string) error
	ListByDoc(ctx context.Context, tenantID uint64, docID string) ([]*types.CollabDocFile, error)
	// PurgeFilesOlderThan removes file rows whose created_at is before the
	// cutoff AND whose version is not the latest per (tenant, doc). Used by
	// the retention scheduler to bound the on-disk footprint of historical
	// .docx/.pptx/.xlsx byte payloads. Returns the number of rows deleted.
	PurgeFilesOlderThan(ctx context.Context, cutoff time.Time, keepLatest int) (int64, error)
}

// CollabDocCommentRepository persists threaded comments for collaborative
// documents. Threads are flattened into rows; ParentID groups replies
// under the thread anchor (the first row of the thread has ParentID=nil).
type CollabDocCommentRepository interface {
	Create(ctx context.Context, c *types.CollabDocComment) error
	Get(ctx context.Context, tenantID uint64, id uint64) (*types.CollabDocComment, error)
	Update(ctx context.Context, tenantID uint64, id uint64, patch types.UpdateCollabDocCommentRequest) (*types.CollabDocComment, error)
	Delete(ctx context.Context, tenantID uint64, id uint64) error
	ListByDoc(ctx context.Context, tenantID uint64, docID string, filter types.ListCollabDocCommentsFilter) ([]*types.CollabDocComment, error)
	DeleteByDoc(ctx context.Context, tenantID uint64, docID string) (int64, error)
}

// CollabDocAuditRepository persists immutable operation history for
// collaborative documents. Rows are never updated; Record appends, List
// reads. Summary aggregates by action and by day for the history panel.
type CollabDocAuditRepository interface {
	// Record writes a new entry. Caller is responsible for filling TenantID,
	// DocID, Action, ActorUserID, and (when available) ActorName/IP/UA.
	Record(ctx context.Context, in types.RecordAuditRequest) (*types.CollabDocAuditEntry, error)
	// Get returns a single entry by id (mostly used for testing/debug).
	Get(ctx context.Context, tenantID uint64, id uint64) (*types.CollabDocAuditEntry, error)
	// List returns paginated entries matching the filter. Defaults
	// (Limit<=0 -> 50, Offset<0 -> 0) are applied at the application
	// layer not the repo, so test fixtures can exercise the no-default
	// path.
	List(ctx context.Context, tenantID uint64, filter types.ListCollabDocAuditFilter) ([]*types.CollabDocAuditEntry, error)
	// Count returns the number of entries matching the filter (used for
	// pagination + summary).
	Count(ctx context.Context, tenantID uint64, filter types.ListCollabDocAuditFilter) (int64, error)
	// Summary returns aggregated counts by action and by day for the
	// history panel.
	Summary(ctx context.Context, tenantID uint64, filter types.ListCollabDocAuditFilter) (*types.CollabDocAuditSummary, error)
	// DeleteByDoc removes every entry for a doc (used on hard delete).
	DeleteByDoc(ctx context.Context, tenantID uint64, docID string) (int64, error)
}

// -----------------------------------------------------------------------------
// v0.7.90 — collab_doc_form_responses repository
// -----------------------------------------------------------------------------

// CollabDocFormResponseRepository persists submitted answers for form
// documents. The public responder page calls Create; the owner-side
// summary/export endpoints call ListByDoc / Count. DeleteByDoc is invoked
// when the owning doc is hard-deleted so we never leak PII across docs.
type CollabDocFormResponseRepository interface {
	Create(ctx context.Context, r *types.CollabDocFormResponse) error
	Get(ctx context.Context, tenantID uint64, id uint64) (*types.CollabDocFormResponse, error)
	ListByDoc(ctx context.Context, tenantID uint64, docID string, filter types.ListCollabDocFormResponsesFilter) ([]*types.CollabDocFormResponse, error)
	CountByDoc(ctx context.Context, tenantID uint64, docID string) (int64, error)
	DeleteByDoc(ctx context.Context, tenantID uint64, docID string) (int64, error)
}
