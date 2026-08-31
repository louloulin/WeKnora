package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiCommentRepository is the persistence-layer abstraction for wiki
// page comments. Mirrors the concrete repository.WikiCommentRepository
// in internal/application/repository/wiki_comment.go so future tests
// can drop in a fake without booting a database.
type WikiCommentRepository interface {
	// Create persists a new comment row. The caller is responsible for
	// populating ID, CreatedAt, UpdatedAt, TenantID.
	Create(ctx context.Context, comment *types.WikiPageComment) error

	// GetByID fetches a single (non-deleted) comment by primary key.
	// Repository-sentinel ErrWikiPageCommentNotFound wraps to nil to
	// keep this interface transparent.
	GetByID(ctx context.Context, id string) (*types.WikiPageComment, error)

	// ListByPage returns the visible (non-deleted) thread for a page,
	// oldest first, with limit/offset pagination. Limit is clamped to
	// [1, 500] inside the repository.
	ListByPage(ctx context.Context, pageID string, limit, offset int) ([]*types.WikiPageComment, int64, error)

	// Update rewrites body + mentions + updated_at. The handler is
	// responsible for permission checks before calling.
	Update(ctx context.Context, c *types.WikiPageComment) error

	// SetResolved toggles resolved_at / resolved_by. resolved=false
	// clears both columns so the thread reopens.
	SetResolved(ctx context.Context, id string, resolved bool, resolverID string) error

	// SoftDelete sets deleted_at = NOW() so the row stays in the table
	// for audit / future "show deleted comments" UI.
	SoftDelete(ctx context.Context, id string) error
}

// WikiCommentService is the business-logic facade exposed to the
// handler. Mirrors the concrete service.WikiPageCommentService in
// internal/application/service/wiki_comment.go. The interface is
// provided so handler tests can swap in a fake without spinning up the
// repository.
type WikiCommentService interface {
	// Create validates input, mints ID + timestamps, and persists a
	// new top-level comment or reply.
	Create(ctx context.Context, kbID string, pageID string, authorID string, tenantID uint64, req *types.CreateWikiCommentRequest) (*types.WikiPageComment, error)

	// GetByID fetches a single comment and applies tenant scoping.
	GetByID(ctx context.Context, tenantID uint64, id string) (*types.WikiPageComment, error)

	// ListByPage returns the visible thread + total for a page.
	ListByPage(ctx context.Context, tenantID uint64, pageID string, limit, offset int) ([]*types.WikiPageComment, int64, error)

	// Update applies a body + mentions patch. Only the comment author
	// or a KB admin can edit.
	Update(ctx context.Context, tenantID uint64, commentID string, actorID string, isOwnerOrAdmin bool, req *types.UpdateWikiCommentRequest) (*types.WikiPageComment, error)

	// SetResolved marks the thread resolved / unresolved. Only KB
	// owners / tenant admins may resolve threads authored by others.
	SetResolved(ctx context.Context, tenantID uint64, commentID string, actorID string, isOwnerOrAdmin bool, resolved bool) (*types.WikiPageComment, error)

	// SoftDelete removes a comment. Only the author, KB owner, or
	// tenant admin may delete.
	SoftDelete(ctx context.Context, tenantID uint64, commentID string, actorID string, isOwnerOrAdmin bool) error
}
