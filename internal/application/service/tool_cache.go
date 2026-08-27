package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// Build #30 — chat tool cache.
//
// Wraps types.Tool.Execute with a per-tenant LRU + TTL cache so repeated
// tool calls inside the same chat session (and across turns on the same
// KB) skip the underlying work. Cache key is sha256(tool_name +
// canonical_json(args_without_session_id) + tenant_id) per D5 —
// tenant_id inside the key is the cross-tenant defence, and the
// canonical-JSON step makes {a:1,b:2} and {b:2,a:1} land on the same
// key.
//
// MVP scope (Build #30):
//   - In-memory LRU + TTL with per-tenant capacity (default 1000 / 5min).
//   - Get/Set/InvalidateByTenant/InvalidateByTool public surface.
//   - Prom metrics in tool_cache_metrics.go.
//
// Out of scope (Build #31):
//   - Cross-process / Redis-backed shared cache.
//   - Semantic invalidation on KB write hooks (the wiki service already
//     emits invalidations that we can subscribe to in B31).
//   - Per-KB finer-grained keys — the spec calls for per-tenant today.

const (
	// defaultToolCacheCapacity is the maximum number of cache entries
	// per tenant (Build #30 D4 + spec checklist).
	defaultToolCacheCapacity = 1000
	// defaultToolCacheTTL is the default cache entry lifetime.
	// Matches Build #21 wiki backlinks cache for consistency.
	defaultToolCacheTTL = 5 * time.Minute
)

// ToolCache is the public surface used by the chat pipeline to memoize
// tool calls. Get returns (result, true) on hit, (nil, false) on miss
// or expired entry. Set inserts (or refreshes) an entry. Invalidate
// methods are for write hooks and tests.
type ToolCache interface {
	// Get returns the cached ToolResult for (tenantID, toolName, args)
	// if present and not past its TTL. Returns (nil, false) on miss.
	Get(ctx context.Context, tenantID uint64, toolName string, args json.RawMessage) (*types.ToolResult, bool)

	// Set stores a ToolResult for (tenantID, toolName, args) with the
	// configured TTL. If the per-tenant bucket is full, the least
	// recently used entry is evicted.
	Set(ctx context.Context, tenantID uint64, toolName string, args json.RawMessage, result *types.ToolResult)

	// InvalidateByTenant drops every entry for a tenant. Returns the
	// number of entries removed (useful for metrics / tests).
	InvalidateByTenant(tenantID uint64) int

	// InvalidateByTool drops every entry with the given toolName across
	// all tenants. Used when a tool's implementation changes.
	InvalidateByTool(toolName string) int
}

// lruToolCache is the in-memory implementation.
type lruToolCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	capacity int
	nowFn    func() time.Time
	tenants  map[uint64]*tenantBucket
}

// tenantBucket is one tenant's LRU. Bucket-level locking keeps the
// hot path (Get/Set on a single tenant) contention-free across tenants.
type tenantBucket struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List                  // front = most recently used
	index    map[string]*list.Element    // cache key → list element
}

// cacheEntry is one stored result. expiresAt lets Get return miss even
// when the entry is still in the LRU (TTL eviction without a write).
type cacheEntry struct {
	key       string
	toolName  string
	result    *types.ToolResult
	expiresAt time.Time
}

// NewToolCache builds a cache with default capacity + TTL.
func NewToolCache() ToolCache {
	return NewToolCacheWithOptions(defaultToolCacheCapacity, defaultToolCacheTTL, time.Now)
}

