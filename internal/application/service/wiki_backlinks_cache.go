package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// slugSetStrategies is the Build #28 registry — every known op maps to
// exactly one SlugSetStrategy. The 9-op × 1-strategy cells are
// exhaustively tested by TestInvalidatorResolve_AllOpsHaveStrategy
// in wiki_backlinks_cache_invalidator_test.go; adding an op without
// registering it panics at Resolve time (D1 — no silent fallback).
//
// Two ops (cleanup_sweep, acl_change) never call into Resolve —
// CleanupService and the acl service own their wipe paths directly.
// They're listed anyway so the registry can flag future callers that
// try to send them through the invalidator's standard dispatch and
// forces an explicit decision rather than a "well, slug only" silent
// fallback.
var slugSetStrategies = map[types.BacklinkCacheInvalidateOp]types.SlugSetStrategy{
	types.BacklinkCacheInvalidateCreatePage:  types.SlugSetStrategySelfOutgoing,
	types.BacklinkCacheInvalidateUpdatePage:  types.SlugSetStrategySelfOutgoing,
	types.BacklinkCacheInvalidateDeletePage:  types.SlugSetStrategySelfIncoming,
	types.BacklinkCacheInvalidateMovePage:    types.SlugSetStrategySelfOutgoing,
	types.BacklinkCacheInvalidateBatchMove:   types.SlugSetStrategySelfOutgoing,
	types.BacklinkCacheInvalidateBatchDelete: types.SlugSetStrategySelfIncoming,
	types.BacklinkCacheInvalidateBatchStatus: types.SlugSetStrategySelf,
	types.BacklinkCacheInvalidateSweep:       types.SlugSetStrategyKBWide,
	types.BacklinkCacheInvalidateAclChange:   types.SlugSetStrategyReverseLookupIndexed,
}

