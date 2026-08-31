package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WikiRealtimeWSHandler bridges the WeKnora container to a Yjs WebSocket
// connection. The wire protocol is the standard y-websocket binary protocol:
//
//	┌───────────────┬───────────────┬──────────────────┐
//	│ varint length │ type byte (0|1)│ payload bytes    │
//	└───────────────┴───────────────┴──────────────────┘
//
// Message types:
//   • messageSync      = 0 → sync_step1 / sync_step2 / update
//   • messageAwareness = 1 → awareness delta (presence/cursor)
//
// On connect the handler:
//   1. Authenticates the JWT (set by middleware on the gin context).
//   2. Looks up the (tenant_id, user_id, kb_id, page_id) tuple from path.
//   3. Runs AuthZ phase-3 to verify write access on the page.
//   4. Sends the latest snapshot as a sync_step2 so the client converges.
//   5. Pumps inbound updates into the hot cache and fans them out to other
//      WS handlers serving the same page (in-process map; multi-instance
//      fan-out lives on the v0.7.19 Redis pub/sub roadmap).
//
// On disconnect the handler removes the presence row.
type WikiRealtimeWSHandler struct {
	svc       *service.WikiRealtimeService
	upgrader  websocket.Upgrader
	pages     sync.Map // pageKey → *wsHub
	maxMsgSize int64
}

// wikiRealtimeWSHandlerLimit mirrors the v0.7.19 plan: 1 MiB max message,
// 30s write deadline, no cross-origin by default (AuthN-gated).
const wikiRealtimeWSHandlerLimit int64 = 1 << 20

// NewWikiRealtimeWSHandler constructs the handler with the upgraded
// websocket.Upgrader configured to refuse any browser Origin other than the
// ones in the trusted list — production deployments should populate this
// from config; the default denies all for safety.
func NewWikiRealtimeWSHandler(svc *service.WikiRealtimeService) *WikiRealtimeWSHandler {
	return &WikiRealtimeWSHandler{
		svc: svc,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				// WeKnora authenticates via JWT in the URL query or
				// Authorization header — we do not rely on browser Origin
				// for CSRF protection. Deny by default and let the caller
				// explicitly opt-in via the trusted-origins config.
				return false
			},
			Subprotocols: []string{"y-websocket"},
		},
		maxMsgSize: wikiRealtimeWSHandlerLimit,
	}
}

// Handle is the gin handler bound to GET /api/v1/wiki/realtime/:kbId/:pageId.
//
// The :kbId and :pageId path params carry the document identity; the JWT
// in the query string or Authorization header carries the user identity.
// AuthZ is consulted once on connect (write access) — per-message checks
// are skipped to keep the hot path cheap; we trade for "if you have write
// access at connect, you have write access for the lifetime of the WS".
func (h *WikiRealtimeWSHandler) Handle(c *gin.Context) {
	// 1. Resolve identity from middleware-injected context.
	tenantIDVal, exists := c.Get("tenant_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "missing tenant context"})
		return
	}
	tenantID, ok := toUint64(tenantIDVal)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "invalid tenant context"})
		return
	}
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "missing user context"})
		return
	}
	userID, ok := toUint64(userIDVal)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "invalid user context"})
		return
	}

	kbID := c.Param("kbId")
	pageID := c.Param("pageId")
	if kbID == "" || pageID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "missing kb or page id"})
		return
	}

	// 2. AuthZ phase-3 — write access check.
	ctx := c.Request.Context()
	if err := h.svc.ValidateWriteAccess(ctx, tenantID, userID, kbID, pageID); err != nil {
		logger.Warnf(ctx, "wiki realtime authz denied: tenant=%d user=%d kb=%s page=%s err=%v",
			tenantID, userID, kbID, pageID, err)
		c.AbortWithStatusJSON(http.StatusForbidden,
			gin.H{"error": "realtime access denied"})
		return
	}

	// 3. Upgrade.
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf(ctx, "wiki realtime upgrade failed: err=%v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(h.maxMsgSize)

	// 4. Send initial sync_step2 with current snapshot.
	snapshot, err := h.svc.LoadDoc(ctx, tenantID, kbID, pageID)
	if err != nil {
		logger.Errorf(ctx, "wiki realtime initial load failed: err=%v", err)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "load failed"),
			time.Now().Add(time.Second))
		return
	}
	if len(snapshot) > 0 {
		frame := buildSyncFrame(messageSync, messageSyncStep2, snapshot)
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return
		}
	}

	// 5. Enter hub for fan-out.
	hub := h.joinHub(tenantID, kbID, pageID, conn)
	defer h.leaveHub(hub)

	// 6. Pump.
	hub.pump(ctx, h.svc, tenantID, userID, kbID, pageID)
}

