package service

import (
	"context"
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
type WikiAclRepo interface {
	// GetAclBySlug fetches just the acl column for a page. Returns
	// (nil, nil) when the row exists but acl is NULL (legacy inherit).
	GetAclBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error)
	// UpdateAclWithRevision writes a new ACL value after checking the
	// stored revision still matches expectedRevision. Returns
	// types.ErrWikiPageAclRevisionConflict on mismatch. Audit row is
	// written in the same transaction.
	UpdateAclWithRevision(ctx context.Context, kbID string, slug string,
		newAcl types.WikiPageAcl, expectedRevision int64,
		actorUserID string, actorRole string, action string) (*types.WikiPageAcl, error)
	// PageOwnerAndAdmin returns the page's owner user id and whether the
	// caller is a KB admin. Used by Resolve to short-circuit owner/admin
	// to allow before reading the allow_list.
	PageOwnerAndAdmin(ctx context.Context, kbID string, slug string, callerUserID string) (ownerID string, isAdmin bool, err error)
	// GroupMembers returns the union of user IDs belonging to any of the
	// given groups. Used by Resolve to expand allow_group_ids.
	GroupMembers(ctx context.Context, tenantID uint64, groupIDs []string) ([]string, error)
}

// WikiAclService is the single decision point for page-level ACL. Every
// wiki read path consults Resolve before returning content; private /
// allow_list mismatches are mapped to a "page not found" 404 by the caller
// so the page's existence is not leaked.
type WikiAclService interface {
	Resolve(ctx context.Context, kbID string, slug string, callerUserID string) (string, error)
	GetAcl(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error)
	PutAcl(ctx context.Context, kbID string, slug string,
		req types.WikiPageAclSaveRequest, callerUserID string, callerRole string) (*types.WikiPageAcl, error)
	SearchAclCandidates(ctx context.Context, tenantID uint64, query string, limit int) ([]*types.User, error)
}

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
	cache     *aclCache
}

// NewWikiAclService wires the service. userSvc is used for the ACL dialog's
// candidate picker (SearchAclCandidates).
func NewWikiAclService(repo WikiAclRepo, userSvc interfaces.UserService) WikiAclService {
	return &wikiAclService{repo: repo, userSvc: userSvc, cache: newAclCache()}
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
// this page across all users.
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
	action := actionForMode(mode)
	updated, err := s.repo.UpdateAclWithRevision(ctx, kbID, slug, types.WikiPageAcl{
		Mode:          mode,
		AllowUserIDs:  req.AllowUserIDs,
		AllowGroupIDs: req.AllowGroupIDs,
		DenyInherited: req.DenyInherited,
	}, req.BaseRevision, callerUserID, callerRole, action)
	if err != nil {
		if errors.Is(err, types.ErrWikiPageAclRevisionConflict) {
			return nil, err
		}
		return nil, err
	}
	s.cache.invalidatePrefix(kbID + "|" + slug + "|")
	return updated, nil
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