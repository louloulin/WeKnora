package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiAclRepo is the minimal storage surface WikiAclService needs. It is
// kept private to the ACL service so the canonical WikiPageRepository
// interface (which is huge and used by every read path) does not have to
// grow an ACL-specific method on every implementation.
type WikiAclRepo = interfaces.WikiAclRepository

// aclChangeCacheThreshold is the per-KB row count above which the
// Build #24 ACL→cache hook switches from a full-wipe path
// (DeleteByKB) to a reverse-lookup path (FindReferencingSlugs → Delete).
// 10k is the same threshold Build #23 uses for the cold-row count
// surface; below it the wipe is sub-millisecond on every supported
// dialect.
const aclChangeCacheThreshold = 10000

// WikiAclService is the single decision point for page-level ACL. Every
// wiki read path consults Resolve before returning content; private /
// allow_list mismatches are mapped to a "page not found" 404 by the caller
// so the page's existence is not leaked.
type WikiAclService interface {
	Resolve(ctx context.Context, kbID string, slug string, callerUserID string) (string, error)
	// ResolveBulk fans Resolve out across many (kbID, slug) pairs for a
	// single caller. The search v2 service uses it to filter a hit list
	// without serializing on per-hit ACL round trips. Cache behaviour is
	// identical to Resolve — each pair is keyed as kb|slug|user and the
	// existing 60 s TTL applies. Per-hit errors map to the conservative
	// `deny_allow_list` decision (and are logged) so the caller never
	// sees a hit whose ACL could not be verified.
	ResolveBulk(ctx context.Context, items []AclResolveItem, callerUserID string) (map[string]string, error)
	GetAcl(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error)
	PutAcl(ctx context.Context, kbID string, slug string,
		req types.WikiPageAclSaveRequest, callerUserID string, callerRole string) (*types.WikiPageAcl, error)
	SearchAclCandidates(ctx context.Context, tenantID uint64, query string, limit int) ([]*types.User, error)
}

// AclResolveItem is one (kbID, slug) pair for ResolveBulk. Slug is the
// per-page identifier inside a KB; KBID scopes the page to a tenant-scoped
// KB so the ACL service can read the right column.
type AclResolveItem struct {
	KBID string
	Slug string
}

// aclResolveBulkWorkers bounds the goroutine fan-out for ResolveBulk. Four
// is enough to keep most search-result pages in-flight without spinning up
// a goroutine per hit; the cache absorbs the rest.
const aclResolveBulkWorkers = 4

// aclCacheTTL bounds how long a single decision stays cached. 60 s is
// short enough that a manual permission grant by an admin propagates
// quickly even when the PUT path is missed, and long enough that the
// decision hot path stays cheap.
const aclCacheTTL = 60 * time.Second

// aclCacheEntry pairs the decision with its expiry. value is one of the
// WikiPageAclAllow / WikiPageAclDeny* constants.
type aclCacheEntry struct {
	value     string
	expiresAt time.Time
}

// aclCache is a tiny per-process LRU for ACL decisions. Keys use the form
// `kbID|slug|userID` so a single PUT can invalidate every entry for a page
// by scanning the key prefix.
type aclCache struct {
	mu    sync.Mutex
	store map[string]aclCacheEntry
}

func newAclCache() *aclCache {
	return &aclCache{store: make(map[string]aclCacheEntry)}
}

func (c *aclCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.store[key]
	if !ok || time.Now().After(e.expiresAt) {
		if ok {
			delete(c.store, key)
		}
		return "", false
	}
	return e.value, true
}

func (c *aclCache) set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = aclCacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
}

// invalidatePrefix removes every entry whose key starts with prefix. Used
// after a successful PUT so the next read picks up the new ACL. O(n) over
// the cache; the cache is bounded by page×user cardinality per KB and
// reset on each restart, so this stays cheap.
func (c *aclCache) invalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.store {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.store, k)
		}
	}
}

// wikiAclService is the production implementation of WikiAclService.
type wikiAclService struct {
	repo      WikiAclRepo
	userSvc   interfaces.UserService
	cacheRepo interfaces.WikiBacklinksCacheRepository
	cache     *aclCache
}

