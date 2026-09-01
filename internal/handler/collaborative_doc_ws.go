// Package handler — v0.7.25 collaborative_docs WebSocket bridge.
//
// Wire protocol: standard y-websocket binary frames (same as Wiki realtime
// in v0.7.19). The handler is structurally a sibling of WikiRealtimeWSHandler
// but is keyed on doc_id rather than (kb_id, page_id) and writes to the
// collab_doc_snapshots / collab_doc_sessions tables.
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// CollabDocRealtimeWSHandler is the Yjs bridge for collaborative docs.
type CollabDocRealtimeWSHandler struct {
	svc        *service.CollabDocService
	upgrader   websocket.Upgrader
	pages      sync.Map // docKey -> *collabDocWSHub
	maxMsgSize int64
}

// collabDocWSHubLimit mirrors the wiki realtime plan: 1 MiB max message.
const collabDocWSHubLimit int64 = 1 << 20

// NewCollabDocRealtimeWSHandler constructs the WS handler.
func NewCollabDocRealtimeWSHandler(svc *service.CollabDocService) *CollabDocRealtimeWSHandler {
	return &CollabDocRealtimeWSHandler{
		svc: svc,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil || u.Hostname() == "" {
				return false
			}
			// Development convenience: the Vite dev server proxies the
			// collab-doc realtime socket at 127.0.0.1:5173 -> backend on
			// localhost:8080 and rewrites the Host header, so Origin and
			// Host hostnames diverge in dev. Trust loopback hosts until
			// production deployment supplies a stricter allowlist.
			if u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1" {
				return true
			}
			return strings.EqualFold(u.Hostname(), requestHostname(r.Host))
		},
			Subprotocols: []string{"y-websocket"},
		},
		maxMsgSize: collabDocWSHubLimit,
	}
}

func requestHostname(hostport string) string {
	if u, err := url.Parse("//" + hostport); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return hostport
}

// Handle is the gin handler bound to GET /collaborative-docs/:id/realtime.
func (h *CollabDocRealtimeWSHandler) Handle(c *gin.Context) {
	tenantID, userID, ok := resolveCollabDocIdentity(c)
	if !ok {
		return
	}
	docID := c.Param("id")
	if docID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing doc id"})
		return
	}
	ctx := c.Request.Context()
	doc, err := h.svc.GetDoc(ctx, tenantID, userID, docID)
	if err != nil || doc == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "collab doc not found"})
		return
	}
	ok, err = h.svc.AuthzWrite(ctx, tenantID, userID, docID)
	if err != nil || !ok {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "realtime access denied"})
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf(ctx, "collab doc ws upgrade failed: err=%v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(h.maxMsgSize)

	// v0.7.87 — handshake model mirrors wiki_realtime_ws.go: stay silent
	// on connect and wait for the client's sync step1, which carries
	// its state vector. The pump() loop below replies with sync step2
	// (snapshot bytes or empty), then both sides fall through to the
	// normal y-websocket fanout loop. Pre-emptively sending a frame
	// caused y-websocket to interpret it as a duplicate of its own
	// outgoing frame and close the socket immediately.

	hub := h.joinHub(tenantID, docID, conn)
	defer h.leaveHub(tenantID, docID, hub, conn)

	h.pump(ctx, conn, hub, tenantID, userID, docID)
}

// collabDocWSHub fans frames between WS connections on the same doc.
type collabDocWSHub struct {
	mu     sync.RWMutex
	conns  map[*websocket.Conn]struct{}
	docKey string
}

func newCollabDocWSHub(docKey string) *collabDocWSHub {
	return &collabDocWSHub{docKey: docKey, conns: make(map[*websocket.Conn]struct{})}
}

func (h *CollabDocRealtimeWSHandler) joinHub(tenantID uint64, docID string, conn *websocket.Conn) *collabDocWSHub {
	key := fmt.Sprintf("%d:%s", tenantID, docID)
	hubI, _ := h.pages.LoadOrStore(key, newCollabDocWSHub(key))
	hub := hubI.(*collabDocWSHub)
	hub.mu.Lock()
	hub.conns[conn] = struct{}{}
	hub.mu.Unlock()
	h.svc.TrackConnection(tenantID, docID, +1)
	return hub
}

func (h *CollabDocRealtimeWSHandler) leaveHub(tenantID uint64, docID string, hub *collabDocWSHub, conn *websocket.Conn) {
	hub.mu.Lock()
	// The handler owns one connection; do not evict other collaborators when
	// a single browser tab disconnects.
	delete(hub.conns, conn)
	hub.mu.Unlock()
	hub.mu.RLock()
	empty := len(hub.conns) == 0
	hub.mu.RUnlock()
	if empty {
		h.pages.Range(func(k, v interface{}) bool {
			if v == hub {
				h.pages.Delete(k)
				return false
			}
			return true
		})
	}
	h.svc.TrackConnection(tenantID, docID, -1)
	h.svc.RemoveSession(context.Background(), tenantID, docID, 0)
}

