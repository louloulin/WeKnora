package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

// y-protocols reference notes:
//
//	y-websocket uses binary message types for sync updates and awareness.
//	We deliberately do NOT parse those frames here — they are opaque
//	to the server. The server's role is to:
//	  1. Authenticate the connection (JWT in `?token=`).
//	  2. Fan out frames between clients in the same room.
//	  3. Debounce-persist the room's Y.Doc snapshot.
//	  4. Replay the snapshot to late joiners.
//
//	Snapshot bytes are produced by encoding the server-side Y.Doc using
//	`y.encodeStateAsUpdate`, but since we keep no NodeJS-style runtime
//	on the server, we instead store the raw binary frames the active
//	clients exchange. This is a pragmatic first cut: clients always
//	have a complete document state, the server only needs to persist
//	a fragment that late joiners can replay. To keep that simple we
//	rely on the CRDT property that any superset of sync-step-1 +
//	sync-step-2 responses fully reconstructs the document.
//
//	For Build #8 prototype we persist the **full broadcast buffer**
//	collected during a debounce window. When a client connects, we
//	send the entire buffer (replay) plus the live fan-out. CRDT
//	convergence is preserved because every entry in the buffer is
//	a valid update message; replaying the same messages through
//	Y.applyUpdate on the joining client yields the same document.

const (
	wikiCollabHeartbeatInterval = 15 * time.Second
	wikiCollabReadTimeout       = 75 * time.Second // y-websocket can be idle while a peer composes; 75s covers one full heartbeat window plus jitter
	wikiCollabHandshakeTimeout  = 10 * time.Second
	wikiCollabWriteTimeout      = 10 * time.Second
	wikiCollabMaxMessageSize    = 4 << 20 // 4 MiB — large document fragments (CRDT snapshots) need more headroom than IM chat
	wikiCollabSnapshotDebounce  = 30 * time.Second
	wikiCollabSendQueueSize     = 128
)

// wikiCollabHub manages all live wiki collab rooms.
//
//	Lifetime: a single Hub is constructed at process start and shared
//	across the request handlers. Rooms are created on first join and
//	GC'd after the last client leaves + the snapshot is persisted.
//
//	Concurrency model:
//	  - Hub.mu guards `rooms` (the map) only.
//	  - Each room has its own mutex (Room.mu) guarding client membership
//	    and the broadcast queue.
//	  - Snapshot writes are issued on the room's own goroutine, never
//	    from the reader goroutine.
type wikiCollabHub struct {
	mu        sync.RWMutex
	rooms     map[string]*wikiCollabRoom
	snapshots wikiCollabSnapshotStore
}

func newWikiCollabHub(snapshots wikiCollabSnapshotStore) *wikiCollabHub {
	return &wikiCollabHub{
		rooms:     make(map[string]*wikiCollabRoom),
		snapshots: snapshots,
	}
}

// roomKey returns the canonical room identifier for a (kb, slug) pair.
// Slashes inside the slug are preserved (y-websocket treats the trailing
// path as opaque) but the kb_id is sanitized so a malicious caller can't
// forge a different room by URL-encoding slashes.
func wikiCollabRoomKey(kbID, slug string) string {
	return path.Clean("/" + secutils.SanitizeForLog(kbID)) + "/" + path.Clean("/"+strings.TrimSpace(slug))
}

// join attaches a client to the named room, creating the room if needed.
func (h *wikiCollabHub) join(key string, c *wikiCollabClient) *wikiCollabRoom {
	h.mu.Lock()
	room, ok := h.rooms[key]
	if !ok {
		room = newWikiCollabRoom(key, h.snapshots)
		h.rooms[key] = room
		go room.run()
	}
	h.mu.Unlock()
	room.addClient(c)
	return room
}

// leave removes a client from its room. The room is GC'd by run() once
// the last client exits.
func (h *wikiCollabHub) leave(c *wikiCollabClient) {
	if c == nil || c.room == nil {
		return
	}
	c.room.removeClient(c)
}

