package handler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// WikiCollabSnapshotSchema is the DDL we expect at the SQL store layer.
// Kept here as a constant so a future migration test can verify the
// table shape without reaching into migration files.
//
// Field types:
//   room_key     TEXT PRIMARY KEY  — canonical "/{kb}/{slug}"
//   snapshot     BLOB              — concatenated CRDT frames
//   version      INTEGER           — monotonic, incremented per write
//   last_write   TIMESTAMP         — last successful persistence
type wikiCollabSnapshotRow struct {
	roomKey  string
	snapshot []byte
	version  int64
	lastWrite time.Time
}

// InMemoryWikiCollabSnapshotStore is the default Build #8 prototype store.
//
//	For a single-process backend this is enough: rooms survive within
//	a process lifetime, the debounced flush is a no-op, and LoadLatest
//	reads from the in-memory buffer. Production deployments swap this
//	for the SQL-backed implementation in wiki_collab_sql_store.go.
//
//	Thread-safety: a single sync.RWMutex guards the map. The buffer is
//	appended to under write lock; LoadLatest takes the read lock.
type InMemoryWikiCollabSnapshotStore struct {
	mu    sync.RWMutex
	rooms map[string]*wikiCollabSnapshotRow
}

func NewInMemoryWikiCollabSnapshotStore() *InMemoryWikiCollabSnapshotStore {
	return &InMemoryWikiCollabSnapshotStore{rooms: make(map[string]*wikiCollabSnapshotRow)}
}

// NoteFrame records a CRDT frame in the room's in-memory buffer.
//
//	Production trade-off: we keep appending the raw bytes the room
//	collected during the debounce window. CRDT update messages are
//	commutative when applied in any order — replaying them through
//	Y.applyUpdate on a joiner always reconstructs the same document.
//	Eventually a full document state (encoded via Y.encodeStateAsUpdate)
//	should replace the append-only buffer; that requires running the
//	JS-side yjs encoder on a Node child process or porting it to Go,
//	neither of which Build #8 attempts.
func (s *InMemoryWikiCollabSnapshotStore) NoteFrame(roomKey string, frame []byte) {
	if len(frame) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rooms[roomKey]
	if !ok {
		row = &wikiCollabSnapshotRow{roomKey: roomKey}
		s.rooms[roomKey] = row
	}
	row.snapshot = append(row.snapshot, frame...)
	row.version++
	row.lastWrite = time.Now()
}

// FlushNow is a no-op for the in-memory store — the buffer is already
// live in memory. It exists so the hub's debounce timer doesn't have to
// know which store it's wired to.
func (s *InMemoryWikiCollabSnapshotStore) FlushNow(_ context.Context, _ string) error {
	return nil
}

// LoadLatest returns the accumulated buffer for replay.
func (s *InMemoryWikiCollabSnapshotStore) LoadLatest(_ context.Context, roomKey string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.rooms[roomKey]
	if !ok || len(row.snapshot) == 0 {
		return nil, errSnapshotNotFound
	}
	// Copy out so the caller can mutate without racing the writer.
	out := make([]byte, len(row.snapshot))
	copy(out, row.snapshot)
	return out, nil
}

// --- SQL-backed snapshot store (Build #8 production target) ----------------
//
// Build #8 ships with the in-memory store above as the default. A
// future iteration will replace it with the SQL-backed store below —
// the migration 000094_wiki_collab_snapshots defines the table shape
// the SQL store reads/writes. Keeping the type here makes the upgrade
// path a one-line wiring change at startup time.

// SQLWikiCollabSnapshotStore persists frames to the wiki_collab_snapshots
// table. The store writes each appended frame inside the same transaction
// so a crash mid-flush doesn't truncate history.
//
// Concurrency: SQLite single-writer, MySQL/ParadeDB multi-writer.
// We rely on the database driver to serialize writes; the in-process
// mutex is only here to batch concurrent frame arrivals within a single
// FlushNow call.
type SQLWikiCollabSnapshotStore struct {
	db    *sql.DB
	drive wikiCollabDriver

	mu    sync.Mutex
	rooms map[string]*wikiCollabSnapshotRow
}

// wikiCollabDriver is a tiny abstraction over the two SQL dialects we
// ship. We avoid GORM here because the snapshot write path is
// performance-sensitive (potentially called every 30s per active room)
// and the SQL is small enough that hand-written queries beat the
// reflection overhead.
type wikiCollabDriver int

