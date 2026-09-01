package authz

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// Adapter is the per-object-type policy plug-in. Each registered
// adapter answers Check calls for a specific ObjectType. The composite
// dispatches by ObjectType; adapters must be safe for concurrent use.
type Adapter interface {
	// ObjectType returns the ObjectType this adapter handles. Used
	// by the composite for dispatch. Must be unique per adapter.
	ObjectType() ObjectType

	// Check answers the request when Object.Type matches
	// ObjectType(). Adapters may consult caches and the
	// underlying services. Returning Decision{} (zero value) is
	// treated as Deny(CodeNoRelation) — adapters should always
	// return a fully-populated Decision so Source/Message are
	// useful in audit logs.
	Check(ctx context.Context, req CheckRequest) Decision

	// Invalidate drops any cached state for the given object.
	// The composite forwards Invalidate calls to the matching
	// adapter only.
	Invalidate(ctx context.Context, obj Object) error
}

// CompositeChecker is the default Checker implementation. It owns
// the adapter registry, the bulk fan-out worker pool, and a tiny
// decision cache.
//
// The cache is intentionally minimal (no TTL, just per-instance
// invalidation by Object pointer) because each adapter is expected
// to maintain its own richer cache (Wiki ACL has a 60s TTL today;
// KB Share is a no-op cache). The composite-level cache is only a
// "I just answered this, do not re-walk the adapter chain" hint.
type CompositeChecker struct {
	mu       sync.RWMutex
	adapters map[ObjectType]Adapter

	// bulkWorkers bounds CheckBulk fan-out. 8 is a safe default
	// for both PostgreSQL and SQLite backed services; the
	// underlying adapter caches absorb the rest.
	bulkWorkers int

	// decisionCache is keyed by the request tuple (user, object,
	// relation). Nil when caching is disabled (tests).
	decisionCache *decisionLRU
}

// NewCompositeChecker builds a Checker with the given adapters. Pass
// adapters in any order. Duplicate ObjectTypes are tolerated — the
// first adapter for each ObjectType wins. The TupleAdapter
// intentionally reuses ObjectTypeTenant so it can sit alongside the
// TenantRoleAdapter without forcing the composite to special-case
// fallthrough dispatch; production wiring puts the role adapter
// first so it stays authoritative.
func NewCompositeChecker(adapters ...Adapter) *CompositeChecker {
	c := &CompositeChecker{
		adapters:      make(map[ObjectType]Adapter, len(adapters)),
		bulkWorkers:   8,
		decisionCache: newDecisionLRU(4096),
	}
	for _, a := range adapters {
		// Duplicate ObjectTypes are tolerated: the first adapter
		// wins. The TupleAdapter intentionally reuses
		// ObjectTypeTenant as a sentinel so it can be looked up
		// alongside the role adapter without the composite
		// framework refusing to register it. Keeping the first
		// registration preserves the documented ranking
		// ("ranked below the in-memory adapters").
		ot := a.ObjectType()
		if _, exists := c.adapters[ot]; !exists {
			c.adapters[ot] = a
		}
	}
	return c
}

// Register adds an adapter after construction. Intended for tests and
// for plugin-style opt-ins; production wiring registers everything
// in NewCompositeChecker. Duplicate ObjectTypes are ignored — the
// first registration wins — see NewCompositeChecker for rationale.
func (c *CompositeChecker) Register(a Adapter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ot := a.ObjectType()
	if _, exists := c.adapters[ot]; !exists {
		c.adapters[ot] = a
	}
}

// DisableCache turns off the composite-level decision cache. Tests
// that exercise invalidation need this; production leaves it on.
func (c *CompositeChecker) DisableCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decisionCache = nil
}

// Check is the primary entry point. Cache lookup runs first; then
// adapter dispatch; then cache write-back. Cross-tenant handling
// lives inside each adapter because KB / Agent / WikiPage must
// consult their cross-tenant share / ACL layer before denying;
// the TenantRole / Notification / ChatMessage adapters re-check
// cross-tenant themselves and deny conservatively.
func (c *CompositeChecker) Check(ctx context.Context, req CheckRequest) Decision {
	if key := c.cacheKey(req); key != "" {
		if d, ok := c.lookupCache(key); ok {
			return d
		}
	}
	c.mu.RLock()
	a, ok := c.adapters[req.Object.Type]
	c.mu.RUnlock()
	if !ok {
		return Deny(CodeNoSuchAdapter, "composite",
			"no adapter registered for object type "+string(req.Object.Type))
	}
	d := a.Check(ctx, req)
	if key := c.cacheKey(req); key != "" {
		c.storeCache(key, d)
	}
	return d
}