// wikiCollabRoom is the per-(kb, slug) fan-out broker.
//
//	Each room has a single goroutine (`run`) that owns the
//	broadcast queue, persistence debouncing, and room cleanup.
type wikiCollabRoom struct {
	key       string
	snapshots wikiCollabSnapshotStore

	mu        sync.Mutex
	clients   map[*wikiCollabClient]struct{}
	closed    bool
	broadcast chan []byte

	// joined   tracks whether we've already replayed the persisted
	// snapshot to a freshly-joined client. The room itself doesn't
	// own the buffer; each client maintains its own replay queue.
}

func newWikiCollabRoom(key string, snapshots wikiCollabSnapshotStore) *wikiCollabRoom {
	return &wikiCollabRoom{
		key:       key,
		snapshots: snapshots,
		clients:   make(map[*wikiCollabClient]struct{}),
		broadcast: make(chan []byte, wikiCollabSendQueueSize*4),
	}
}

func (r *wikiCollabRoom) addClient(c *wikiCollabClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		// Late joiner after the room was marked for cleanup; treat as a
		// fresh room via Hub.join's creation path.
		return
	}
	r.clients[c] = struct{}{}
}

func (r *wikiCollabRoom) removeClient(c *wikiCollabClient) {
	r.mu.Lock()
	delete(r.clients, c)
	empty := len(r.clients) == 0
	r.mu.Unlock()
	if empty && !r.closed {
		// Trigger persistence + self-close by closing broadcast channel.
		// Caller must not block on broadcast after this point.
		r.mu.Lock()
		if !r.closed {
			r.closed = true
			close(r.broadcast)
		}
		r.mu.Unlock()
	}
}

// run is the room's single owner goroutine. It drains the broadcast
// queue, debounces snapshot writes, and exits when the room closes.
func (r *wikiCollabRoom) run() {
	defer func() {
		// Final snapshot flush on shutdown — last write wins.
		// Errors are logged but never block shutdown.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.snapshots.FlushNow(ctx, r.key); err != nil {
			logger.Warnf(ctx, "[wiki-collab] final snapshot flush failed for room %s: %v", r.key, err)
		}
	}()
	debounce := time.NewTimer(wikiCollabSnapshotDebounce)
	defer debounce.Stop()
	for {
		select {
		case frame, ok := <-r.broadcast:
			if !ok {
				// Room closed; Hub will sweep us out of the map.
				return
			}
			r.fanout(frame)
			r.snapshots.NoteFrame(r.key, frame)
			// (Re)arm debounce on every activity.
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(wikiCollabSnapshotDebounce)
		case <-debounce.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.snapshots.FlushNow(ctx, r.key); err != nil {
				logger.Warnf(ctx, "[wiki-collab] debounced snapshot flush failed for room %s: %v", r.key, err)
			}
			cancel()
			debounce.Reset(wikiCollabSnapshotDebounce)
		}
	}
}

func (r *wikiCollabRoom) fanout(frame []byte) {
	r.mu.Lock()
	clients := make([]*wikiCollabClient, 0, len(r.clients))
	for c := range r.clients {
		clients = append(clients, c)
	}
	r.mu.Unlock()
	for _, c := range clients {
		c.enqueue(frame)
	}
}

// wikiCollabClient is one connected WS peer.
type wikiCollabClient struct {
	conn       *ws.Conn
	room       *wikiCollabRoom
	send       chan []byte
	tenantID   uint64
	userID     string
	display    string
	closeOnce  sync.Once
	closed     atomic.Bool
	replayBuf  []byte // persisted snapshot bytes to replay on join
	replayOnce sync.Once
}