// NewWikiAclService wires the service. userSvc is used for the ACL dialog's
// candidate picker (SearchAclCandidates).
func NewWikiAclService(repo WikiAclRepo, userSvc interfaces.UserService) WikiAclService {
	return &wikiAclService{repo: repo, userSvc: userSvc, cache: newAclCache()}
}

// SetCacheRepo wires the Build #24 ACL→cache hook dependency. Passing
// nil disables the wipe-on-write side effect — the service warns and
// skips when PutAcl runs with cacheRepo == nil. Container.go calls
// this after the DI graph resolves the backlinks cache repository so
// the existing constructor signature stays unchanged for harness tests
// that don't care about the cache layer.
func (s *wikiAclService) SetCacheRepo(cacheRepo interfaces.WikiBacklinksCacheRepository) {
	s.cacheRepo = cacheRepo
}

// Resolve returns the ACL decision for a caller against a page. Owner and
// KB admin always get allow without consulting the column; otherwise the
// stored ACL mode drives the decision, with allow_list expanded through
// GroupMembers so a user can land on the allow side via group membership.
//
// Cache key: `kb|slug|user`. The decision is cached for 60 s. A PUT calls
// invalidatePrefix(kb|slug|) so all users for that page see the new ACL
// on the next read.
func (s *wikiAclService) Resolve(ctx context.Context, kbID string, slug string, callerUserID string) (string, error) {
	key := kbID + "|" + slug + "|" + callerUserID
	if v, ok := s.cache.get(key); ok {
		return v, nil
	}
	ownerID, isAdmin, err := s.repo.PageOwnerAndAdmin(ctx, kbID, slug, callerUserID)
	if err != nil {
		return "", err
	}
	if isAdmin || (ownerID != "" && ownerID == callerUserID) {
		s.cache.set(key, types.WikiPageAclAllow, aclCacheTTL)
		return types.WikiPageAclAllow, nil
	}
	acl, err := s.repo.GetAclBySlug(ctx, kbID, slug)
	if err != nil {
		return "", err
	}
	if acl == nil {
		// Legacy NULL row → inherit. Caller is not owner/admin (caught above),
		// and inherit grants every KB member, so allow.
		s.cache.set(key, types.WikiPageAclAllow, aclCacheTTL)
		return types.WikiPageAclAllow, nil
	}
	switch acl.Mode {
	case types.WikiPageAclModeInherit:
		s.cache.set(key, types.WikiPageAclAllow, aclCacheTTL)
		return types.WikiPageAclAllow, nil
	case types.WikiPageAclModePrivate:
		s.cache.set(key, types.WikiPageAclDenyPrivate, aclCacheTTL)
		return types.WikiPageAclDenyPrivate, nil
	case types.WikiPageAclModeAllowList:
		if containsString(acl.AllowUserIDs, callerUserID) {
			s.cache.set(key, types.WikiPageAclAllow, aclCacheTTL)
			return types.WikiPageAclAllow, nil
		}
		if len(acl.AllowGroupIDs) > 0 {
			tenantID, ok := types.TenantIDFromContext(ctx)
			if !ok || tenantID == 0 {
				logger.Warnf(ctx, "wiki acl resolve: tenant id missing in context for kb=%s slug=%s", kbID, slug)
			} else {
				memberIDs, err := s.repo.GroupMembers(ctx, tenantID, acl.AllowGroupIDs)
				if err != nil {
					return "", err
				}
				if containsString(memberIDs, callerUserID) {
					s.cache.set(key, types.WikiPageAclAllow, aclCacheTTL)
					return types.WikiPageAclAllow, nil
				}
			}
		}
		s.cache.set(key, types.WikiPageAclDenyAllowList, aclCacheTTL)
		return types.WikiPageAclDenyAllowList, nil
	default:
		// Unknown mode: be conservative and deny. This should never happen
		// because IsValidWikiPageAclMode would have rejected the value at
		// write time, but if it sneaks in (e.g. column mutated by hand)
		// we don't want to silently allow.
		logger.Warnf(ctx, "wiki acl resolve: unknown mode %q for kb=%s slug=%s, denying", acl.Mode, kbID, slug)
		s.cache.set(key, types.WikiPageAclDenyAllowList, aclCacheTTL)
		return types.WikiPageAclDenyAllowList, nil
	}
}