const (
	wikiCollabDriverMySQL wikiCollabDriver = iota
	wikiCollabDriverSQLite
	wikiCollabDriverPostgres
)

func NewSQLWikiCollabSnapshotStore(db *sql.DB, driver wikiCollabDriver) *SQLWikiCollabSnapshotStore {
	return &SQLWikiCollabSnapshotStore{
		db:     db,
		drive:  driver,
		rooms:  make(map[string]*wikiCollabSnapshotRow),
	}
}

func (s *SQLWikiCollabSnapshotStore) NoteFrame(roomKey string, frame []byte) {
	if len(frame) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rooms[roomKey]
	if !ok {
		row = &wikiCollabSnapshotRow{roomKey: roomKey}
		s.rooms[roomKey] = row
	}
	row.snapshot = append(row.snapshot, frame...)
	row.version++
	row.lastWrite = time.Now()
}

// FlushNow writes the accumulated buffer to SQL. Idempotent: re-flushing
// the same buffer is safe (we always upsert by room_key).
func (s *SQLWikiCollabSnapshotStore) FlushNow(ctx context.Context, roomKey string) error {
	s.mu.Lock()
	row, ok := s.rooms[roomKey]
	if !ok || len(row.snapshot) == 0 {
		s.mu.Unlock()
		return nil
	}
	buf := make([]byte, len(row.snapshot))
	copy(buf, row.snapshot)
	version := row.version
	s.mu.Unlock()

	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.upsert(ctx, roomKey, buf, version)
		if err == nil {
			return nil
		}
		lastErr = err
		logger.Warnf(ctx, "[wiki-collab] snapshot upsert retry %d/%d for %s: %v", attempt, maxRetries, roomKey, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		}
	}
	return fmt.Errorf("snapshot flush gave up after %d retries: %w", maxRetries, lastErr)
}

func (s *SQLWikiCollabSnapshotStore) upsert(ctx context.Context, roomKey string, buf []byte, version int64) error {
	var (
		query string
		args  []any
	)
	switch s.drive {
	case wikiCollabDriverMySQL:
		query = `INSERT INTO wiki_collab_snapshots (room_key, snapshot, version, last_write_at)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				snapshot = VALUES(snapshot),
				version = VALUES(version),
				last_write_at = VALUES(last_write_at)`
		args = []any{roomKey, buf, version, time.Now().UTC()}
	case wikiCollabDriverPostgres:
		query = `INSERT INTO wiki_collab_snapshots (room_key, snapshot, version, last_write_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (room_key) DO UPDATE SET
				snapshot = EXCLUDED.snapshot,
				version = EXCLUDED.version,
				last_write_at = EXCLUDED.last_write_at`
		args = []any{roomKey, buf, version, time.Now().UTC()}
	case wikiCollabDriverSQLite:
		query = `INSERT INTO wiki_collab_snapshots (room_key, snapshot, version, last_write_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(room_key) DO UPDATE SET
				snapshot = excluded.snapshot,
				version = excluded.version,
				last_write_at = excluded.last_write_at`
		args = []any{roomKey, buf, version, time.Now().UTC()}
	default:
		return fmt.Errorf("unsupported wiki-collab driver %d", s.drive)
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLWikiCollabSnapshotStore) LoadLatest(ctx context.Context, roomKey string) ([]byte, error) {
	const query = `SELECT snapshot FROM wiki_collab_snapshots WHERE room_key = ?`
	row := s.db.QueryRowContext(ctx, query, roomKey)
	var buf []byte
	if err := row.Scan(&buf); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errSnapshotNotFound
		}
		return nil, err
	}
	if len(buf) == 0 {
		return nil, errSnapshotNotFound
	}
	// Defensive copy so subsequent mutations don't race the driver buffer.
	out := make([]byte, len(buf))
	copy(out, buf)
	return out, nil
}

// Reset clears all in-memory state for tests.
func (s *InMemoryWikiCollabSnapshotStore) Reset() {
	s.mu.Lock()
	s.rooms = make(map[string]*wikiCollabSnapshotRow)
	s.mu.Unlock()
}

// Helper to detect empty buffer quickly for callers that want to skip persistence.
func isEmptySnapshot(b []byte) bool {
	return len(bytes.TrimSpace(b)) == 0
}
