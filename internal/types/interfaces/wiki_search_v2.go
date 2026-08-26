// Package interfaces — Build #19 / P2.x.a wiki search v2 interface.
//
// The v2 path lives on a separate repo + service to avoid disrupting the
// legacy `Search` repo (Build #9-A's regex path) and the `WikiPageService`
// surface. A single endpoint URL fans out to either via `?v=2` query.
//
// Build #19.x extends the repo signature with `zhQuery` (jieba-tokenized
// form of `req.Query`) so the SQL can hit the new `content_ts_zh` GIN
// index (migration 000096). The service layer is responsible for calling
// gojieba before invoking the repo, keeping the SQL itself jieba-free.
package interfaces

import (
	"context"

	"github.com/louloulin/WeKnora/internal/types"
)

// WikiSearchV2Repository performs PostgreSQL tsvector search with
// ts_headline snippet generation and ts_rank ordering. The legacy
// `Search` repo (regex `~*`) stays untouched for the deprecation window.
type WikiSearchV2Repository interface {
	// Search executes the three-layer OR query. visibleKBIDs is the
	// KB-ACL pre-filter (already validated upstream); nil means "all
	// KBs in the caller's tenant". zhQuery is the jieba-tokenized form
	// of req.Query (space-joined tokens); pass "" when there is no
	// useful Chinese tokenization (latin-only query, empty after cut,
	// etc.) to skip the ts_zh arm. Returns hits + total + took_ms.
	Search(
		ctx context.Context,
		tenantID uint64,
		denyUserIDs []string,
		req types.WikiSearchV2Request,
		visibleKBIDs []string,
		zhQuery string,
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