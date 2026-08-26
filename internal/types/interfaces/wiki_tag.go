package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiTagRepository is the persistence-layer abstraction. The service
// translates repository errors into ErrWikiTagNotFound /
// ErrWikiTagConflict so the handler does not depend on GORM details.
type WikiTagRepository interface {
	// List returns every tag in the KB, sorted by (sort_order ASC, name ASC).
	List(ctx context.Context, kbID string) ([]types.WikiTag, error)

	// ListWithCount returns every tag plus its current usage count via a
	// single LEFT JOIN + GROUP BY. Use this when the panel needs to show
	// "X uses" badges.
	ListWithCount(ctx context.Context, kbID string) ([]types.WikiTagWithCount, error)

	// GetByID returns a single tag scoped to the KB. Returns (nil, nil)
	// if the tag does not exist in this KB — the service translates nil
	// into ErrWikiTagNotFound for the handler.
	GetByID(ctx context.Context, kbID string, tagID string) (*types.WikiTag, error)

	// Create inserts a new tag. Returns ErrWikiTagConflict (already
	// translated by the repo layer) if (kb_id, name) is taken.
	Create(ctx context.Context, tag *types.WikiTag) error

	// Update applies non-nil fields from patch. Returns the updated row.
	Update(ctx context.Context, kbID string, tagID string, patch types.WikiTagUpdateRequest) (*types.WikiTag, error)

	// Delete removes the tag definition. wiki_page_tags rows cascade at
	// the DB level; the service still calls ClearPageTags as a safety
	// belt in case the cascade fires before the join-table cleanup.
	Delete(ctx context.Context, kbID string, tagID string) error

	// GetPageTags returns the tags currently attached to a single page.
	// Empty slice (not error) if the page has no tags.
	GetPageTags(ctx context.Context, kbID string, slug string) ([]types.WikiTag, error)

	// SetPageTags atomically replaces the join rows for one page.
	// The implementation owns the Tx boundary; the service calls into it
	// after validating that len(tagIDs) <= WikiTagMaxPerPage.
	SetPageTags(ctx context.Context, kbID string, slug string, tagIDs []string) ([]types.WikiTag, error)

	// AddTagToPages adds tagID to every page in slugs that lives in this
	// KB. Used by BatchTag with op='add'. Returns per-slug failure rows.
	AddTagToPages(ctx context.Context, kbID string, slugs []string, tagID string) (succeeded []string, failed []types.WikiPageBatchFailure, err error)

	// RemoveTagFromPages removes tagID from every page in slugs that
	// lives in this KB. "no row" is silent success — pages that don't
	// carry the tag are not counted as failures.
	RemoveTagFromPages(ctx context.Context, kbID string, slugs []string, tagID string) (succeeded []string, failed []types.WikiPageBatchFailure, err error)

	// ClearPageTags wipes the join rows for one page. Called by the
	// wiki_page DeletePage path so a soft-deleted page does not leave
	// orphan rows behind (wiki_page_tags has no FK on wiki_pages).
	ClearPageTags(ctx context.Context, pageID string) error
}

// WikiTagService is the high-level tag-management interface. It owns
// tag definitions (CRUD) and the join rows that bind tags to pages.
//
// Cross-cutting concerns enforced by the implementation (not the
// contract) so the interface stays small:
//
//   - tenant_id is stamped from the request context
//   - knowledge_base_id matches the URL path parameter
//   - all per-slug lookups happen against this KB only
//
// WikiBatchTag returns the same WikiBatchResponse shape as
// WikiPageService.Batch{Move,Delete,Status} so the handler can route
// the response through the same async/sync discriminator as Build #13.
type WikiTagService interface {
	// List returns every tag in the KB with its current page_count.
	// Sorted by (sort_order ASC, name ASC).
	List(ctx context.Context, kbID string) ([]types.WikiTagWithCount, error)

	// Get returns a single tag by ID. Returns (nil, ErrNotFound) if the
	// tag does not exist or lives in another KB.
	Get(ctx context.Context, kbID string, tagID string) (*types.WikiTag, error)

	// Create inserts a new tag. Returns ErrConflict when the same KB
	// already carries a tag with the same name.
	Create(ctx context.Context, kbID string, name string, color string) (*types.WikiTag, error)

	// Update applies a partial update. Only non-nil fields are touched.
	// Returns ErrNotFound if the tag does not belong to this KB.
	Update(ctx context.Context, kbID string, tagID string, patch types.WikiTagUpdateRequest) (*types.WikiTag, error)

	// Delete removes the tag definition and cascade-cleans the join
	// rows. Returns ErrNotFound if the tag does not belong to this KB.
	Delete(ctx context.Context, kbID string, tagID string) error

	// GetPageTags returns the tags currently attached to a single page.
	// Empty slice (not error) if the page has no tags.
	GetPageTags(ctx context.Context, kbID string, slug string) ([]types.WikiTag, error)

	// SetPageTags atomically replaces the tag associations of a page.
	// Returns ErrTagLimitExceeded when the new tag set exceeds
	// WikiTagMaxPerPage. The entire operation is wrapped in a single
	// transaction; on any error the existing associations are restored.
	SetPageTags(ctx context.Context, kbID string, slug string, tagIDs []string) ([]types.WikiTag, error)

	// BatchTag queues a batch-tag job (one WikiBatchJob row, type='tag')
	// when len(slugs) >= WikiBatchAsyncThreshold, or runs synchronously
	// otherwise. Returns WikiBatchRouteResult with kind='sync' or 'job'.
	BatchTag(ctx context.Context, kbID string, slugs []string, tagID string, op string) (*types.WikiBatchRouteResult, error)

	// ApplyBatchTagOneSlug runs one slug's add/remove on the worker
	// pool's per-slug path. Idempotent for op=add (ON CONFLICT DO
	// NOTHING) and silent-success for op=remove when the page doesn't
	// carry the tag. Returns the per-slug error so the worker can
	// translate it into a WikiBatchJobFailureRecord.
	//
	// Build #17.
	ApplyBatchTagOneSlug(ctx context.Context, kbID string, slug string, tagID string, op string) error
}