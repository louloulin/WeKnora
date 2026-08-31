package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiCommentRepository is the persistence-layer abstraction for wiki
// page comments. The repo owns the cross-dialect SQL; the service layer
// translates errors into sentinels so the handler does not depend on
// GORM details.
type WikiCommentRepository interface {
	// Create inserts a new comment row. The caller is expected to have
	// populated ID + CreatedAt. Returns ErrWikiCommentConflict on a
	// duplicate ID.
	Create(ctx context.Context, comment *types.WikiComment) error

	// GetByID fetches a single comment by ID, returning (nil, nil) when
	// the row is missing — the service translates nil into
	// ErrWikiCommentNotFound.
	GetByID(ctx context.Context, id string) (*types.WikiComment, error)

	// ListByPage returns every comment on a page, sorted by (parent_id
	// ASC, created_at ASC) so parents appear before replies.
	ListByPage(ctx context.Context, kbID string, slug string) ([]types.WikiComment, error)

	// Update applies the body + mentions patch and returns the updated row.
	Update(ctx context.Context, id string, body string, mentionsJSON string) (*types.WikiComment, error)

	// SetResolved toggles the resolve state. resolved=false clears the
	// resolved_at + resolved_by columns.
	SetResolved(ctx context.Context, id string, resolved bool, resolvedBy string) (*types.WikiComment, error)

	// Delete removes the row. ON DELETE CASCADE on parent_id handles the
	// reply tree.
	Delete(ctx context.Context, id string) error

	// CountByPage returns (open_count, resolved_count, reply_count).
	CountByPage(ctx context.Context, kbID string, slug string) (open int, resolved int, replies int, err error)
}

// WikiCommentService is the business-logic facade exposed to the
// handler. It validates inputs, applies tenant scoping, and translates
// repository errors into typed sentinels.
type WikiCommentService interface {
	// Create validates + persists a new top-level comment or reply.
	Create(ctx context.Context, tenantID uint64, kbID string, slug string, authorID string, authorName string, authorAvatar string, req types.WikiCommentCreateRequest) (*types.WikiComment, error)

	// List returns the flattened thread + stats for a single page.
	List(ctx context.Context, tenantID uint64, kbID string, slug string) (*types.WikiCommentListResponse, error)

	// Update applies a body + mentions patch authored by the comment's
	// author. Resolved threads remain editable so users can refine the
	// answer before closing.
	Update(ctx context.Context, tenantID uint64, kbID string, commentID string, authorID string, req types.WikiCommentUpdateRequest) (*types.WikiComment, error)

	// SetResolved marks a comment thread resolved / unresolved. Only
	// KB owners / tenant admins can mark a thread resolved by anyone;
	// regular KB members may only resolve threads they authored.
	SetResolved(ctx context.Context, tenantID uint64, kbID string, commentID string, actorID string, isOwnerOrAdmin bool, resolved bool) (*types.WikiComment, error)

	// Delete removes a comment. Only the comment author, KB owner, or
	// tenant admin may delete.
	Delete(ctx context.Context, tenantID uint64, kbID string, commentID string, actorID string, isOwnerOrAdmin bool) error
}
