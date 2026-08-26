package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiTemplateService is the surface the wiki template skeleton
// engine exposes. The handler depends on this interface so tests
// can swap in a stub.
//
// Build #18 / P1.2.
type WikiTemplateService interface {
	// ApplyTemplate is the single entry point used by both
	// the apply-template dialog and the rebuild-children button.
	// The service is responsible for:
	//   - validating the skeleton (size cap, slug uniqueness)
	//   - deleting prior auto-generated children atomically
	//   - creating N new children inside the same transaction
	//   - rewriting the parent body in-place (placeholders)
	//   - resolving tagged-pages tokens via WikiTagService
	//
	// Returns the new WikiApplyTemplateResult.
	ApplyTemplate(
		ctx context.Context,
		kbID string,
		parentSlug string,
		req types.WikiApplyTemplateRequest,
	) (*types.WikiApplyTemplateResult, error)

	// PreviewSkeleton is a pure helper the frontend calls before
	// the user clicks "确认应用". It resolves tagged-pages tokens
	// (which may be expensive on big KBs) and produces the same
	// body rewrite the apply path will produce, without writing
	// anything. The handler returns it as PreviewSkeletonResponse.
	PreviewSkeleton(
		ctx context.Context,
		kbID string,
		parentSlug string,
		req types.WikiApplyTemplateRequest,
	) (*types.WikiApplyTemplateResult, error)
}