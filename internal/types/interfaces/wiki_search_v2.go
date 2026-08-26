// Package interfaces — Build #19 / P2.x.a wiki search v2 interface.
//
// The v2 path lives on a separate repo + service to avoid disrupting the
// legacy `Search` repo (Build #9-A's regex path) and the `WikiPageService`
// surface. A single endpoint URL fans out to either via `?v=2` query.
package interfaces

import (
	"context"

	"github.com/louloulin/WeKnora/internal/types"
)

// WikiSearchV2Repository performs PostgreSQL tsvector search with
// ts_headline snippet generation and ts_rank ordering. The legacy
// `Search` repo (regex `~*`) stays untouched for the deprecation window.
type WikiSearchV2Repository interface {
	// Search executes the tsvector query. visibleKBIDs is the KB-ACL
	// pre-filter (already validated upstream); nil means "all KBs in
	// the caller's tenant". Returns hits + total + took_ms.
	Search(
		ctx context.Context,
		tenantID uint64,
		denyUserIDs []string,
		req types.WikiSearchV2Request,
		visibleKBIDs []string,
	) (types.WikiSearchV2Result, error)
}

// WikiSearchV2Service validates input, calls the repository, and
// returns a result. Pagination/limit clamping happens here so the repo
// can stay dumb about input bounds.
type WikiSearchV2Service interface {
	Search(
		ctx context.Context,
		tenantID uint64,
		userID string,
		req types.WikiSearchV2Request,
		visibleKBIDs []string,
	) (types.WikiSearchV2Result, error)
}