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
}
