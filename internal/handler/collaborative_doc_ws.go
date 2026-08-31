// Package handler — v0.7.25 collaborative_docs WebSocket bridge.
//
// Wire protocol: standard y-websocket binary frames (same as Wiki realtime
// in v0.7.19). The handler is structurally a sibling of WikiRealtimeWSHandler
// but is keyed on doc_id rather than (kb_id, page_id) and writes to the
// collab_doc_snapshots / collab_doc_sessions tables.
package handler

import (
	"context"
	"encoding/binary"
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

// CollabDocRealtimeWSHandler is the Yjs bridge for collaborative docs.
type CollabDocRealtimeWSHandler struct {
	svc       *service.CollabDocService
	upgrader  websocket.Upgrader
	pages     sync.Map // docKey -> *collabDocWSHub
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
			CheckOrigin:     func(r *http.Request) bool { return false },
			Subprotocols:    []string{"y-websocket"},
		},
		maxMsgSize: collabDocWSHubLimit,
	}
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

	snapshot, err := h.svc.LoadDocState(ctx, tenantID, docID)
	if err != nil {
		logger.Errorf(ctx, "collab doc ws initial load failed: err=%v", err)
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "load failed"),
			time.Now().Add(time.Second))
		return
	}
	if len(snapshot) > 0 {
		frame := buildCollabSyncFrame(collabMsgSync, collabMsgSyncStep2, snapshot)
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return
		}
	}

	hub := h.joinHub(tenantID, docID, conn)
	defer h.leaveHub(tenantID, docID, hub)

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

func (h *CollabDocRealtimeWSHandler) leaveHub(tenantID uint64, docID string, hub *collabDocWSHub) {
	hub.mu.Lock()
	for c := range hub.conns {
		delete(hub.conns, c)
	}
	hub.mu.Unlock()
	h.pages.Range(func(k, v interface{}) bool {
		if v == hub {
			h.pages.Delete(k)
			return false
		}
		return true
	})
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
			// Append the update to the hot cache + opportunistic snapshot.
			h.svc.AppendUpdate(tenantID, docID, frame.payload)
			if h.svc.SnapshotDue(tenantID, docID) {
				go func() {
					state, _ := h.svc.LoadDocState(context.Background(), tenantID, docID)
					if len(state) > 0 {
						_ = h.svc.PersistSnapshot(context.Background(), tenantID, docID, state, nil)
					}
				}()
			}
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
	if len(data) < 2 {
		return collabDocFrame{}, fmt.Errorf("frame too short")
	}
	typ := data[0]
	rest := data[1:]
	// sync frames are varint-length-prefixed; awareness frames carry the
	// client count then payload (we ignore the count and broadcast whole).
	if typ == collabMsgSync {
		if len(rest) < 1 {
			return collabDocFrame{}, fmt.Errorf("sync frame missing length")
		}
		// The y-websocket sync frame is: [messageType=0, varint length, subType byte, payload]
		// We accept the standard "varint length, sub, payload" form:
		l, n := binary.Uvarint(rest)
		if n <= 0 {
			return collabDocFrame{}, fmt.Errorf("sync frame invalid varint")
		}
		if len(rest) < n+int(l) {
			return collabDocFrame{}, fmt.Errorf("sync frame short")
		}
		payload := rest[n+1 : n+int(l)]
		return collabDocFrame{typ: typ, sub: rest[n], payload: payload}, nil
	}
	return collabDocFrame{typ: typ, sub: 0, payload: rest}, nil
}

func buildCollabSyncFrame(msgType, subType byte, payload []byte) []byte {
	// [type byte, varint length, sub byte, payload...]
	var buf []byte
	buf = append(buf, msgType)
	var lenBuf [binary.MaxVarintLen64]byte
	ln := binary.PutUvarint(lenBuf[:], uint64(len(payload)+1))
	buf = append(buf, lenBuf[:ln]...)
	buf = append(buf, subType)
	buf = append(buf, payload...)
	return buf
}

func resolveCollabDocIdentity(c *gin.Context) (uint64, uint64, bool) {
	t, exists := c.Get("tenant_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing tenant context"})
		return 0, 0, false
	}
	tenantID, ok := toUint64(t)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant context"})
		return 0, 0, false
	}
	u, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user context"})
		return 0, 0, false
	}
	userID, ok := toUint64(u)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user context"})
		return 0, 0, false
	}
	return tenantID, userID, true
}