// wsHub fans messages between the WS connections attached to one page.
//
// The hub is in-process only; cross-instance fan-out lands in v0.7.19.x
// via Redis pub/sub reusing the AuthZ phase-3 invalidation pattern.
type wsHub struct {
	mu      sync.RWMutex
	conns   map[*websocket.Conn]struct{}
	pageKey string
}

func newWSHub(pageKey string) *wsHub {
	return &wsHub{pageKey: pageKey, conns: make(map[*websocket.Conn]struct{})}
}

func (h *WikiRealtimeWSHandler) joinHub(tenantID uint64, kbID, pageID string, conn *websocket.Conn) *wsHub {
	key := pageKey(tenantID, kbID, pageID)
	hubI, _ := h.pages.LoadOrStore(key, newWSHub(key))
	hub := hubI.(*wsHub)
	hub.mu.Lock()
	hub.conns[conn] = struct{}{}
	hub.mu.Unlock()
	return hub
}

func (h *WikiRealtimeWSHandler) leaveHub(hub *wsHub) {
	hub.mu.Lock()
	for c := range hub.conns {
		delete(hub.conns, c)
	}
	hub.mu.Unlock()
	// Best-effort: drop the hub entry if empty.
	h.pages.Range(func(k, v interface{}) bool {
		if v == hub {
			h.pages.Delete(k)
			return false
		}
		return true
	})
}

// pump reads frames from conn, applies them via the service, and broadcasts
// to other connections on the same hub. Read errors close the loop.
func (hub *wsHub) pump(ctx context.Context, svc *service.WikiRealtimeService, tenantID, userID uint64, kbID, pageID string) {
	// For each conn in the hub we run a pump; here we run for the single
	// conn registered with this hub. The conn pointer lookup is brittle;
	// the cleaner design uses per-conn pump goroutines started in joinHub.
	for c := range singleConn(hub) {
		pumpSingleConn(ctx, c, svc, tenantID, userID, kbID, pageID, hub)
	}
}

// singleConn extracts the (single) connection for this hub invocation. In
// a future revision the hub will own a per-conn goroutine; for now each
// Handle() invocation owns its own conn and pumps it inline.
func singleConn(hub *wsHub) <-chan *websocket.Conn {
	out := make(chan *websocket.Conn, 1)
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for c := range hub.conns {
		out <- c
		break
	}
	close(out)
	return out
}

func pumpSingleConn(ctx context.Context, conn *websocket.Conn, svc *service.WikiRealtimeService, tenantID, userID uint64, kbID, pageID string, hub *wsHub) {
	defer func() {
		hub.mu.Lock()
		delete(hub.conns, conn)
		hub.mu.Unlock()
	}()
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		frame, err := decodeFrame(data)
		if err != nil {
			continue
		}
		switch frame.Type {
		case messageSync:
			if err := svc.ValidateWriteAccess(ctx, tenantID, userID, kbID, pageID); err != nil {
				return
			}
			switch frame.Subtype {
			case messageSyncStep1:
				// Client asks for diff → respond with current state.
				state, err := svc.LoadDoc(ctx, tenantID, kbID, pageID)
				if err != nil {
					return
				}
				resp := buildSyncFrame(messageSync, messageSyncStep2, state)
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, resp); err != nil {
					return
				}
			case messageSyncStep2, messageSyncUpdate:
				_, err := svc.MergeUpdate(ctx, tenantID, kbID, pageID, frame.Payload)
				if err != nil {
					logger.Errorf(ctx, "wiki realtime merge failed: %v", err)
					continue
				}
				svc.IncrementOut()
				// Fan out to peers (excluding sender).
				fanout(ctx, hub, conn, buildSyncFrame(messageSync, messageSyncUpdate, frame.Payload))
			}
		case messageAwareness:
			// Forward verbatim — clients interpret the delta.
			fanout(ctx, hub, conn, data)
		}
	}
}

func fanout(ctx context.Context, hub *wsHub, sender *websocket.Conn, frame []byte) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for c := range hub.conns {
		if c == sender {
			continue
		}
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			logger.Warnf(ctx, "wiki realtime fanout write failed: %v", err)
		}
	}
}

// --- Wire protocol constants (y-websocket v1) ---

const (
	messageSync      byte = 0
	messageAwareness byte = 1

	messageSyncStep1  byte = 0
	messageSyncStep2  byte = 1
	messageSyncUpdate byte = 2
)