// CheckBulk fans out Check across many requests. Order of returned
// decisions matches the order of input requests. Bounded worker
// pool avoids goroutine blow-up on huge search-result lists.
func (c *CompositeChecker) CheckBulk(ctx context.Context, reqs []CheckRequest) []Decision {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]Decision, len(reqs))
	workers := c.bulkWorkers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(reqs) {
		workers = len(reqs)
	}
	type job struct{ idx int; req CheckRequest }
	jobs := make(chan job, len(reqs))
	for i, r := range reqs {
		jobs <- job{idx: i, req: r}
	}
	close(jobs)
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				out[j.idx] = c.Check(ctx, j.req)
			}
		}()
	}
	wg.Wait()
	return out
}

// Invalidate drops the composite cache entry for every (user,
// relation) tuple attached to the object and forwards to the
// matching adapter. Returns nil if no adapter is registered so
// callers in tests can fan out invalidations without nil-checking.
func (c *CompositeChecker) Invalidate(ctx context.Context, obj Object) error {
	if c.decisionCache != nil {
		c.decisionCache.invalidateByObject(obj)
	}
	c.mu.RLock()
	a, ok := c.adapters[obj.Type]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	return a.Invalidate(ctx, obj)
}

// cacheKey returns a stable string for the (user, object, relation,
// role, capabilities) tuple so CheckBulk + Check share the cache.
// User.String() omits Role and Capabilities because they are
// explainability metadata, but the cache must distinguish them —
// the same (user, object, relation) with different roles MUST
// produce different keys so a role bump on the hot path is observed.
// Empty string disables caching for the request.
func (c *CompositeChecker) cacheKey(req CheckRequest) string {
	if c.decisionCache == nil {
		return ""
	}
	key := req.User.String() + "|" + req.Object.String() + "|" + string(req.Relation)
	if req.User.Role != "" {
		key += "|role=" + req.User.Role
	}
	if len(req.User.Capabilities) > 0 {
		key += "|caps="
		// Stable order — caller may pass capabilities in any order.
		sorted := make([]string, len(req.User.Capabilities))
		copy(sorted, req.User.Capabilities)
		sort.Strings(sorted)
		key += strings.Join(sorted, ",")
	}
	return key
}

func (c *CompositeChecker) lookupCache(key string) (Decision, bool) {
	if c.decisionCache == nil {
		return Decision{}, false
	}
	return c.decisionCache.get(key)
}

func (c *CompositeChecker) storeCache(key string, d Decision) {
	if c.decisionCache == nil {
		return
	}
	c.decisionCache.put(key, d)
}


// StableReasonOrder is a small helper for tests + admin UI:
// sort a set of Decisions so the most "specific" deny wins.
// Returns the first Decision in the sorted order.
func StableReasonOrder(decisions []Decision) Decision {
	if len(decisions) == 0 {
		return Deny(CodeNoRelation, "composite", "no decisions to order")
	}
	sort.SliceStable(decisions, func(i, j int) bool {
		return decisionSeverity(decisions[i]) < decisionSeverity(decisions[j])
	})
	return decisions[0]
}

// decisionSeverity ranks decisions so the most specific answer
// surfaces. Allows always win (lowest number). Among denies, more
// specific codes beat more general ones; CodeError is treated as
// the most specific (a real underlying failure we want surfaced).
func decisionSeverity(d Decision) int {
	if d.Allowed {
		return 0
	}
	switch d.Code {
	case CodeError:
		return 1
	case CodeNoSuchResource:
		return 2
	case CodeOwnerOnly:
		return 3
	case CodeNotShared:
		return 4
	case CodeRoleTooLow:
		return 5
	case CodeWrongTenant:
		return 6
	case CodeNoRelation:
		return 7
	case CodeNoSuchAdapter:
		return 8
	}
	return 9
}
