package dlp

import (
	"sync"
	"time"
)

// ruleCache memoises the compiled-rule set per tenant so repeated
// scans on the same tenant don't recompile regexes. Entries expire
// after ruleCacheTTL.
//
// v0.7.22: in-process only. Production should swap to Redis so the
// cache is shared across replicas (AuthZ phase-3 already plumbs
// Redis pub/sub for similar reasons).
type ruleCache struct {
	mu      sync.RWMutex
	entries map[uint64]ruleCacheEntry
}

type ruleCacheEntry struct {
	rules    []policyRule
	cachedAt time.Time
}

const ruleCacheTTL = 1 * time.Minute

func newRuleCache() *ruleCache {
	return &ruleCache{entries: map[uint64]ruleCacheEntry{}}
}

func (c *ruleCache) get(tenantID uint64) ([]policyRule, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[tenantID]
	if !ok {
		return nil, false
	}
	if time.Since(e.cachedAt) > ruleCacheTTL {
		return nil, false
	}
	return e.rules, true
}

func (c *ruleCache) set(tenantID uint64, rules []policyRule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[tenantID] = ruleCacheEntry{rules: rules, cachedAt: time.Now()}
}

func (c *ruleCache) invalidate(tenantID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, tenantID)
}
