package service

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// fakeClock returns a deterministic time source so TTL tests don't race
// against wall-clock skew. Tests advance it via Advance().
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func mustRaw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func newSampleResult(payload string) *types.ToolResult {
	return &types.ToolResult{
		Success: true,
		Output:  payload,
		Data:    map[string]interface{}{"echo": payload},
	}
}

// TestToolCacheMissThenHit exercises the basic miss → set → hit path.
// Without D5's canonical-key derivation this test is the bare minimum
// the spec checklist requires.
func TestToolCacheMissThenHit(t *testing.T) {
	cache := NewToolCache()
	args := mustRaw(t, map[string]interface{}{"query": "foo"})
	result := newSampleResult("foo-data")

	_, ok := cache.Get(t.Context(), 1, "search", args)
	require.False(t, ok, "miss before Set")

	cache.Set(t.Context(), 1, "search", args, result)
	got, ok := cache.Get(t.Context(), 1, "search", args)
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, "foo-data", got.Output)
}

// TestToolCacheKeyIsOrderIndependent enforces D5: {a:1,b:2} and
// {b:2,a:1} must collide on the same cache key. Without canonical
// JSON, encoding/json's map iteration order would randomise the
// resulting bytes and the second call would miss.
func TestToolCacheKeyIsOrderIndependent(t *testing.T) {
	argsA := mustRaw(t, map[string]interface{}{"a": 1, "b": 2})
	argsB := mustRaw(t, map[string]interface{}{"b": 2, "a": 1})

	keyA := ToolCacheKey(7, "search", argsA)
	keyB := ToolCacheKey(7, "search", argsB)
	require.Equal(t, keyA, keyB, "key derivation must be canonical (D5)")
}

// TestToolCacheKeyIsTenantIsolated enforces D5's second clause: the
// same args for two different tenants must NOT collide. Without the
// tenant_id segment of the hash, a multi-tenant deploy would leak
// results across KBs.
func TestToolCacheKeyIsTenantIsolated(t *testing.T) {
	args := mustRaw(t, map[string]interface{}{"a": 1})
	require.NotEqual(t,
		ToolCacheKey(1, "search", args),
		ToolCacheKey(2, "search", args),
		"different tenants must hash to different keys",
	)
	require.NotEqual(t,
		ToolCacheKey(1, "search", args),
		ToolCacheKey(1, "other-tool", args),
		"different tools must hash to different keys",
	)
}

// TestToolCacheStripsSessionBoundFields verifies that adding a
// session_id / request_id to args does not bust the cache hit. A
// multi-turn chat pipeline passes these on every call, so without
// the strip the cache hit rate would collapse to zero.
func TestToolCacheStripsSessionBoundFields(t *testing.T) {
	argsBase := mustRaw(t, map[string]interface{}{"q": "hello"})
	argsWithSession := mustRaw(t, map[string]interface{}{"q": "hello", "session_id": "abc"})
	argsWithRequest := mustRaw(t, map[string]interface{}{"q": "hello", "request_id": "req-42"})

	keyBase := ToolCacheKey(1, "search", argsBase)
	keySession := ToolCacheKey(1, "search", argsWithSession)
	keyRequest := ToolCacheKey(1, "search", argsWithRequest)
	require.Equal(t, keyBase, keySession)
	require.Equal(t, keyBase, keyRequest)
}

// TestToolCacheTTLExpires verifies the TTL path: an entry inserted at
// t=0 must be retrievable before ttl elapses and gone after. The
// fake clock keeps the test deterministic.
func TestToolCacheTTLExpires(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cache := NewToolCacheWithOptions(10, 30*time.Second, clock.Now)
	args := mustRaw(t, map[string]interface{}{"q": "ttl"})

	cache.Set(t.Context(), 1, "search", args, newSampleResult("v1"))

	clock.Advance(29 * time.Second)
	_, ok := cache.Get(t.Context(), 1, "search", args)
	require.True(t, ok, "still within TTL — must be a hit")

	clock.Advance(2 * time.Second) // total 31s > 30s TTL
	_, ok = cache.Get(t.Context(), 1, "search", args)
	require.False(t, ok, "after TTL — must be a miss")
}