// wikiBacklinksCacheInvalidator is the service-layer implementation
// of WikiBacklinksCacheInvalidator (Build #21). It only knows the
// slug-resolution policy; the cache write/DELETE is delegated to the
// repository (WikiBacklinksCacheRepository) which knows the table
// layout.
//
// Build #28 — Resolve now returns the picked SlugSetStrategy as a
// second value; Invalidate takes it as a parameter and stamps it
// into the audit row's details.strategy JSON field. The
// (op × strategy) matrix is the single source of truth in
// slugSetStrategies — the switch in Resolve is gone, replaced by a
// table lookup + per-strategy helper.
type wikiBacklinksCacheInvalidator struct {
	pageRepo  interfaces.WikiPageRepository
	cacheRepo interfaces.WikiBacklinksCacheRepository
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

// NewWikiBacklinksCacheInvalidator wires the production invalidator for DI.
func NewWikiBacklinksCacheInvalidator(
	pageRepo interfaces.WikiPageRepository,
	cacheRepo interfaces.WikiBacklinksCacheRepository,
) interfaces.WikiBacklinksCacheInvalidator {
	return newWikiBacklinksCacheInvalidator(pageRepo, cacheRepo)
}

// Resolve returns the slug set whose cache row must be wiped for the
// given op + the strategy label that explains the choice.
//
// Build #21 rule set, lifted into the table-driven Build #28 form:
//
//	CreatePage(A)  → [A] ∪ A.out_links        (self_outgoing)
//	UpdatePage(A)  → [A] ∪ A.out_links        (self_outgoing)
//	DeletePage(A)  → [A] ∪ A.in_links         (self_incoming)
//	MovePage(A)    → [A] ∪ A.out_links        (self_outgoing)
//	                 (MovePage's caller has already prepended
//	                 oldSlug to the affected-slug set; we only see
//	                 newSlug here.)
//	BatchMove(s)   → [s] ∪ s.out_links       (self_outgoing)
//	BatchDelete(s) → [s] ∪ s.in_links        (self_incoming)
//	BatchStatus(s) → [s]                      (self)
//
// `slug` is the primary slug affected; for batch ops the caller calls
// Resolve once per slug. The function never returns nil — callers can
// iterate / uniq safely.
//
// Unknown ops PANIC (D1). The previous switch had a default branch
// that silently degraded to "slug only" — that branch is what
// produced the production "missing wipe" incident class that
// motivated Build #28.
func (i *wikiBacklinksCacheInvalidator) Resolve(
	ctx context.Context,
	op types.BacklinkCacheInvalidateOp,
	kbID string,
	slug string,
) ([]string, types.SlugSetStrategy, error) {
	if kbID == "" {
		return []string{}, "", nil
	}
	strategy, ok := slugSetStrategies[op]
	if !ok {
		panic(fmt.Sprintf(
			"wikiBacklinksCacheInvalidator.Resolve: op %q not registered in slugSetStrategies; "+
				"add it to the table in wiki_backlinks_cache.go before using it (Build #28 D1)",
			op,
		))
	}
	// self / self_outgoing / self_incoming run through Resolve because
	// they need pageRepo. kb_wide / reverse_lookup_indexed are listed
	// for registry completeness but their wipe paths live elsewhere —
	// returning (nil, strategy, nil) lets the type system reflect
	// "this op registered but didn't pick a slug set here" without
	// forcing the caller to special-case them.
	switch strategy {
	case types.SlugSetStrategySelfOutgoing:
		return i.slugWithOutLinks(ctx, kbID, slug), strategy, nil
	case types.SlugSetStrategySelfIncoming:
		return i.slugWithInLinks(ctx, kbID, slug), strategy, nil
	case types.SlugSetStrategySelf:
		if slug == "" {
			return []string{}, strategy, nil
		}
		return []string{slug}, strategy, nil
	case types.SlugSetStrategyKBWide, types.SlugSetStrategyReverseLookupIndexed:
		// CleanupService / acl service own their wipe paths. Returning
		// ([], strategy) here means a stray call through Resolve would
		// wipe nothing — but the registry makes that visible. Defensive
		// callers can still call Invalidate(req, strategy) and get the
		// strategy stamped on the audit row.
		return []string{}, strategy, nil
	default:
		// Should be unreachable given the slugSetStrategies table
		// already covered all 5 constants; defensive panic.
		panic(fmt.Sprintf(
			"wikiBacklinksCacheInvalidator.Resolve: strategy %q has no dispatch arm",
			strategy,
		))
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

// Invalidate runs the actual cache DELETE for the affected slug set
// and writes the audit row tagged with `strategy`. Strategy is
// threaded into the audit row's details.strategy JSON field so
// operators can read "what kind of wipe was this" off the log alone.
//
// Build #28 — the strategy parameter replaces the implicit "whatever
// Resolve picked" the previous API forced callers to compute
// separately. Now the contract is: caller runs Resolve(op, kbID,
// slug) → (slugs, strategy), then Invalidate(req, strategy).
//
// Warnings are logged but never returned — a failed wipe just means
// the next read recomputes on miss and writes the new value, so the
// system self-heals on first read.
func (i *wikiBacklinksCacheInvalidator) Invalidate(
	ctx context.Context,
	req types.BacklinkCacheInvalidateRequest,
	strategy types.SlugSetStrategy,
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
	// Best-effort: persist the audit row. Details now carries
	// strategy so the reader can render "what rule wiped these
	// slugs" without consulting the source. Errors are warn-logged
	// but never returned — losing one audit row must not block the
	// cache DELETE.
	detailsJSON, _ := encodeInvalidationDetails(req.AffectedSlugs, string(req.Op), string(strategy))
	sourceEventID := wikiSourceEventIDFromContext(ctx)
	actorUserID := wikiActorUserIDFromContext(ctx)
	if logErr := i.cacheRepo.LogInvalidation(ctx, &types.WikiBacklinksCacheInvalidationLogEntry{
		KbID:          req.KbID,
		Slug:          req.AffectedSlugs[0], // first slug as the canonical "primary" key
		Op:            string(req.Op),
		ActorUserID:   actorUserID,
		CorrelationID: sourceEventID,
		AffectedCount: int(affected),
		Details:       string(detailsJSON),
	}); logErr != nil {
		log.Printf("wikiBacklinksCacheInvalidator.Invalidate: invalidation log insert failed (op=%s kb=%s): %v",
			req.Op, req.KbID, logErr)
	}
	return affected, nil
}

// encodeInvalidationDetails marshals the audit row's Details JSON.
// Build #28 — adds a `strategy` field so the audit log can answer
// "what rule picked this slug set" without grep'ing the source. Old
// rows pre-Build #28 lack the field; the reader is responsible for
// the missing-key fallback (see WikiAuditService.ListAuditEvents).
func encodeInvalidationDetails(slugs []string, op string, strategy string) ([]byte, error) {
	payload := map[string]any{
		"slugs":    slugs,
		"op":       op,
		"strategy": strategy,
	}
	return json.Marshal(payload)
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
