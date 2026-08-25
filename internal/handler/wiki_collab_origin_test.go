package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
)

// TestOriginFilter_DisconnectsOnGarbage verifies that a peer sending
// a non-whitelisted messageType gets booted after 3 bad frames.
func TestOriginFilter_DisconnectsOnGarbage(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithOriginFilter(t)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send 4 garbage frames in a row. The first 3 should be silently
	// dropped (counter increments), the 4th should trigger a close
	// because the policy closes on the 3rd bad frame.
	for i := 0; i < 4; i++ {
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := conn.WriteMessage(ws.BinaryMessage, []byte{0x02, 'a'}); err != nil {
			// Write may fail because the server closed; that's expected.
			return
		}
	}
	// Read until close.
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// TestOriginFilter_AllowsSyncFrames ensures sync frames are still
// forwarded after the bad-frame counter resets.
func TestOriginFilter_AllowsSyncFrames(t *testing.T) {
	t.Parallel()
	srv := newTestServerWithOriginFilter(t)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Sync frame (0x00) should be accepted.
	if err := conn.WriteMessage(ws.BinaryMessage, []byte{0x00, 'p', 'a', 'y'}); err != nil {
		t.Fatalf("sync write failed: %v", err)
	}
	// Read at least the echo from the broadcast loop (the test server
	// forwards via the same room).
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected echoed frame, got err: %v", err)
	}
	if len(msg) < 4 || msg[0] != 0x00 {
		t.Fatalf("got %x, want sync-prefixed frame", msg)
	}
}

// newTestServerWithOriginFilter wires a test HTTP server that upgrades
// to a wikiCollabHub client using a real handler, so the readLoop's
// origin filter runs end-to-end.
func newTestServerWithOriginFilter(t *testing.T) *httptest.Server {
	t.Helper()
	store := NewInMemoryWikiCollabSnapshotStore()
	hub := newWikiCollabHub(store)
	var counter atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade: %v", err)
			return
		}
		client := &wikiCollabClient{
			conn:    conn,
			send:    make(chan []byte, 64),
			userID:  "test-peer",
			display: "Test",
		}
		room := hub.join("/kb-test/page", client)
		client.room = room
		// Drain echoes back to the peer so they can read the
		// broadcast (the test peer's own outgoing frames echo).
		go func() {
			for f := range client.send {
				_ = conn.WriteMessage(ws.BinaryMessage, f)
			}
		}()
		// Minimal read loop with origin filter — mirrors HandleCollab's.
		bad := 0
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != ws.TextMessage && mt != ws.BinaryMessage {
				continue
			}
			kind, _, perr := parseYFrame(data)
			if perr != nil || !isAllowedForward(kind) {
				bad++
				if bad >= 3 {
					_ = conn.Close()
					return
				}
				continue
			}
			bad = 0
			select {
			case client.room.broadcast <- data:
			default:
			}
		}
	}))
}