// TestToolCacheLRUEvictsOldest exercises the LRU discipline: capacity
// is 3, fill 3, then ask for the first one again — it should miss,
// and the eviction counter must increment.
func TestToolCacheLRUEvictsOldest(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cache := NewToolCacheWithOptions(3, time.Minute, clock.Now)

	mk := func(i int) (json.RawMessage, *types.ToolResult) {
		args := mustRaw(t, map[string]interface{}{"i": i})
		return args, newSampleResult(string(rune('a' + i)))
	}

	for i := 0; i < 3; i++ {
		args, r := mk(i)
		cache.Set(t.Context(), 1, "search", args, r)
	}

	// Touch entries 1 and 2 so that "a" is the LRU tail.
	args1, _ := mk(1)
	_, ok := cache.Get(t.Context(), 1, "search", args1)
	require.True(t, ok)

	args2, _ := mk(2)
	_, ok = cache.Get(t.Context(), 1, "search", args2)
	require.True(t, ok)

	// Insert a 4th entry — "a" must be evicted to keep capacity at 3.
	args3, r3 := mk(3)
	cache.Set(t.Context(), 1, "search", args3, r3)

	args0, _ := mk(0)
	_, ok = cache.Get(t.Context(), 1, "search", args0)
	require.False(t, ok, "oldest entry must be evicted under capacity pressure")

	args1, _ = mk(1)
	_, ok = cache.Get(t.Context(), 1, "search", args1)
	require.True(t, ok, "touched entries must remain")
}

// TestToolCacheTenantIsolation verifies that InvalidateByTenant only
// touches the addressed tenant's bucket. Two tenants inserted side by
// side; invalidate tenant 1; tenant 2's entries must still hit.
func TestToolCacheTenantIsolation(t *testing.T) {
	cache := NewToolCache()
	args := mustRaw(t, map[string]interface{}{"q": "shared-args"})

	cache.Set(t.Context(), 1, "search", args, newSampleResult("v1"))
	cache.Set(t.Context(), 2, "search", args, newSampleResult("v2"))

	removed := cache.InvalidateByTenant(1)
	require.Equal(t, 1, removed)

	_, ok := cache.Get(t.Context(), 1, "search", args)
	require.False(t, ok, "tenant 1 entry must be gone")
	_, ok = cache.Get(t.Context(), 2, "search", args)
	require.True(t, ok, "tenant 2 entry must survive")
}

// TestToolCacheInvalidateByTool verifies that a per-tool invalidation
// drops every entry for that tool across every tenant.
func TestToolCacheInvalidateByTool(t *testing.T) {
	cache := NewToolCache()
	argsSearch := mustRaw(t, map[string]interface{}{"q": "search-args"})
	argsOther := mustRaw(t, map[string]interface{}{"q": "other-args"})

	cache.Set(t.Context(), 1, "search", argsSearch, newSampleResult("s1"))
	cache.Set(t.Context(), 2, "search", argsSearch, newSampleResult("s2"))
	cache.Set(t.Context(), 1, "other", argsOther, newSampleResult("o1"))

	removed := cache.InvalidateByTool("search")
	require.Equal(t, 2, removed)

	_, ok := cache.Get(t.Context(), 1, "search", argsSearch)
	require.False(t, ok)
	_, ok = cache.Get(t.Context(), 2, "search", argsSearch)
	require.False(t, ok)
	_, ok = cache.Get(t.Context(), 1, "other", argsOther)
	require.True(t, ok, "untouched tool must survive")
}

// TestToolCacheSetRefreshesExistingEntry verifies the "refresh in
// place" semantics: a Set with the same key overwrites the prior
// value but does not duplicate it in the LRU. Without this the LRU
// would balloon under repeated identical calls.
func TestToolCacheSetRefreshesExistingEntry(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cache := NewToolCacheWithOptions(10, time.Minute, clock.Now)
	args := mustRaw(t, map[string]interface{}{"q": "refresh"})

	for i := 0; i < 5; i++ {
		cache.Set(t.Context(), 1, "search", args, newSampleResult(string(rune('a'+i))))
	}

	got, ok := cache.Get(t.Context(), 1, "search", args)
	require.True(t, ok)
	require.Equal(t, "e", got.Output, "last Set must win")
}

// TestToolCacheSetDropsNil is the defensive guard: Set(nil) must not
// insert a tombstone that later collides on hits.
func TestToolCacheSetDropsNil(t *testing.T) {
	cache := NewToolCache()
	args := mustRaw(t, map[string]interface{}{"q": "nil-guard"})

	cache.Set(t.Context(), 1, "search", args, nil)

	_, ok := cache.Get(t.Context(), 1, "search", args)
	require.False(t, ok, "Set(nil) must not store a sentinel entry")
}

// TestToolCacheConcurrentSafe is a smoke test for the locking: 100
// goroutines hammer Get/Set on the same key, and the cache must not
// race or lose updates. Run with -race to catch regressions.
func TestToolCacheConcurrentSafe(t *testing.T) {
	cache := NewToolCache()
	args := mustRaw(t, map[string]interface{}{"q": "race"})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Set(t.Context(), 1, "search", args, newSampleResult("v"))
			_, _ = cache.Get(t.Context(), 1, "search", args)
		}()
	}
	wg.Wait()
	got, ok := cache.Get(t.Context(), 1, "search", args)
	require.True(t, ok)
	require.NotNil(t, got)
}