// pump reads frames from conn, applies them, and broadcasts to peers.
// This is the in-process equivalent of WikiRealtimeWSHandler.pump, scoped
// to one doc.
func (h *CollabDocRealtimeWSHandler) pump(
	ctx context.Context, conn *websocket.Conn, hub *collabDocWSHub,
	tenantID, userID uint64, docID string,
) {
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warnf(ctx, "collab doc ws read error doc=%s err=%v", docID, err)
			}
			return
		}
		_ = msgType

		frame, err := decodeCollabFrame(data)
		if err != nil {
			logger.Warnf(ctx, "collab doc ws decode error doc=%s err=%v", docID, err)
			continue
		}
		switch frame.typ {
		case collabMsgSync:
			if frame.sub == collabMsgSyncStep1 {
				// The binary editor payload is the durable source of truth for
				// DOC/SHEET/SLIDE/FORM. Do not send collab_doc_snapshots here:
				// older builds stored raw Yjs updates by concatenating them,
				// which is not a valid Yjs document update and causes clients to
				// fail with "Unexpected end of array". A zero-length update is
				// encoded by Yjs as [0, 0] (not an empty byte slice); use that
				// valid empty document update to complete the handshake.
				response := buildCollabSyncFrame(collabMsgSync, collabMsgSyncStep2, []byte{0, 0})
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, response); err != nil {
					return
				}
				continue
			}
			// Step2/update payloads are already valid standard y-websocket
			// frames. Keep them opaque here and fan them out verbatim; Yjs
			// clients apply and merge the updates locally.
		case collabMsgAwareness:
			// presence frame — keep the session row warm.
			_ = h.svc.UpsertSession(ctx, &types.CollabDocSession{
				ID:            fmt.Sprintf("%d-%d", userID, time.Now().UnixNano()),
				TenantID:      tenantID,
				DocID:         docID,
				UserID:        userID,
				ClientID:      0,
				Color:         "#58a6ff",
				DisplayName:   "",
				LastHeartbeat: time.Now(),
				JoinedAt:      time.Now(),
			})
		}
		// fan-out to peers
		fanoutCollabDoc(hub, conn, data)
	}
}

func fanoutCollabDoc(hub *collabDocWSHub, sender *websocket.Conn, data []byte) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for c := range hub.conns {
		if c == sender {
			continue
		}
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
			logger.Warnf(context.Background(), "collab doc ws fanout error: %v", err)
		}
	}
}

// --- wire-protocol codec ---------------------------------------------------
//
// The frame format is identical to wiki_realtime_ws.go so both surfaces
// share the standard y-websocket protocol. We define our own message-type
// constants (1 == sync, 2 == awareness) to avoid an import cycle on the
// wiki handler package.

// collabDocFrame is the decoded wire-protocol frame.
type collabDocFrame struct {
	typ     byte
	sub     byte
	payload []byte
}

const (
	collabMsgSync      byte = 0
	collabMsgAwareness byte = 1

	collabMsgSyncStep1  byte = 0
	collabMsgSyncStep2  byte = 1
	collabMsgSyncUpdate byte = 2
)

func decodeCollabFrame(data []byte) (collabDocFrame, error) {
	typ, n := binary.Uvarint(data)
	if n <= 0 || typ > 255 {
		return collabDocFrame{}, fmt.Errorf("frame type invalid")
	}
	rest := data[n:]
	if typ != uint64(collabMsgSync) {
		return collabDocFrame{typ: byte(typ), payload: rest}, nil
	}
	sub, n := binary.Uvarint(rest)
	if n <= 0 || sub > 255 {
		return collabDocFrame{}, fmt.Errorf("sync subtype invalid")
	}
	rest = rest[n:]
	payload, _, ok := readCollabByteArray(rest)
	if !ok {
		return collabDocFrame{}, fmt.Errorf("sync payload invalid")
	}
	return collabDocFrame{typ: byte(typ), sub: byte(sub), payload: payload}, nil
}

func buildCollabSyncFrame(msgType, subType byte, payload []byte) []byte {
	// v0.7.85 — encode with standard y-websocket layout:
	//   varUint(messageType) | varUint(syncSubtype) | varUint8Array(payload)
	// y-websocket writes its sync/awareness frames using y-protocols/sync,
	// which always length-prefixes both the outer messageType and the
	// inner payload via lib0's LEB128 varUint. The earlier 1-byte-length
	// layout silently truncated snapshots >127 bytes.
	var buf [binary.MaxVarintLen64]byte
	out := make([]byte, 0, binary.MaxVarintLen64*3+len(payload))
	n := binary.PutUvarint(buf[:], uint64(msgType))
	out = append(out, buf[:n]...)
	n = binary.PutUvarint(buf[:], uint64(subType))
	out = append(out, buf[:n]...)
	n = binary.PutUvarint(buf[:], uint64(len(payload)))
	out = append(out, buf[:n]...)
	out = append(out, payload...)
	return out
}

func resolveCollabDocIdentity(c *gin.Context) (uint64, uint64, bool) {
	t, exists := c.Get(types.TenantIDContextKey.String())
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return 0, 0, false
	}
	tenantID, ok := toUint64(t)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return 0, 0, false
	}
	u, exists := c.Get(types.UserIDContextKey.String())
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return 0, 0, false
	}
	userID, ok := collabRealtimeUserID(u)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user context"})
		return 0, 0, false
	}
	return tenantID, userID, true
}

func collabRealtimeUserID(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, typed > 0
	case string:
		if typed == "" {
			return 0, false
		}
		hash := sha256.Sum256([]byte(typed))
		id := binary.BigEndian.Uint64(hash[:8]) &^ (uint64(1) << 63)
		return id, id > 0
	default:
		return 0, false
	}
}
func readCollabByteArray(data []byte) ([]byte, int, bool) {
	length, n := binary.Uvarint(data)
	if n <= 0 || length > uint64(len(data)-n) {
		return nil, 0, false
	}
	end := n + int(length)
	return data[n:end], end, true
}

// keep the placeholder so future cleanup is intentional rather than accidental
var _ = readCollabByteArray
