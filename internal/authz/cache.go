package authz

import (
	"container/list"
	"strings"
	"sync"
)

// decisionLRU is a tiny LRU cache for composite-level decisions.
// The full relation walk is cheap (adapters are cached themselves)
// but the composite cache exists so a CheckBulk hit on the same
// (user, object, relation) tuple only pays for the lookup, not the
// cross-tenant check + adapter map lookup + cache write-back.
//
// The cache is safe for concurrent use.
type decisionLRU struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[string]*list.Element

	// objectIndex lets Invalidate drop every entry for an object
	// in O(k) where k is the number of cached (user, relation)
	// tuples for that object. Without it we would have to scan
	// the whole map.
	objectIndex map[string]map[string]struct{}
}

type decisionEntry struct {
	key   string
	obj   Object
	value Decision
}

func newDecisionLRU(capacity int) *decisionLRU {
	if capacity <= 0 {
		capacity = 1024
	}
	return &decisionLRU{
		capacity:    capacity,
		ll:          list.New(),
		items:       make(map[string]*list.Element, capacity),
		objectIndex: make(map[string]map[string]struct{}, capacity),
	}
}

func (l *decisionLRU) get(key string) (Decision, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[key]
	if !ok {
		return Decision{}, false
	}
	l.ll.MoveToFront(el)
	return el.Value.(*decisionEntry).value, true
}

func (l *decisionLRU) put(key string, d Decision) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.items[key]; ok {
		l.ll.MoveToFront(el)
		el.Value.(*decisionEntry).value = d
		return
	}
	entry := &decisionEntry{key: key, value: d}
	el := l.ll.PushFront(entry)
	l.items[key] = el
	l.indexAdd(entry)
	for l.ll.Len() > l.capacity {
		back := l.ll.Back()
		if back == nil {
			break
		}
		l.ll.Remove(back)
		old := back.Value.(*decisionEntry)
		delete(l.items, old.key)
		l.indexRemove(old)
	}
}

func (l *decisionLRU) invalidateByObject(obj Object) {
	l.mu.Lock()
	defer l.mu.Unlock()
	keys, ok := l.objectIndex[obj.String()]
	if !ok {
		return
	}
	for k := range keys {
		if el, ok := l.items[k]; ok {
			l.ll.Remove(el)
			delete(l.items, k)
		}
	}
	delete(l.objectIndex, obj.String())
}

func (l *decisionLRU) indexAdd(e *decisionEntry) {
	// Best-effort object extraction from the entry key. The key
	// format is "<user>|<object>|<relation>"; we split on "|" so
	// we can group by Object without parsing the request itself.
	parts := strings.SplitN(e.key, "|", 3)
	if len(parts) != 3 {
		return
	}
	objStr := parts[1]
	// We deliberately do not parse Object from the string — the
	// composite caller has the Object on hand and passes it in
	// via Invalidate. The index is a cache-coherence hint; if
	// we cannot reconstruct we just skip.
	if _, ok := l.objectIndex[objStr]; !ok {
		l.objectIndex[objStr] = make(map[string]struct{}, 4)
	}
	l.objectIndex[objStr][e.key] = struct{}{}
}

func (l *decisionLRU) indexRemove(e *decisionEntry) {
	parts := strings.SplitN(e.key, "|", 3)
	if len(parts) != 3 {
		return
	}
	if m, ok := l.objectIndex[parts[1]]; ok {
		delete(m, e.key)
		if len(m) == 0 {
			delete(l.objectIndex, parts[1])
		}
	}
}

// Len returns the current number of cached entries. Tests use this
// to verify invalidation actually evicted what we asked for.
func (l *decisionLRU) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ll.Len()
}
