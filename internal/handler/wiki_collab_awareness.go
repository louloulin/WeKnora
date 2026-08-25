package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// wikiCollabAwarenessEntry is the wire-friendly representation of one
// persisted collaborator. We store the *raw* awareness payload bytes
// alongside a derived `User` summary so the SQL store can return
// results that don't require the caller to re-parse y-protocol state.
//
// Layout on the wire:
//
//	{clientID, userID, displayName, color, lastSeenAt}
type wikiCollabAwarenessEntry struct {
	ClientID    string    `json:"client_id"`
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Color       string    `json:"color"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// wikiCollabAwarenessStore persists awareness frames so a freshly
// joined client can see "who was here recently" before the live
// awareness channel populates.
//
//	Note the interface split from wikiCollabSnapshotStore: the snapshot
//	store owns CRDT content bytes, the awareness store owns ephemeral
//	presence metadata. They live in different tables because their
//	TTL / retention / access patterns are unrelated — snapshots are
//	durable wiki state, awareness is a 24h rolling cache.
type wikiCollabAwarenessStore interface {
	NoteAwareness(roomKey, userID string, frame []byte) error
	LoadRecent(ctx context.Context, roomKey string, limit int, maxAge time.Duration) ([]wikiCollabAwarenessEntry, error)
	Forget(ctx context.Context, roomKey, clientID string) error
	Sweep(ctx context.Context, olderThan time.Time) (int64, error)
}

// --- in-memory implementation ----------------------------------------------

// InMemoryWikiCollabAwarenessStore is the default Build #8.1 prototype.
//
//	Single-process, no persistence across restarts. The hub also keeps
//	a reference so the periodic sweep loop can call Sweep() without
//	external scheduling.
type InMemoryWikiCollabAwarenessStore struct {
	mu      sync.RWMutex
	entries map[string]map[string]*inMemAwarenessEntry // roomKey → clientID → entry
	now     func() time.Time
}

type inMemAwarenessEntry struct {
	ClientID    string
	UserID      string
	DisplayName string
	Color       string
	LastSeenAt  time.Time
	// Raw payload bytes — preserved for replay to late joiners that
	// want the full y-protocol state vector. Currently we don't ship
	// these on the wire (the hub extracts a JSON summary instead);
	// keeping them around for the next iteration.
	RawPayload []byte
}

func NewInMemoryWikiCollabAwarenessStore() *InMemoryWikiCollabAwarenessStore {
	return &InMemoryWikiCollabAwarenessStore{
		entries: make(map[string]map[string]*inMemAwarenessEntry),
		now:     time.Now,
	}
}

// NoteAwareness upserts the entry. The frame payload is parsed best-effort
// for {user, color} — if parsing fails (a malicious or stale client
// emits non-Awareness bytes tagged with kind=awareness), we still
// record the entry with whatever userID the hub derived from the JWT
// so the entry is at least identifiable.
func (s *InMemoryWikiCollabAwarenessStore) NoteAwareness(roomKey, userID string, frame []byte) error {
	summary, _ := parseAwarenessSummary(frame)
	display, color := "", ""
	if summary != nil {
		display, color = summary.DisplayName, summary.Color
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.entries[roomKey]
	if !ok {
		room = make(map[string]*inMemAwarenessEntry)
		s.entries[roomKey] = room
	}
	clientID := userID // 1:1 in our build; awareness frames don't carry a separate clientID
	room[clientID] = &inMemAwarenessEntry{
		ClientID:    clientID,
		UserID:      userID,
		DisplayName: display,
		Color:       color,
		LastSeenAt:  s.now(),
		RawPayload:  frame,
	}
	return nil
}

func (s *InMemoryWikiCollabAwarenessStore) Forget(_ context.Context, roomKey, clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if room, ok := s.entries[roomKey]; ok {
		delete(room, clientID)
		if len(room) == 0 {
			delete(s.entries, roomKey)
		}
	}
	return nil
}

func (s *InMemoryWikiCollabAwarenessStore) LoadRecent(_ context.Context, roomKey string, limit int, maxAge time.Duration) ([]wikiCollabAwarenessEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.entries[roomKey]
	if !ok {
		return nil, nil
	}
	cutoff := s.now().Add(-maxAge)
	out := make([]wikiCollabAwarenessEntry, 0, len(room))
	for _, e := range room {
		if e.LastSeenAt.Before(cutoff) {
			continue
		}
		out = append(out, wikiCollabAwarenessEntry{
			ClientID:    e.ClientID,
			UserID:      e.UserID,
			DisplayName: e.DisplayName,
			Color:       e.Color,
			LastSeenAt:  e.LastSeenAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *InMemoryWikiCollabAwarenessStore) Sweep(_ context.Context, olderThan time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed int64
	for roomKey, room := range s.entries {
		for clientID, e := range room {
			if e.LastSeenAt.Before(olderThan) {
				delete(room, clientID)
				removed++
			}
		}
		if len(room) == 0 {
			delete(s.entries, roomKey)
		}
	}
	return removed, nil
}

// --- SQL implementation ----------------------------------------------------

// SQLWikiCollabAwarenessStore persists awareness frames into the
// wiki_collab_awareness table. Stores the raw frame as JSONB for
// future y-protocol-level replay plus a denormalized summary so the
// "recent collaborators" list can be assembled without a parse pass.
type SQLWikiCollabAwarenessStore struct {
	db    *sql.DB
	drive wikiCollabDriver
	mu    sync.Mutex
	now   func() time.Time
}

func NewSQLWikiCollabAwarenessStore(db *sql.DB, driver wikiCollabDriver) *SQLWikiCollabAwarenessStore {
	return &SQLWikiCollabAwarenessStore{db: db, drive: driver, now: time.Now}
}

func (s *SQLWikiCollabAwarenessStore) NoteAwareness(roomKey, userID string, frame []byte) error {
	summary, _ := parseAwarenessSummary(frame)
	display, color := "", ""
	if summary != nil {
		display, color = summary.DisplayName, summary.Color
	}
	payloadJSON, err := json.Marshal(map[string]string{
		"raw":    string(frame),
		"client": userID,
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		query string
		args  []any
	)
	switch s.drive {
	case wikiCollabDriverMySQL:
		query = `INSERT INTO wiki_collab_awareness (room_key, client_id, user_id, display_name, color, payload, last_seen_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				display_name = VALUES(display_name),
				color = VALUES(color),
				payload = VALUES(payload),
				last_seen_at = VALUES(last_seen_at),
				expires_at = VALUES(expires_at)`
		now := s.now().UTC()
		args = []any{roomKey, userID, userID, display, color, payloadJSON, now, now.Add(24 * time.Hour)}
	case wikiCollabDriverPostgres:
		query = `INSERT INTO wiki_collab_awareness (room_key, client_id, user_id, display_name, color, payload, last_seen_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (room_key, client_id) DO UPDATE SET
				display_name = EXCLUDED.display_name,
				color = EXCLUDED.color,
				payload = EXCLUDED.payload,
				last_seen_at = EXCLUDED.last_seen_at,
				expires_at = EXCLUDED.expires_at`
		now := s.now().UTC()
		args = []any{roomKey, userID, userID, display, color, payloadJSON, now, now.Add(24 * time.Hour)}
	case wikiCollabDriverSQLite:
		query = `INSERT INTO wiki_collab_awareness (room_key, client_id, user_id, display_name, color, payload, last_seen_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(room_key, client_id) DO UPDATE SET
				display_name = excluded.display_name,
				color = excluded.color,
				payload = excluded.payload,
				last_seen_at = excluded.last_seen_at,
				expires_at = excluded.expires_at`
		now := s.now().UTC()
		args = []any{roomKey, userID, userID, display, color, payloadJSON, now, now.Add(24 * time.Hour)}
	default:
		return fmt.Errorf("unsupported wiki-collab driver %d", s.drive)
	}
	_, err = s.db.ExecContext(context.Background(), query, args...)
	return err
}

func (s *SQLWikiCollabAwarenessStore) Forget(ctx context.Context, roomKey, clientID string) error {
	const query = `DELETE FROM wiki_collab_awareness WHERE room_key = ? AND client_id = ?`
	_, err := s.db.ExecContext(ctx, query, roomKey, clientID)
	return err
}

func (s *SQLWikiCollabAwarenessStore) LoadRecent(ctx context.Context, roomKey string, limit int, maxAge time.Duration) ([]wikiCollabAwarenessEntry, error) {
	if limit <= 0 {
		limit = 16
	}
	cutoff := s.now().UTC().Add(-maxAge)
	const query = `SELECT client_id, user_id, COALESCE(display_name,''), COALESCE(color,''), last_seen_at
		FROM wiki_collab_awareness
		WHERE room_key = ? AND last_seen_at >= ?
		ORDER BY last_seen_at DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, roomKey, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent awareness: %w", err)
	}
	defer rows.Close()
	var out []wikiCollabAwarenessEntry
	for rows.Next() {
		var e wikiCollabAwarenessEntry
		if err := rows.Scan(&e.ClientID, &e.UserID, &e.DisplayName, &e.Color, &e.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan awareness row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLWikiCollabAwarenessStore) Sweep(ctx context.Context, olderThan time.Time) (int64, error) {
	const query = `DELETE FROM wiki_collab_awareness WHERE expires_at < ?`
	res, err := s.db.ExecContext(ctx, query, olderThan.UTC())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- payload summary parser ------------------------------------------------

// awarenessSummary is the JSON-decoded form of an awareness frame's
// inner state. We only parse the bits we render in the UI; the rest
// of the binary payload is preserved verbatim in `RawPayload` for
// future replay.
type awarenessSummary struct {
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
}

// parseAwarenessSummary extracts {display_name, color} from an
// awareness frame. Awareness frames in y-protocol are themselves
// CRDT-shaped; the inner state is `{clientID: {user: {name, color,
// ...}}}`. Parsing that without a full yjs port is brittle — the safe
// fallback is to scan the byte slice for the JSON markers we know our
// frontend sends. If we miss, the entry is still persisted (under
// the JWT userID) but with empty display/color.
func parseAwarenessSummary(frame []byte) (*awarenessSummary, error) {
	// Cheap heuristics: y-frames for awareness are typically
	// `[0x01, <varint length>, ...]` followed by a JSON-ish blob.
	// We scan for `{` to find the start of the state map, then walk
	// the string bytes to find `"name"`/`"color"`.
	s := string(frame)
	nameStart := indexJSONString(s, "name")
	if nameStart < 0 {
		return nil, errors.New("no name field")
	}
	colorStart := indexJSONString(s, "color")
	out := &awarenessSummary{}
	if nameStart >= 0 {
		out.DisplayName = readJSONStringValue(s, nameStart)
	}
	if colorStart >= 0 {
		out.Color = readJSONStringValue(s, colorStart)
	}
	return out, nil
}

func indexJSONString(s, key string) int {
	needle := `"` + key + `"`
	return bytesIndex(s, needle)
}

func readJSONStringValue(s string, keyPos int) string {
	rest := s[keyPos:]
	colon := bytesIndex(rest, ":")
	if colon < 0 {
		return ""
	}
	quote := bytesIndex(rest[colon:], `"`)
	if quote < 0 {
		return ""
	}
	rest = rest[colon+quote+1:]
	end := bytesIndex(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// bytesIndex avoids the strings.Index in hot paths.
func bytesIndex(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

// --- start a periodic sweep when the SQL store is wired --------------------

// StartAwarenessSweeper launches a background goroutine that calls
// Sweep() on `store` at `interval`. Returns a stop function the
// caller should defer at process shutdown.
//
//	Production deployments may already have an external cron sweeping
//	expired rows; this helper is a noop-friendly default for the
//	prototype so single-process deployments don't leak awareness rows
//	forever.
func StartAwarenessSweeper(ctx context.Context, store wikiCollabAwarenessStore, interval time.Duration) func() {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cutoff := time.Now().Add(-24 * time.Hour)
				n, err := store.Sweep(ctx, cutoff)
				if err != nil {
					logger.Warnf(ctx, "[wiki-collab] awareness sweep failed: %v", err)
				} else if n > 0 {
					logger.Infof(ctx, "[wiki-collab] awareness sweep removed %d stale rows", n)
				}
			}
		}
	}()
	return cancel
}
