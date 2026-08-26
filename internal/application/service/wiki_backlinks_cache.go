package service

import (
	"context"
	"log"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// wikiBacklinksCacheInvalidator is the service-layer implementation
// of WikiBacklinksCacheInvalidator (Build #21). It only knows the
// slug-resolution policy; the cache write/DELETE is delegated to the
// repository (WikiBacklinksCacheRepository) which knows the table
// layout.
//
// The invalidator does NOT depend on the WikiPageRepository directly:
// every public method takes (op, kbID, slug) and lets the caller
// (WikiPageService.CreatePage / UpdatePage / ...) supply the slug set
// it has already resolved from the just-written wiki_pages row. This
// keeps the dependency graph one-way:
//
//   WikiPageService ──> CacheInvalidator ──> CacheRepository
//                              │
//                              └── reads from WikiPageRepo (for in_links / out_links)
//
// In tests the WikiPageRepo is replaced by an in-memory stub so the
// invalidator's slug-resolution rules can be driven without a DB.
type wikiBacklinksCacheInvalidator struct {
	pageRepo   interfaces.WikiPageRepository
	cacheRepo  interfaces.WikiBacklinksCacheRepository
}

// newWikiBacklinksCacheInvalidator wires the invalidator into the DI
// container. Returns the interface so the service depends on the
// contract.
func newWikiBacklinksCacheInvalidator(
	pageRepo interfaces.WikiPageRepository,
	cacheRepo interfaces.WikiBacklinksCacheRepository,
) interfaces.WikiBacklinksCacheInvalidator {
	return &wikiBacklinksCacheInvalidator{
		pageRepo:  pageRepo,
		cacheRepo: cacheRepo,
	}
}

// Resolve returns the slug set whose cache row must be wiped for the
// given op. The rule (from spec D5):
//
//   CreatePage(A)  → [A] ∪ A.out_links      (A is new; its out-links now
//                                              resolve to it as a new target)
//   UpdatePage(A)  → [A] ∪ A.out_links      (A.out_links might have changed;
//                                              target pages' in_links changed
//                                              transitively → their cache stale)
//   DeletePage(A)  → [A] ∪ A.in_links       (A disappears; sources lose a backlink)
//   MovePage(A old → new) → [old, new] ∪ new.out_links
//                                            (slug rename: oldSlug no longer
//                                              resolves; newSlug + its targets
//                                              need fresh in_links counts)
//   BatchMove(slugs[]) → uniq(slugs ∪ recursiveOutLinks(slugs))
//   BatchDelete(slugs[]) → uniq(slugs ∪ recursiveInLinks(slugs))
//   BatchStatus(slugs[]) → uniq(slugs)       (status doesn't affect backlink
//                                              content; the slug itself may
//                                              have lost visibility, but that's
//                                              ACL-handled separately)
//
// `slug` is the primary slug affected; for batch ops we read each slug
// and accumulate.
//
// The function never returns nil — callers can iterate / uniq safely.
func (i *wikiBacklinksCacheInvalidator) Resolve(
	ctx context.Context,
	op types.BacklinkCacheInvalidateOp,
	kbID string,
	slug string,
) ([]string, error) {
	if kbID == "" {
		return []string{}, nil
	}
	switch op {
	case types.BacklinkCacheInvalidateCreatePage:
		return i.slugWithOutLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateUpdatePage:
		return i.slugWithOutLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateDeletePage:
		return i.slugWithInLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateMovePage:
		// MovePage's caller already computed [oldSlug, newSlug]; the
		// `slug` parameter here is the newSlug. Old-slug invalidation
		// is the caller's responsibility (we never have the old slug
		// post-rename).
		return i.slugWithOutLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateBatchMove:
		return i.slugWithOutLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateBatchDelete:
		return i.slugWithInLinks(ctx, kbID, slug), nil
	case types.BacklinkCacheInvalidateBatchStatus:
		if slug == "" {
			return []string{}, nil
		}
		return []string{slug}, nil
	default:
		log.Printf("wikiBacklinksCacheInvalidator.Resolve: unknown op %q (slug=%q), returning slug only", op, slug)
		if slug == "" {
			return []string{}, nil
		}
		return []string{slug}, nil
	}
}

// slugWithOutLinks returns {slug} ∪ page.out_links, dedup'd, with
// empty entries filtered. Used by Create/Update/MovePage — the slug
// itself is the "just-written" page whose cache is definitely stale,
// and the out_links are the targets whose in_links count just changed.
func (i *wikiBacklinksCacheInvalidator) slugWithOutLinks(
	ctx context.Context,
	kbID string,
	slug string,
) []string {
	out := make([]string, 0, 8)
	if slug != "" {
		out = append(out, slug)
	}
	if slug == "" || i.pageRepo == nil {
		return uniqNonEmpty(out)
	}
	page, err := i.pageRepo.GetBySlug(ctx, kbID, slug)
	if err != nil || page == nil {
		return uniqNonEmpty(out)
	}
	for _, link := range page.OutLinks {
		s := trimLink(link)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return uniqNonEmpty(out)
}

// slugWithInLinks returns {slug} ∪ page.in_links, dedup'd. Used by
// DeletePage / BatchDelete — A is gone, A.in_links lost a source.
func (i *wikiBacklinksCacheInvalidator) slugWithInLinks(
	ctx context.Context,
	kbID string,
	slug string,
) []string {
	out := make([]string, 0, 8)
	if slug != "" {
		out = append(out, slug)
	}
	if slug == "" || i.pageRepo == nil {
		return uniqNonEmpty(out)
	}
	page, err := i.pageRepo.GetBySlug(ctx, kbID, slug)
	if err != nil || page == nil {
		return uniqNonEmpty(out)
	}
	for _, link := range page.InLinks {
		s := trimLink(link)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return uniqNonEmpty(out)
}

// Invalidate runs the actual cache DELETE for the affected slug set.
// Warnings are logged but never returned — a failed wipe just means
// the next read recomputes on miss and writes the new value, so the
// system self-heals on first read.
func (i *wikiBacklinksCacheInvalidator) Invalidate(
	ctx context.Context,
	req types.BacklinkCacheInvalidateRequest,
) (int64, error) {
	if req.KbID == "" || len(req.AffectedSlugs) == 0 {
		return 0, nil
	}
	affected, err := i.cacheRepo.Delete(ctx, req.KbID, req.AffectedSlugs)
	if err != nil {
		log.Printf("wikiBacklinksCacheInvalidator.Invalidate: %s op=%s kb=%s slugs=%v: %v",
			"cache wipe failed", req.Op, req.KbID, req.AffectedSlugs, err)
		return 0, nil // warnings only
	}
	if affected == 0 {
		log.Printf("wikiBacklinksCacheInvalidator.Invalidate: %s op=%s kb=%s slugs=%v (cache row already absent)",
			"noop", req.Op, req.KbID, req.AffectedSlugs)
	}
	return affected, nil
}

// trimLink trims a single link entry. The Build #20 service stores
// links as plain slugs ("entity/acme"); trimLink leaves them intact
// but guards against leading/trailing whitespace from manual edits.
func trimLink(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// uniqNonEmpty removes empty entries and duplicates while preserving
// the first-seen order (so the slug always appears first).
func uniqNonEmpty(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}