func (c *wikiCollabClient) enqueue(frame []byte) {
	if c.closed.Load() {
		return
	}
	select {
	case c.send <- frame:
	default:
		// Slow consumer: drop the connection rather than block the
		// whole room. The client can reconnect and re-sync via
		// y-websocket's sync protocol.
		logger.Warnf(context.Background(), "[wiki-collab] dropping slow consumer %s in room %s", c.userID, c.room.key)
		c.close(ws.ClosePolicyViolation, "send queue full")
	}
}

func (c *wikiCollabClient) close(code int, reason string) {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		msg := ws.FormatCloseMessage(code, reason)
		_ = c.conn.SetWriteDeadline(time.Now().Add(wikiCollabWriteTimeout))
		_ = c.conn.WriteMessage(ws.CloseMessage, msg)
		_ = c.conn.Close()
		close(c.send)
	})
}

// wikiCollabSnapshotStore is the persistence interface used by the hub.
//
//	Production wires this to a SQL-backed store; tests can plug in
//	an in-memory implementation. The interface is intentionally small:
//	NoteFrame records a frame in the in-memory buffer (debounced),
//	FlushNow force-persists the buffer, and LoadLatest fetches bytes
//	to replay to a late joiner.
type wikiCollabSnapshotStore interface {
	NoteFrame(roomKey string, frame []byte)
	FlushNow(ctx context.Context, roomKey string) error
	LoadLatest(ctx context.Context, roomKey string) ([]byte, error)
}

// WikiCollabHandler exposes the WS upgrade endpoint for wiki real-time
// collaboration. It does NOT route via the standard wiki CRUD group
// because WebSocket upgrades are sensitive to auth ordering: we accept
// the connection here, then validate KB access inside the handler
// (using the same KB access checks the REST routes apply) before
// dispatching frames.
type WikiCollabHandler struct {
	hub         *wikiCollabHub
	userService interfaces.UserService
	kbService   interfaces.KnowledgeBaseService
	wikiService interfaces.WikiPageService
	awareness   wikiCollabAwarenessStore
}

// NewWikiCollabHandler constructs a new handler with its hub. The hub
// is shared across all instances so rooms survive handler pooling.
//
// The wikiService may be nil in environments where Build #7's ACL
// table is not yet applied — in that case the handler falls through
// to KB-level access only (the legacy behaviour). Production wiring
// passes the same WikiPageService the REST handlers use.
//
// awareness is optional — when nil, awareness frames are still
// forwarded but not persisted, and no recent-collaborators replay is
// sent to new joiners. Prototype / single-tenant deployments pass an
// InMemoryWikiCollabAwarenessStore; multi-tenant production should
// pass an SQLWikiCollabAwarenessStore.
func NewWikiCollabHandler(
	userService interfaces.UserService,
	kbService interfaces.KnowledgeBaseService,
	wikiService interfaces.WikiPageService,
	snapshots wikiCollabSnapshotStore,
	awareness wikiCollabAwarenessStore,
) *WikiCollabHandler {
	return &WikiCollabHandler{
		hub:         newWikiCollabHub(snapshots),
		userService: userService,
		kbService:   kbService,
		wikiService: wikiService,
		awareness:   awareness,
	}
}

// upgrader is shared. CheckOrigin is permissive — wiki pages are
// tenant-scoped and authenticated via JWT, so cross-origin handshakes
// are no worse than the underlying XHR calls. Tighten later if abuse
// surfaces.
var wikiCollabUpgrader = ws.Upgrader{
	HandshakeTimeout: wikiCollabHandshakeTimeout,
	ReadBufferSize:   4096,
	WriteBufferSize:  4096,
	CheckOrigin:      func(r *http.Request) bool { return true },
}