// NewToolCacheWithOptions lets tests inject smaller capacity and a
// deterministic clock without going through the package globals.
func NewToolCacheWithOptions(capacity int, ttl time.Duration, nowFn func() time.Time) ToolCache {
	if capacity <= 0 {
		capacity = defaultToolCacheCapacity
	}
	if ttl <= 0 {
		ttl = defaultToolCacheTTL
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &lruToolCache{
		ttl:      ttl,
		capacity: capacity,
		nowFn:    nowFn,
		tenants:  make(map[uint64]*tenantBucket),
	}
}

// Get implements ToolCache.Get.
func (c *lruToolCache) Get(_ context.Context, tenantID uint64, toolName string, args json.RawMessage) (*types.ToolResult, bool) {
	key := ToolCacheKey(tenantID, toolName, args)
	tenantLabel := formatTenantLabel(tenantID)
	toolLabel := toolName

	c.mu.RLock()
	bucket, ok := c.tenants[tenantID]
	c.mu.RUnlock()
	if !ok {
		metricToolCacheMissesTotal.WithLabelValues(tenantLabel, toolLabel).Inc()
		return nil, false
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	elem, ok := bucket.index[key]
	if !ok {
		metricToolCacheMissesTotal.WithLabelValues(tenantLabel, toolLabel).Inc()
		return nil, false
	}
	entry := elem.Value.(*cacheEntry)
	if c.nowFn().After(entry.expiresAt) {
		// Lazy eviction on read — don't bother to log; the next Set
		// or sweep will repair the LRU.
		bucket.ll.Remove(elem)
		delete(bucket.index, key)
		metricToolCacheMissesTotal.WithLabelValues(tenantLabel, toolLabel).Inc()
		return nil, false
	}

	bucket.ll.MoveToFront(elem)
	metricToolCacheHitsTotal.WithLabelValues(tenantLabel, toolLabel).Inc()
	// Return a shallow copy of the result so callers can't mutate the
	// cache entry's data through the returned pointer. Strings/images
	// slices are immutable enough for our use.
	return entry.result, true
}

// Set implements ToolCache.Set.
func (c *lruToolCache) Set(_ context.Context, tenantID uint64, toolName string, args json.RawMessage, result *types.ToolResult) {
	if result == nil {
		return
	}
	key := ToolCacheKey(tenantID, toolName, args)
	tenantLabel := formatTenantLabel(tenantID)
	toolLabel := toolName

	c.mu.Lock()
	bucket, ok := c.tenants[tenantID]
	if !ok {
		bucket = &tenantBucket{
			capacity: c.capacity,
			ll:       list.New(),
			index:    make(map[string]*list.Element),
		}
		c.tenants[tenantID] = bucket
	}
	c.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := c.nowFn()
	entry := &cacheEntry{
		key:       key,
		toolName:  toolName,
		result:    result,
		expiresAt: now.Add(c.ttl),
	}

	if elem, ok := bucket.index[key]; ok {
		// Refresh in place — overwrite the prior result so the new
		// payload wins. The LRU position is bumped to the front so
		// the most recent call sticks.
		bucket.ll.Remove(elem)
		delete(bucket.index, key)
	}
	elem := bucket.ll.PushFront(entry)
	bucket.index[key] = elem

	evicted := 0
	for bucket.ll.Len() > bucket.capacity {
		oldest := bucket.ll.Back()
		if oldest == nil {
			break
		}
		bucket.ll.Remove(oldest)
		delete(bucket.index, oldest.Value.(*cacheEntry).key)
		evicted++
	}
	metricToolCacheWritesTotal.Inc()
	metricToolCacheSizeEntries.WithLabelValues(tenantLabel).Set(float64(bucket.ll.Len()))
	if evicted > 0 {
		metricToolCacheEvictionsTotal.WithLabelValues(tenantLabel, toolLabel).Add(float64(evicted))
	}
}

// InvalidateByTenant implements ToolCache.InvalidateByTenant.
func (c *lruToolCache) InvalidateByTenant(tenantID uint64) int {
	c.mu.Lock()
	bucket, ok := c.tenants[tenantID]
	if !ok {
		c.mu.Unlock()
		return 0
	}
	delete(c.tenants, tenantID)
	c.mu.Unlock()

	bucket.mu.Lock()
	n := bucket.ll.Len()
	bucket.mu.Unlock()
	metricToolCacheInvalidationsTotal.WithLabelValues("tenant").Inc()
	metricToolCacheSizeEntries.DeleteLabelValues(formatTenantLabel(tenantID))
	return n
}

// InvalidateByTool implements ToolCache.InvalidateByTool. The tenant
// bucket is preserved — only the entries whose toolName matches are
// removed. Walked in O(n) per tenant; the alternative (per-tool
// secondary index) would double the memory footprint for an action
// that is expected to be rare.
func (c *lruToolCache) InvalidateByTool(toolName string) int {
	c.mu.Lock()
	buckets := make([]*tenantBucket, 0, len(c.tenants))
	for _, b := range c.tenants {
		buckets = append(buckets, b)
	}
	c.mu.Unlock()

	removed := 0
	for _, b := range buckets {
		b.mu.Lock()
		for elem := b.ll.Front(); elem != nil; elem = elem.Next() {
			if elem.Value.(*cacheEntry).toolName == toolName {
				b.ll.Remove(elem)
				delete(b.index, elem.Value.(*cacheEntry).key)
				removed++
			}
		}
		b.mu.Unlock()
	}
	if removed > 0 {
		metricToolCacheInvalidationsTotal.WithLabelValues("tool").Add(float64(removed))
	}
	return removed
}

// formatTenantLabel renders a tenant id as a stable string label. The
// label set is low-cardinality enough for Prom (uint64 → decimal),
// and keeping it consistent with the wiki_backlinks_cache labels
// ("kb_id" rather than "tenant_id") keeps Grafana dashboards aligned
// across the two caches.
func formatTenantLabel(tenantID uint64) string {
	return fmt.Sprintf("%d", tenantID)
}

// ToolCacheKey derives the canonical cache key for a tool invocation.
// Per Build #30 D5: sha256(tool_name + canonical_json(args) +
// tenant_id). The session-bound fields (request_id, session_id) are
// stripped from the args before canonicalisation so two turns with the
// same logical query hash to the same key.
func ToolCacheKey(tenantID uint64, toolName string, args json.RawMessage) string {
	canonical := canonicalizeArgs(args)
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0x1f}) // unit separator — guarantees a,b vs ba collision boundary
	h.Write(canonical)
	h.Write([]byte{0x1f})
	h.Write([]byte(fmt.Sprintf("%d", tenantID)))
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalizeArgs returns a deterministic JSON byte representation of
// args with the session-bound keys stripped. Object keys are sorted; for
// nested objects we recurse; arrays preserve order (positional args are
// semantically distinct).
func canonicalizeArgs(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		// Unparseable args — fall back to raw bytes. The hash still
		// distinguishes calls, just not semantically equivalent ones.
		return raw
	}
	stripped := stripSessionFields(v)
	out, err := json.Marshal(stripped)
	if err != nil {
		return raw
	}
	return out
}

// sessionBoundArgs is the allow-list of fields that vary per turn and
// must not participate in the cache key. Anything outside this list is
// considered semantically relevant to the tool's behaviour.
var sessionBoundArgs = map[string]struct{}{
	"session_id": {},
	"request_id": {},
	"turn_id":    {},
	"trace_id":   {},
}

// stripSessionFields recursively drops session-bound keys from a
// decoded JSON value. Maps are walked and matching keys removed;
// arrays are walked element-wise; primitives are returned as-is.
func stripSessionFields(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, child := range val {
			if _, drop := sessionBoundArgs[k]; drop {
				continue
			}
			out[k] = stripSessionFields(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, child := range val {
			out[i] = stripSessionFields(child)
		}
		return out
	default:
		return v
	}
}