// GetAcl returns the current ACL for a page, normalizing NULL/empty to
// a fresh inherit-mode record with revision 0.
func (s *wikiAclService) GetAcl(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error) {
	acl, err := s.repo.GetAclBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if acl == nil {
		return &types.WikiPageAcl{Mode: types.WikiPageAclModeInherit, Revision: 0}, nil
	}
	return acl, nil
}

// PutAcl writes a new ACL after validating mode and verifying the optimistic
// lock. On conflict, returns types.ErrWikiPageAclRevisionConflict so the
// handler can map it to HTTP 409. On success, invalidates the cache for
// this page across all users, then fires the Build #24 ACL→cache wipe
// hook so the backlinks cache row for this page — and every row that
// references it — is dropped before the next ListBacklinkGraph call.
//
// Build #27 — when the new ACL payload's hash matches the stored
// `acl_snapshot_hash` value, the cache wipe + invalidation-log row are
// skipped (the revision still bumps and the audit row is written with
// action="noop_match" so the audit-trail invariant holds). A legacy row
// with an empty stored hash always runs the wipe — see spec D4.
func (s *wikiAclService) PutAcl(ctx context.Context, kbID string, slug string,
	req types.WikiPageAclSaveRequest, callerUserID string, callerRole string) (*types.WikiPageAcl, error) {
	// Empty mode is treated as inherit on read, but a PUT must specify
	// a target mode explicitly — silently rewriting the ACL to "inherit"
	// when the frontend sent an empty string would hide a real bug.
	mode := req.Mode
	if mode == "" {
		mode = types.WikiPageAclModeInherit
	}
	if !types.IsValidWikiPageAclMode(mode) {
		return nil, fmt.Errorf("invalid acl mode %q", req.Mode)
	}
	// Build #27 — compute the new payload's hash up-front so we can
	// compare against the stored value before the write commits.
	newHash := HashAcl(mode, req.AllowUserIDs, req.AllowGroupIDs, req.DenyInherited)
	// Capture the before-state + current revision + stored hash in one
	// read. The before-mode is consumed by the audit Details payload;
	// the beforeHash drives the noop decision. Read best-effort: a read
	// failure here is treated as "no prior state" → noop=false → wipe
	// runs (the safe default — never skip a wipe we couldn't verify).
	var (
		beforeMode = ""
		beforeHash = ""
		noop       = false
	)
	if before, getErr := s.repo.GetAclBySlug(ctx, kbID, slug); getErr == nil && before != nil {
		beforeMode = before.Mode
		beforeHash = before.SnapshotHash
	}
	// D4 — a stored hash of "" never matches a real hash, so legacy rows
	// always run the wipe on their first PutAcl post-migration.
	if beforeHash != "" && beforeHash == newHash {
		noop = true
	}
	action := actionForMode(mode)
	if noop {
		// Spec D5 — the audit row still gets written so the timeline
		// has no gaps, but the action label is overridden to make the
		// "no semantic change" signal explicit.
		action = "noop_match"
	}
	updated, err := s.repo.UpdateAclWithRevision(ctx, kbID, slug, types.WikiPageAcl{
		Mode:          mode,
		AllowUserIDs:  req.AllowUserIDs,
		AllowGroupIDs: req.AllowGroupIDs,
		DenyInherited: req.DenyInherited,
	}, req.BaseRevision, newHash, callerUserID, callerRole, action)
	if err != nil {
		if errors.Is(err, types.ErrWikiPageAclRevisionConflict) {
			return nil, err
		}
		return nil, err
	}
	s.cache.invalidatePrefix(kbID + "|" + slug + "|")
	s.invalidateBacklinksCacheOnAclChange(ctx, kbID, slug, beforeMode, mode, req.BaseRevision, updated.Revision, noop)
	if noop {
		metricAclChangeSkippedTotal.WithLabelValues("hash_match").Inc()
	}
	return updated, nil
}