// HandleCollab upgrades the HTTP request to a Y.js WebSocket session.
//
//	URL contract: WS /api/v1/wiki/collab/:kb_id/:slug?token=:jwt&user=:id&name=:display
//	The token is mandatory — anonymous collaborators are rejected at
//	upgrade time. user/name are advisory hints the frontend sends so
//	awareness state survives a reconnect; the server validates them
//	against the JWT subject and rejects mismatches.
func (h *WikiCollabHandler) HandleCollab(c *gin.Context) {
	ctx := c.Request.Context()

	kbID := strings.TrimSpace(c.Param("kb_id"))
	slug := strings.TrimSpace(strings.TrimPrefix(c.Param("slug"), "/"))
	if kbID == "" || slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kb_id and slug are required"})
		return
	}

	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing collab token"})
		return
	}

	user, _, err := h.userService.ValidateToken(ctx, token)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid collab token"})
		return
	}

	// KB existence + wiki-enabled check (mirrors REST handlers' validateWikiKB).
	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	if kb == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge base not found"})
		return
	}
	if !kb.IsWikiEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wiki feature not enabled"})
		return
	}
	if kb.TenantID != user.TenantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cross-tenant access denied"})
		return
	}

	// Per-page ACL: if Build #7 set this page to private/allow_list, the
	// connecting user must be on the allow list. We surface the result
	// here without breaking the contract: 403 closes the WS cleanly.
	if acl, aclErr := h.loadPageACL(ctx, kbID, slug); aclErr == nil && acl != nil {
		if !h.aclAllows(acl, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "page ACL denies access"})
			return
		}
	}

	conn, err := wikiCollabUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warnf(ctx, "[wiki-collab] upgrade failed for kb=%s slug=%s: %v", kbID, slug, err)
		return
	}
	conn.SetReadLimit(wikiCollabMaxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(wikiCollabReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wikiCollabReadTimeout))
	})

	display := strings.TrimSpace(c.Query("name"))
	if display == "" {
		display = user.Username
	}
	userID := strings.TrimSpace(c.Query("user"))
	if userID == "" {
		userID = fmt.Sprintf("u-%d", user.ID)
	}
	// Reject mismatches between query hints and the authenticated subject
	// to prevent one user from masquerading as another in awareness.
	if hintUser := strings.TrimSpace(c.Query("user")); hintUser != "" {
		expected := fmt.Sprintf("u-%d", user.ID)
		if hintUser != user.ID && hintUser != expected {
			_ = conn.Close()
			c.JSON(http.StatusForbidden, gin.H{"error": "user id does not match token subject"})
			return
		}
	}

	client := &wikiCollabClient{
		conn:     conn,
		send:     make(chan []byte, wikiCollabSendQueueSize),
		tenantID: user.TenantID,
		userID:   userID,
		display:  display,
	}

	roomKey := wikiCollabRoomKey(kbID, slug)
	room := h.hub.join(roomKey, client)
	client.room = room

	// Replay persisted snapshot to the new joiner, then close replayBuf
	// so we don't keep the buffer pinned for the connection's lifetime.
	if snapshot, err := h.hub.snapshots.LoadLatest(ctx, roomKey); err == nil && len(snapshot) > 0 {
		client.replayOnce.Do(func() {
			client.replayBuf = snapshot
			client.enqueue(snapshot)
		})
	} else if err != nil && !errors.Is(err, errSnapshotNotFound) {
		logger.Warnf(ctx, "[wiki-collab] snapshot load failed for %s: %v", roomKey, err)
	}

	logger.Infof(ctx, "[wiki-collab] join room=%s user=%s", roomKey, userID)

	// Build #8.1: send a "recent collaborators" replay frame as the
	// first thing the new joiner receives. The frame is wrapped with a
	// leading `WCA1` magic + JSON body so the frontend can distinguish
	// it from raw y-protocol bytes — the wrapper never collides with
	// y-protocol because y-protocol frames start with a varint 0-7
	// (ASCII 0x00..0x07) and "W" is 0x57.
	if h.awareness != nil {
		h.enqueueRecentCollaborators(ctx, client, roomKey)
	}

	go h.writeLoop(client)
	h.readLoop(client)
}

