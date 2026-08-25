package handler

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryAwareness_NoteAndLoad(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabAwarenessStore()
	roomKey := "/kb-a/page"

	// Three clients touch the room in order: alice, bob, alice again.
	if err := store.NoteAwareness(roomKey, "alice", []byte(`{"u":{"name":"Alice","color":"#ff0"}}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.NoteAwareness(roomKey, "bob", []byte(`{"u":{"name":"Bob","color":"#0ff"}}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.NoteAwareness(roomKey, "alice", []byte(`{"u":{"name":"Alice","color":"#ff0"}}`)); err != nil {
		t.Fatal(err)
	}

	entries, err := store.LoadRecent(context.Background(), roomKey, 10, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (alice+bob), got %d", len(entries))
	}
	// Most recent first.
	if entries[0].UserID != "alice" {
		t.Errorf("most recent should be alice, got %s", entries[0].UserID)
	}
	if entries[1].UserID != "bob" {
		t.Errorf("older entry should be bob, got %s", entries[1].UserID)
	}
	if entries[0].DisplayName != "Alice" {
		t.Errorf("display_name = %q, want %q", entries[0].DisplayName, "Alice")
	}
	if entries[1].Color != "#0ff" {
		t.Errorf("color = %q, want %q", entries[1].Color, "#0ff")
	}
}

func TestInMemoryAwareness_ForgetRemovesEntry(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabAwarenessStore()
	roomKey := "/kb-a/page"
	if err := store.NoteAwareness(roomKey, "alice", []byte(`{"u":{"name":"Alice"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(context.Background(), roomKey, "alice"); err != nil {
		t.Fatal(err)
	}
	entries, err := store.LoadRecent(context.Background(), roomKey, 10, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after Forget, got %d", len(entries))
	}
}

func TestInMemoryAwareness_SweepExpiresOldEntries(t *testing.T) {
	t.Parallel()
	store := &InMemoryWikiCollabAwarenessStore{
		entries: make(map[string]map[string]*inMemAwarenessEntry),
		now:     func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) },
	}
	roomKey := "/kb-a/page"
	if err := store.NoteAwareness(roomKey, "alice", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	// Move "now" forward 25h and sweep.
	store.now = func() time.Time { return time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC) }
	cutoff := store.now().Add(-24 * time.Hour)
	n, err := store.Sweep(context.Background(), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected to sweep 1 entry, got %d", n)
	}
	entries, _ := store.LoadRecent(context.Background(), roomKey, 10, 24*time.Hour)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after sweep, got %d", len(entries))
	}
}

func TestInMemoryAwareness_LoadRecentRespectsLimitAndAge(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabAwarenessStore()
	roomKey := "/kb-a/page"
	// 5 entries: alice 5h ago, bob 4h ago, carol 3h ago, dave 2h ago, eve 1h ago.
	// We'll inject them with manual LastSeenAt.
	base := time.Now()
	store.mu.Lock()
	store.entries[roomKey] = map[string]*inMemAwarenessEntry{
		"alice": {ClientID: "alice", UserID: "alice", DisplayName: "Alice", LastSeenAt: base.Add(-5 * time.Hour)},
		"bob":   {ClientID: "bob", UserID: "bob", DisplayName: "Bob", LastSeenAt: base.Add(-4 * time.Hour)},
		"carol": {ClientID: "carol", UserID: "carol", DisplayName: "Carol", LastSeenAt: base.Add(-3 * time.Hour)},
		"dave":  {ClientID: "dave", UserID: "dave", DisplayName: "Dave", LastSeenAt: base.Add(-2 * time.Hour)},
		"eve":   {ClientID: "eve", UserID: "eve", DisplayName: "Eve", LastSeenAt: base.Add(-1 * time.Hour)},
	}
	store.mu.Unlock()

	// Limit=3 → most-recent 3: eve, dave, carol.
	entries, err := store.LoadRecent(context.Background(), roomKey, 3, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	wantOrder := []string{"eve", "dave", "carol"}
	for i, w := range wantOrder {
		if entries[i].UserID != w {
			t.Errorf("entry[%d] = %s, want %s", i, entries[i].UserID, w)
		}
	}

	// maxAge=3h → only carol, dave, eve.
	entries, err = store.LoadRecent(context.Background(), roomKey, 10, 3*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries within 3h, got %d", len(entries))
	}
}

func TestInMemoryAwareness_ForgetGCRoomOnEmpty(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabAwarenessStore()
	roomKey := "/kb-a/page"
	if err := store.NoteAwareness(roomKey, "alice", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(context.Background(), roomKey, "alice"); err != nil {
		t.Fatal(err)
	}
	store.mu.RLock()
	_, ok := store.entries[roomKey]
	store.mu.RUnlock()
	if ok {
		t.Fatal("room map entry should be deleted when last client leaves")
	}
}