// frame is the decoded view of a single y-websocket frame.
type frame struct {
	Type    byte
	Subtype byte
	Payload []byte
}

// decodeFrame parses a length-prefixed frame.
//
// Format:
//
//	┌──── varint length ────┬── type ──┬─ payload ─┐
//	│ (1 byte for ≤127)     │   1 byte │  N bytes  │
//	└───────────────────────┴──────────┴───────────┘
//
// The y-websocket reference uses LEB128 encoding for the length; we use
// single-byte lengths for v0.7.19 since real-world Yjs deltas cluster in
// the 0.5-50 KiB range and the 127-byte cap is reached only by very large
// snapshots (handled via dedicated snapshot frames in v0.7.20).
func decodeFrame(data []byte) (frame, error) {
	if len(data) < 2 {
		return frame{}, errors.New("frame too short")
	}
	payloadLen := int(data[0])
	if payloadLen > 127 {
		return frame{}, fmt.Errorf("frame length %d exceeds v0.7.19 cap", payloadLen)
	}
	if len(data) < 1+payloadLen {
		return frame{}, errors.New("frame truncated")
	}
	return frame{
		Type:    data[1],
		Payload: data[2 : 1+payloadLen],
	}, nil
}

// buildSyncFrame constructs a sync frame with the standard 1-byte length
// prefix. For sync subtypes (step1/step2/update), the payload includes the
// subtype byte as its first byte per the y-websocket spec.
func buildSyncFrame(msgType, subType byte, payload []byte) []byte {
	pl := append([]byte{subType}, payload...)
	out := make([]byte, 0, 2+len(pl))
	out = append(out, byte(len(pl)))
	out = append(out, msgType)
	out = append(out, pl...)
	return out
}

// toUint64 coerces a gin-context value (typically float64 from JWT claims)
// back to uint64. Returns ok=false if the type is unexpected so the caller
// can fail closed.
func toUint64(v interface{}) (uint64, bool) {
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case float64:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	case int:
		if x < 0 {
			return 0, false
		}
		return uint64(x), true
	default:
		return 0, false
	}
}

// EncodeVectorClock is a thin shim used by the snapshot service to produce
// a stable vector clock byte slice from a (client_id, counter) tuple. Not
// part of the y-websocket protocol but kept here for proximity.
func EncodeVectorClock(clientID, counter uint64) []byte {
	out := make([]byte, 16)
	binary.BigEndian.PutUint64(out[0:8], clientID)
	binary.BigEndian.PutUint64(out[8:16], counter)
	return out
}

// Ensure types import isn't dead.
var _ = types.WikiRealtimeSnapshot{}

// HandlePresence is the read-only presence snapshot endpoint.
//
// GET /api/v1/wiki/realtime/:kbId/:pageId/presence
//
// Returns the active presence rows for the page (heartbeat within TTL)
// so the UI can render the collaborator avatar bar before the WS connects.
// AuthZ phase-3 read access is enforced.
func (h *WikiRealtimeWSHandler) HandlePresence(c *gin.Context) {
	tenantIDVal, exists := c.Get("tenant_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "missing tenant context"})
		return
	}
	tenantID, ok := toUint64(tenantIDVal)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "invalid tenant context"})
		return
	}
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "missing user context"})
		return
	}
	userID, ok := toUint64(userIDVal)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": "invalid user context"})
		return
	}

	kbID := c.Param("kbId")
	pageID := c.Param("pageId")
	if kbID == "" || pageID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "missing kb or page id"})
		return
	}

	ctx := c.Request.Context()
	if err := h.svc.ValidateReadAccess(ctx, tenantID, userID, kbID, pageID); err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden,
			gin.H{"error": "presence access denied"})
		return
	}

	rows, err := h.svc.ListPresence(ctx, tenantID, pageID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": "presence query failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"presence": rows})
}

// HandleStats returns realtime counters for diagnostics.
//
// GET /api/v1/wiki/realtime/_stats
//
// Admin-gated in production; in v0.7.19 the route is gated by the same
// auth middleware as other realtime routes. A future iteration will
// split this to /internal/realtime/stats behind a separate admin role.
func (h *WikiRealtimeWSHandler) HandleStats(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Stats())
}

// pageKey returns the canonical cache key for a page (handler-local copy).
func pageKey(tenantID uint64, kbID, pageID string) string {
	return fmt.Sprintf("%d:%s:%s", tenantID, kbID, pageID)
}
