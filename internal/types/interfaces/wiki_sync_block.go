package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiSyncBlockRepository persists canonical synced blocks.
//
// The contract is intentionally narrow: the service owns merge semantics
// (version increment, fan-out to refs, audit logging) and the repo just
// stores / loads rows keyed by (tenant, block_id).
type WikiSyncBlockRepository interface {
	// Upsert creates or replaces the canonical block. On update, version
	// auto-increments via app-level assignment.
	Upsert(ctx context.Context, in types.WikiSyncBlockUpsert) (*types.WikiSyncBlock, error)

	// Get returns the canonical block by (tenant, block_id) or nil.
	Get(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlock, error)

	// List returns canonical blocks for a (tenant, kb) pair, ordered by
	// updated_at DESC for the picker UI.
	List(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.WikiSyncBlock, error)

	// Delete removes the canonical block. The service decides whether to
	// also cascade or unlink refs.
	Delete(ctx context.Context, tenantID uint64, blockID string) error

	// Stats returns fan-out reach for the picker UI badge.
	Stats(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlockRefStats, error)
}

// WikiSyncBlockRefRepository tracks embedded references.
//
// Refs are denormalized from the page content's `[[sync:UUID]]` markers;
// on every page save the service rewrites this table to reflect the
// current set of embedded blocks.
type WikiSyncBlockRefRepository interface {
	// Upsert creates or refreshes a ref row keyed by (tenant, block, page, anchor).
	Upsert(ctx context.Context, ref *types.WikiSyncBlockRef) error

	// ListByBlock returns all refs to a canonical block.
	ListByBlock(ctx context.Context, tenantID uint64, blockID string) ([]*types.WikiSyncBlockRef, error)

	// ListByPage returns all synced blocks referenced by a page.
	ListByPage(ctx context.Context, tenantID uint64, pageID string) ([]*types.WikiSyncBlockRef, error)

	// DeleteByPage removes every ref that belongs to a page (called on page delete).
	DeleteByPage(ctx context.Context, tenantID uint64, pageID string) error

	// DeleteByBlock removes every ref that belongs to a block (called on cascade delete).
	DeleteByBlock(ctx context.Context, tenantID uint64, blockID string) error

	// MarkRendered updates content_version + rendered_at for a single ref.
	MarkRendered(ctx context.Context, tenantID uint64, blockID, pageID, anchorSlug string, version int64) error
}
