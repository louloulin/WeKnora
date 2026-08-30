// Package service — Build #19 / P2.x.a wiki search v2 service.
//
// The service validates input and post-filters the repo's hits through
// WikiAclService.Resolve so private pages / allow_list pages that the
// caller cannot read never reach the client. The repo intentionally
// stays ACL-unaware — its single concern is tsvector + tenant scoping —
// and the service carries the visibility responsibility end-to-end.
//
// Build #19.x adds jieba tokenization before the repo call so the SQL
// can hit the new `content_ts_zh` GIN index (migration 000096) when the
// query contains Chinese. The repo stays jieba-free; the service is the
// single point where Go-level decisions (tokenization, fuzzy toggle)
// meet SQL-level decisions (tsvector, ts_rank).
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiSearchV2Repo is the minimal repo surface the service needs.
type WikiSearchV2Repo interface {
	Search(
		ctx context.Context,
		tenantID uint64,
		denyUserIDs []string,
		req types.WikiSearchV2Request,
		visibleKBIDs []string,
		zhQuery string,
	) (types.WikiSearchV2Result, error)
}

// WikiSearchV2ServiceParams bundles deps for NewWikiSearchV2Service.
type WikiSearchV2ServiceParams struct {
	Repo WikiSearchV2Repo
	KB   interfaces.KnowledgeBaseService
	ACL  WikiAclService
}

// wikiSearchV2Service is the production implementation.
type wikiSearchV2Service struct {
	repo WikiSearchV2Repo
	kb   interfaces.KnowledgeBaseService
	acl  WikiAclService
}

// NewWikiSearchV2Service wires the service.
func NewWikiSearchV2Service(p WikiSearchV2ServiceParams) interfaces.WikiSearchV2Service {
	return &wikiSearchV2Service{repo: p.Repo, kb: p.KB, acl: p.ACL}
}

// Search validates the request, calls the repo, and applies per-hit ACL
// filtering. A hit whose ACL decision is not `allow` is dropped silently
// (not even a `<mark>` stub) so the caller never learns the page exists.
//
// Pagination clamps: limit defaults to 20, max 100; offset defaults to
// 0, never negative. PageTypes / KBIDs lists have duplicates removed and
// are lowercased / trimmed to keep the SQL parameter set bounded.
func (s *wikiSearchV2Service) Search(
	ctx context.Context,
	tenantID uint64,
	userID string,
	req types.WikiSearchV2Request,
	visibleKBIDs []string,
) (types.WikiSearchV2Result, error) {
	if tenantID == 0 {
		return types.WikiSearchV2Result{}, fmt.Errorf("wiki search v2: tenant id missing")
	}

	// Normalize request.
	req.Query = strings.TrimSpace(req.Query)
	req.KBIDs = normalizeKBIDs(req.KBIDs)
	req.PageTypes = normalizeStrings(req.PageTypes)

	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	// Empty query short-circuits to an empty result set. We still run a
	// single 200 response so the client can render "type to search".
	if req.Query == "" {
		return types.WikiSearchV2Result{
			Hits:   []types.WikiSearchV2Hit{},
			Total:  0,
			TookMS: 0,
			KBIDs:  req.KBIDs,
			Query:  "",
		}, nil
	}

	// Resolve effective KB scope. If the caller asked for explicit
	// kb_ids[], intersect with visibleKBIDs (the caller's KB-ACL list)
	// so a user cannot probe KBs they don't have access to.
	effectiveVisible := visibleKBIDs
	if len(req.KBIDs) > 0 {
		effectiveVisible = intersectKBIDs(visibleKBIDs, req.KBIDs)
	}
	if len(effectiveVisible) == 0 && len(visibleKBIDs) > 0 {
		// Caller is restricted to a KB-ACL set and none of their
		// requested kb_ids[] overlap — return empty hits.
		return types.WikiSearchV2Result{
			Hits:   []types.WikiSearchV2Hit{},
			Total:  0,
			TookMS: 0,
			KBIDs:  req.KBIDs,
			Query:  req.Query,
		}, nil
	}

	// Build #19.x — jieba-tokenize the query once for the ts_zh arm.
	// The repo treats an empty string as "skip the zh arm" so a pure
	// English query falls straight through to ts_simple / trgm.
	zhQuery := JiebaSegmentForSearch("", req.Query)

	raw, err := s.repo.Search(ctx, tenantID, nil, req, effectiveVisible, zhQuery)
	if err != nil {
		return types.WikiSearchV2Result{}, err
	}

	// ACL post-filter. WikiAclService.ResolveBulk fans out across the
	// hits with a small worker pool; the underlying Resolve still uses
	// its per-(kb,slug,user) cache, so the post-filter stays cheap even
	// when the result set is large. Per-hit errors are mapped to the
	// conservative deny inside ResolveBulk, so a transient ACL lookup
	// failure never leaks a hit to the caller.
	filtered := make([]types.WikiSearchV2Hit, 0, len(raw.Hits))
	if len(raw.Hits) > 0 {
		items := make([]AclResolveItem, 0, len(raw.Hits))
		for _, hit := range raw.Hits {
			if hit.Slug == "" || hit.KBID == "" {
				continue
			}
			items = append(items, AclResolveItem{KBID: hit.KBID, Slug: hit.Slug})
		}
		decisions, err := s.acl.ResolveBulk(ctx, items, userID)
		if err != nil {
			logger.Warnf(ctx, "wiki search v2: acl resolve bulk failed: %v", err)
		}
		for _, hit := range raw.Hits {
			if hit.Slug == "" || hit.KBID == "" {
				continue
			}
			key := hit.KBID + ":" + hit.Slug
			if decisions[key] != types.WikiPageAclAllow {
				continue
			}
			filtered = append(filtered, hit)
		}
	}

	raw.Hits = filtered
	raw.Total = len(filtered)
	return raw, nil
}

func normalizeKBIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func normalizeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func intersectKBIDs(visible, requested []string) []string {
	if len(visible) == 0 {
		return requested
	}
	allowed := make(map[string]struct{}, len(visible))
	for _, k := range visible {
		allowed[k] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, k := range requested {
		if _, ok := allowed[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

// Compile-time interface check.
var _ interfaces.WikiSearchV2Service = (*wikiSearchV2Service)(nil)
