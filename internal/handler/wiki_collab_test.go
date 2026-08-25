package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

// TestWikiCollabHubFanout verifies that two clients connected to the
// same room receive each other's frames. We don't drive the hub through
// the full handler (which requires a real JWT) — instead we construct
// the hub directly, attach two fake clients, and observe fan-out.
//
// Run with:
//   cd /root/multica_workspaces/.../WeKnora && go test ./internal/handler/ -run TestWikiCollabHubFanout -count=1
//
// This test is a Build #8 prototype harness. Sandbox CI does not have a
// Go toolchain, so it is committed as a developer-local runbook rather
// than an enforced gate.
func TestWikiCollabHubFanout(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabSnapshotStore()
	hub := newWikiCollabHub(store)

	roomKey := wikiCollabRoomKey("kb-abc", "guides/runbook")

	a := newTestClient(t, hub, roomKey, "alice")
	b := newTestClient(t, hub, roomKey, "bob")
	t.Cleanup(func() {
		a.conn.Close()
		b.conn.Close()
	})

	// Wait for join to settle.
	time.Sleep(50 * time.Millisecond)

	payload := []byte("hello-from-alice")
	if err := a.conn.WriteMessage(ws.BinaryMessage, payload); err != nil {
		t.Fatalf("alice write failed: %v", err)
	}

	got := readFrameWithin(t, b, 2*time.Second)
	if !bytes.Equal(got, payload) {
		t.Fatalf("bob received %q, want %q", got, payload)
	}

	// Alice must NOT receive her own frame (y-websocket avoids echo).
	select {
	case echo := <-a.send:
		t.Fatalf("alice unexpectedly echoed her own frame: %q", echo)
	case <-time.After(200 * time.Millisecond):
	}

	// Snapshot store should now contain the broadcast frame.
	snap, err := store.LoadLatest(context.Background(), roomKey)
	if err != nil {
		t.Fatalf("snapshot load: %v", err)
	}
	if !bytes.Contains(snap, payload) {
		t.Fatalf("snapshot %q missing broadcast payload", snap)
	}
}

// TestWikiCollabRoomGCAfterLastLeave verifies that the hub removes the
// room entry once the last client exits. This is the precondition for
// the final snapshot flush.
func TestWikiCollabRoomGCAfterLastLeave(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabSnapshotStore()
	hub := newWikiCollabHub(store)
	roomKey := wikiCollabRoomKey("kb-xyz", "page")

	a := newTestClient(t, hub, roomKey, "alice")
	b := newTestClient(t, hub, roomKey, "bob")

	time.Sleep(30 * time.Millisecond)
	hub.mu.RLock()
	if _, ok := hub.rooms[roomKey]; !ok {
		hub.mu.RUnlock()
		t.Fatalf("expected room %s to exist after joins", roomKey)
	}
	hub.mu.RUnlock()

	a.conn.Close()
	b.conn.Close()

	// The room's run() goroutine closes the broadcast channel when the
	// last client leaves; that's our GC signal. Wait briefly.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		room, ok := hub.rooms[roomKey]
		hub.mu.RUnlock()
		if !ok {
			return
		}
		// Room still present but maybe closing.
		room.mu.Lock()
		closed := room.closed
		room.mu.Unlock()
		if closed && len(room.clients) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("room %s was not GC'd after last client left", roomKey)
}

// TestWikiCollabRoomIsolation verifies that two rooms don't cross-talk.
func TestWikiCollabRoomIsolation(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabSnapshotStore()
	hub := newWikiCollabHub(store)

	a := newTestClient(t, hub, wikiCollabRoomKey("kb-iso", "page-a"), "alice")
	b := newTestClient(t, hub, wikiCollabRoomKey("kb-iso", "page-b"), "bob")

	t.Cleanup(func() {
		a.conn.Close()
		b.conn.Close()
	})
	time.Sleep(50 * time.Millisecond)

	payload := []byte("page-a-update")
	if err := a.conn.WriteMessage(ws.BinaryMessage, payload); err != nil {
		t.Fatalf("alice write: %v", err)
	}
	select {
	case got := <-b.send:
		t.Fatalf("bob (different room) unexpectedly received %q", got)
	case <-time.After(250 * time.Millisecond):
	}
}

// TestWikiCollabSnapshotReplay ensures a late joiner receives the
// accumulated buffer before any live frames.
func TestWikiCollabSnapshotReplay(t *testing.T) {
	t.Parallel()
	store := NewInMemoryWikiCollabSnapshotStore()
	hub := newWikiCollabHub(store)
	roomKey := wikiCollabRoomKey("kb-replay", "page")

	a := newTestClient(t, hub, roomKey, "alice")
	t.Cleanup(func() { a.conn.Close() })
	time.Sleep(30 * time.Millisecond)

	if _, err := a.conn.WriteMessage(ws.BinaryMessage, []byte("frame-1")); err != nil {
		t.Fatalf("write frame-1: %v", err)
	}
	readFrameWithin(t, a, time.Second) // drain own frame from a.send if echoed
	hub.snapshots.FlushNow(context.Background(), roomKey)

	b := newTestClient(t, hub, roomKey, "bob")
	t.Cleanup(func() { b.conn.Close() })
	got := readFrameWithin(t, b, 2*time.Second)
	if !bytes.Contains(got, []byte("frame-1")) {
		t.Fatalf("late joiner did not receive replay buffer; got %q", got)
	}
}

// --- harness helpers --------------------------------------------------------

func newTestClient(t *testing.T, hub *wikiCollabHub, roomKey, userID string) *wikiCollabClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		c := &wikiCollabClient{
			conn:    conn,
			send:    make(chan []byte, wikiCollabSendQueueSize),
			userID:  userID,
			display: userID,
		}
		room := hub.join(roomKey, c)
		c.room = room
		// Drain anything the hub sends us (replay + future broadcasts)
		// into the test's send channel so readFrameWithin can pick it up.
		go func() {
			for frame := range c.send {
				_ = frame
			}
		}()
		// Read loop: forward incoming frames into room broadcast.
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := ws.DefaultDialer
	dialer.HandshakeTimeout = 2 * time.Second
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	t.Cleanup(func() { conn.Close() })

	c := &wikiCollabClient{
		conn:    conn,
		send:    make(chan []byte, wikiCollabSendQueueSize),
		userID:  userID,
		display: userID,
	}
	room := hub.join(roomKey, c)
	c.room = room

	// Drain the inbound WS connection into the test's send channel.
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				close(c.send)
				return
			}
			select {
			case c.send <- data:
			default:
			}
		}
	}()
	return c
}

func readFrameWithin(t *testing.T, c *wikiCollabClient, d time.Duration) []byte {
	t.Helper()
	select {
	case frame, ok := <-c.send:
		if !ok {
			t.Fatalf("send channel closed before frame arrived")
		}
		return frame
	case <-time.After(d):
		t.Fatalf("timed out after %v waiting for frame", d)
		return nil
	}
}

// helper kept silent; ensures gin route registers without warnings in tests.
func init() {
	gin.SetMode(gin.TestMode)
	var _ sync.Mutex
}