// readLoop consumes frames from the client and forwards them to the
// room's broadcast queue. Heartbeats (ping) are handled implicitly by
// gorilla's SetPongHandler in the upgrade path.
//
//	Origin filtering (Build #8.1):
//	  y-frames carry a varint messageType in the leading byte(s). We
//	  parse it for every incoming frame and:
//	    - sync / awareness (allowed)         → fan out to the room
//	    - auth / queryAwareness / unknown    → silently drop + count
//	    - 3 bad frames within window         → close 1011 (policy)
//	  The bad-frame counter is per-client so a single transient
//	  parse error doesn't lock out an otherwise well-behaved peer.
func (h *WikiCollabHandler) readLoop(c *wikiCollabClient) {
	defer func() {
		h.hub.leave(c)
		c.close(ws.CloseNormalClosure, "client closed")
	}()
	const badFrameLimit = 3
	badFrames := 0
	for {
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			if ws.IsCloseError(err, ws.CloseNormalClosure, ws.CloseGoingAway) {
				return
			}
			if ce, ok := err.(*ws.CloseError); ok {
				logger.Infof(context.Background(), "[wiki-collab] readLoop close code=%d reason=%s", ce.Code, ce.Text)
				return
			}
			logger.Warnf(context.Background(), "[wiki-collab] readLoop error: %v", err)
			return
		}
		// We accept both Text and Binary frames (y-websocket uses binary
		// for sync updates; older clients sometimes emit text wrappers).
		if messageType != ws.TextMessage && messageType != ws.BinaryMessage {
			continue
		}
		if len(data) == 0 {
			continue
		}
		kind, _, perr := parseYFrame(data)
		if perr != nil {
			badFrames++
			logger.Warnf(context.Background(), "[wiki-collab] client %s sent bad frame #%d: %v", c.userID, badFrames, perr)
			if badFrames >= badFrameLimit {
				logger.Warnf(context.Background(), "[wiki-collab] client %s exceeded bad-frame limit; closing", c.userID)
				c.close(ws.ClosePolicyViolation, "too many bad frames")
				return
			}
			continue
		}
		if !isAllowedForward(kind) {
			badFrames++
			logger.Warnf(context.Background(), "[wiki-collab] client %s sent disallowed kind=%d", c.userID, kind)
			if badFrames >= badFrameLimit {
				c.close(ws.ClosePolicyViolation, "disallowed frame type")
				return
			}
			continue
		}
		// Reset the counter on a clean frame — only persistent abuse
		// trips the policy.
		badFrames = 0

		// Build #8.1: awareness frames get persisted before fan-out so
		// late joiners see "who was here recently" without the live
		// awareness channel needing to round-trip through Y.applyUpdate.
		if isAwareness(kind) && h.awareness != nil {
			if err := h.awareness.NoteAwareness(c.room.key, c.userID, data); err != nil {
				logger.Warnf(context.Background(), "[wiki-collab] awareness persist failed: %v", err)
			}
		}

		select {
		case c.room.broadcast <- data:
		default:
			// Room broadcast queue is saturated; this is rare but
			// indicates a runaway publisher. Drop the publisher.
			logger.Warnf(context.Background(), "[wiki-collab] broadcast queue saturated; dropping client %s", c.userID)
			return
		}
	}
}

// writeLoop drains the client's send channel and emits WS frames.
func (h *WikiCollabHandler) writeLoop(c *wikiCollabClient) {
	pingTicker := time.NewTicker(wikiCollabHeartbeatInterval)
	defer pingTicker.Stop()
	for {
		select {
		case frame, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(wikiCollabWriteTimeout))
			if err := c.conn.WriteMessage(ws.BinaryMessage, frame); err != nil {
				logger.Warnf(context.Background(), "[wiki-collab] writeLoop error: %v", err)
				return
			}
		case <-pingTicker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wikiCollabWriteTimeout))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				logger.Warnf(context.Background(), "[wiki-collab] heartbeat write failed: %v", err)
				return
			}
		}
	}
}