// invalidateBacklinksCacheOnAclChange is the Build #24 ACL→cache hook.
// It picks a wipe strategy based on the KB's row count and writes a
// wiki_backlinks_cache_invalidation_log row tagged with op="acl_change".
//
// Strategy:
//
//   - small KB (CountByKB ≤ aclChangeCacheThreshold): DeleteByKB
//     (single DELETE on (kb_id, slug) PK range). Sub-millisecond on
//     every supported dialect. Wipe strategy label: "full".
//   - large KB (CountByKB > aclChangeCacheThreshold): FindReferencingSlugs
//     then Delete. Wipe strategy label: "reverse-lookup". The
//     histogram metricCacheAclChangeWipeDuration records the cost.
//
// Failure mode is consistent with the rest of the cache layer: every
// step warn-logs and continues. Losing one wipe means the next read
// recomputes on miss — the system self-heals.
//
// Build #27 — when noop is true the hook short-circuits before any
// cache work. This is the optimization the snapshot-hash column buys:
// an identical PutAcl (re-submit, idempotent retry, double-click) no
// longer triggers CountByKB + Delete. The caller is responsible for
// incrementing metricAclChangeSkippedTotal so the dashboards still
// observe the skip.
// invalidateBacklinksCacheOnAclChange is wired off PutAcl and never
// called directly.
func (s *wikiAclService) invalidateBacklinksCacheOnAclChange(
	ctx context.Context,
	kbID, slug string,
	beforeMode, afterMode string,
	beforeRev int64, afterRev int64,
	noop bool,
) {
	if s.cacheRepo == nil {
		return
	}
	if noop {
		// Spec D6 — invalidation-log row is intentionally not written
		// when the wipe is skipped; the log is the "did we actually
		// wipe" trail, and nothing was wiped. The audit row in
		// wiki_page_acl_audit carries the action="noop_match" signal
		// for forensics, so no information is lost.
		return
	}

	start := time.Now()
	var strategy string
	var affected int64
	var err error

	rowCount, countErr := s.cacheRepo.CountByKB(ctx, kbID)
	if countErr != nil {
		logger.Warnf(ctx, "wiki acl change hook: count by kb=%s failed: %v", kbID, countErr)
		// Fall through to the small-KB branch with a synthetic 0 count —
		// the DELETE will just no-op if there's actually a lot of data,
		// which is fine for self-healing.
		rowCount = 0
	}
	if rowCount <= aclChangeCacheThreshold {
		strategy = "full"
		affected, err = s.cacheRepo.DeleteByKB(ctx, kbID)
	} else {
		strategy = "reverse-lookup-indexed"
		refSlugs, lookupErr := s.cacheRepo.FindReferencingSlugs(ctx, kbID, slug)
		if lookupErr != nil {
			logger.Warnf(ctx, "wiki acl change hook: find referencing slugs failed (kb=%s slug=%s): %v", kbID, slug, lookupErr)
			metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange)).Inc()
			s.logAclChange(ctx, kbID, slug, beforeMode, afterMode, beforeRev, afterRev, strategy, 0)
			return
		}
		// Always include the affected slug itself; the reverse-lookup may
		// or may not surface it depending on whether the row's payload
		// arrays reference the slug. Dedup keeps the IN clause short.
		slugSet := make(map[string]struct{}, len(refSlugs)+1)
		slugSet[slug] = struct{}{}
		for _, s := range refSlugs {
			slugSet[s] = struct{}{}
		}
		deduped := make([]string, 0, len(slugSet))
		for s := range slugSet {
			deduped = append(deduped, s)
		}
		affected, err = s.cacheRepo.Delete(ctx, kbID, deduped)
		// Histogram: only the large-KB path records a duration because
		// it can be costly and is the one operators want to alert on.
		metricCacheAclChangeWipeDuration.Observe(time.Since(start).Seconds())
	}
	if err != nil {
		logger.Warnf(ctx, "wiki acl change hook: wipe failed (kb=%s slug=%s strategy=%s): %v", kbID, slug, strategy, err)
		metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange)).Inc()
		s.logAclChange(ctx, kbID, slug, beforeMode, afterMode, beforeRev, afterRev, strategy, 0)
		return
	}

	metricCacheInvalidationsTotal.WithLabelValues(string(types.BacklinkCacheInvalidateAclChange)).Inc()
	if affected > 0 {
		logger.Warnf(ctx, "wiki acl change hook wiped %d cache rows (kb=%s slug=%s strategy=%s)",
			affected, kbID, slug, strategy)
	}
	s.logAclChange(ctx, kbID, slug, beforeMode, afterMode, beforeRev, afterRev, strategy, int(affected))
}