// enqueueRecentCollaborators pushes the persisted "recent collaborators"
// list to a freshly-joined client as the first frame on the wire.
//
// Wire format (Build #8.1):
//
//	"WCA1" + JSON(wikiCollabAwarenessReplay{Entries, AsOf})
//
// The "WCA1" magic (4 bytes) lets the frontend reject this frame as
// non-y-protocol before it reaches Y.applyUpdate. The version byte
// (currently '1') lets us evolve the format later without breaking
// older clients.
const wikiCollabReplayMagic = "WCA1"

// wikiCollabAwarenessReplay is the JSON body inside the WCA1 frame.
type wikiCollabAwarenessReplay struct {
	Magic   string                       `json:"magic"`
	Version int                          `json:"version"`
	AsOf    time.Time                    `json:"as_of"`
	Entries []wikiCollabAwarenessEntry   `json:"entries"`
}

func (h *WikiCollabHandler) enqueueRecentCollaborators(ctx context.Context, c *wikiCollabClient, roomKey string) {
	const maxAge = 24 * time.Hour
	const limit = 16
	entries, err := h.awareness.LoadRecent(ctx, roomKey, limit, maxAge)
	if err != nil {
		logger.Warnf(ctx, "[wiki-collab] LoadRecent failed for %s: %v", roomKey, err)
		return
	}
	if len(entries) == 0 {
		return
	}
	// Filter the joiner themselves out — they shouldn't see their own
	// ghost entry.
	filtered := make([]wikiCollabAwarenessEntry, 0, len(entries))
	for _, e := range entries {
		if e.UserID == c.userID {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		return
	}
	body, err := json.Marshal(wikiCollabAwarenessReplay{
		Magic:   wikiCollabReplayMagic,
		Version: 1,
		AsOf:    time.Now().UTC(),
		Entries: filtered,
	})
	if err != nil {
		logger.Warnf(ctx, "[wiki-collab] replay marshal failed: %v", err)
		return
	}
	c.enqueue(body)
}

// --- ACL bridge (Build #7) -------------------------------------------------
//
// Build #7 introduced page-level ACLs on top of KB-level permissions.
// We mirror the REST handler's "if ACL exists, evaluate it; otherwise
// fall through to KB access" rule. The hub itself does not own ACL
// state; it queries the existing ACL handler on connect.

type wikiPageACL struct {
	mode          string   `json:"mode"`
	allowUserIDs  []uint64 `json:"allow_user_ids"`
	allowGroupIDs []uint64 `json:"allow_group_ids"`
	denyInherited bool     `json:"deny_inherited"`
	revision      int64    `json:"revision"`
}

var errSnapshotNotFound = errors.New("wiki collab snapshot not found")

// loadPageACL is a thin indirection over the ACL store. The full ACL
// table lives in Build #7's repo; for Build #8 we read it via the
// wiki service shim so we don't take a hard dependency on the
// ACL table name here. If the wiki service is nil or doesn't expose
// the ACL method (Build #7 not yet applied), this returns
// (nil, nil) — i.e. fall through to KB access.
func (h *WikiCollabHandler) loadPageACL(_ context.Context, kbID, slug string) (*wikiPageACL, error) {
	if h.wikiService == nil {
		return nil, nil
	}
	if aclGetter, ok := h.wikiService.(interface {
		GetWikiPageACL(ctx context.Context, kbID, slug string) (*wikiPageACL, error)
	}); ok {
		return aclGetter.GetWikiPageACL(context.Background(), kbID, slug)
	}
	return nil, nil
}

func (h *WikiCollabHandler) aclAllows(acl *wikiPageACL, userID uint64) bool {
	if acl == nil {
		return true
	}
	switch acl.mode {
	case "", "inherit":
		return true
	case "private":
		return false
	case "allow_list":
		for _, allowed := range acl.allowUserIDs {
			if allowed == userID {
				return true
			}
		}
		return false
	default:
		return true
	}
}