// logAclChange writes the Build #23 invalidation log row with the
// ACL-specific Details JSON. Always best-effort: a failed log insert
// is warn-logged and otherwise swallowed.
func (s *wikiAclService) logAclChange(
	ctx context.Context, kbID, slug string,
	beforeMode, afterMode string,
	beforeRev, afterRev int64,
	strategy string, affected int,
) {
	detailsJSON, _ := json.Marshal(map[string]any{
		"before_mode":     beforeMode,
		"after_mode":      afterMode,
		"before_revision": beforeRev,
		"after_revision":  afterRev,
		"wipe_strategy":   strategy,
		"affected_count":  affected,
	})
	sourceEventID := wikiSourceEventIDFromContext(ctx)
	actorPtr := wikiActorUserIDFromContext(ctx)
	logEntry := &types.WikiBacklinksCacheInvalidationLogEntry{
		KbID:        kbID,
		Slug:        slug,
		Op:          string(types.BacklinkCacheInvalidateAclChange),
		ActorUserID: actorPtr,
		// Renamed from SourceEventID in Build #25 — column is now
		// `correlation_id` to match the 4-source audit join key. The
		// helper above still returns the X-Request-ID from middleware
		// so the meaning is unchanged.
		CorrelationID: sourceEventID,
		AffectedCount: affected,
		Details:       string(detailsJSON),
	}
	if logErr := s.cacheRepo.LogInvalidation(ctx, logEntry); logErr != nil {
		logger.Warnf(ctx, "wiki acl change hook: invalidation log insert failed (kb=%s slug=%s): %v", kbID, slug, logErr)
	}
}

// SearchAclCandidates delegates to the existing user search endpoint so the
// ACL dialog's user picker keeps the same tenant-scoped behaviour as the
// rest of the app.
func (s *wikiAclService) SearchAclCandidates(ctx context.Context, tenantID uint64, query string, limit int) ([]*types.User, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if s.userSvc == nil {
		return []*types.User{}, nil
	}
	return s.userSvc.SearchUsers(ctx, query, limit)
}

// ResolveBulk fans Resolve out across the given items with a small worker
// pool. The map key is `kbID:slug` (URL-friendly, no `|` collision with the
// single-hit cache key). Per-hit errors are logged and mapped to
// `deny_allow_list` so a transient failure on one page never leaks the
// hit to the caller. The returned error is non-nil only when the caller's
// context was cancelled before any work happened — every other error is
// absorbed into the map.
func (s *wikiAclService) ResolveBulk(ctx context.Context, items []AclResolveItem, callerUserID string) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(items))

	type job struct {
		kbID string
		slug string
		key  string
	}
	jobs := make([]job, len(items))
	for i, it := range items {
		jobs[i] = job{kbID: it.KBID, slug: it.Slug, key: it.KBID + ":" + it.Slug}
	}

	workers := aclResolveBulkWorkers
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers < 1 {
		workers = 1
	}

	queue := make(chan job, len(jobs))
	for _, j := range jobs {
		queue <- j
	}
	close(queue)

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if ctx.Err() != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = ctx.Err()
					}
					errMu.Unlock()
					continue
				}
				decision, err := s.Resolve(ctx, j.kbID, j.slug, callerUserID)
				if err != nil {
					logger.Warnf(ctx, "wiki acl resolve bulk: kb=%s slug=%s: %v", j.kbID, j.slug, err)
					out[j.key] = types.WikiPageAclDenyAllowList
					continue
				}
				out[j.key] = decision
			}
		}()
	}
	wg.Wait()
	return out, firstErr
}

// actionForMode picks a short audit label from the new mode. Used both as
// the audit row's `action` and as a tag the frontend can show in the
// activity log.
func actionForMode(mode string) string {
	switch mode {
	case types.WikiPageAclModeInherit:
		return "set_inherit"
	case types.WikiPageAclModePrivate:
		return "set_private"
	case types.WikiPageAclModeAllowList:
		return "set_allow_list"
	default:
		return "set_" + mode
	}
}

func containsString(haystack []string, needle string) bool {
	if needle == "" {
		return false
	}
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// Compile-time interface check.
var _ WikiAclService = (*wikiAclService)(nil)
