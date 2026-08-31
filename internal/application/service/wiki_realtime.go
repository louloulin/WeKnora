package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiRealtimeService owns the application-level semantics of Yjs realtime
// collaboration: snapshot compaction policy, presence TTL, and the in-memory
// hot cache for connected docs.
//
// Design notes:
//   • Snapshot triggers: every 5 minutes OR every 256 KB of accumulated
//     updates since the last snapshot — whichever comes first.
//   • Hot cache: in-process map[pageKey]*RealtimeDoc with the Yjs binary
//     state. Multi-instance deployments fan-out via Redis pub/sub; the
//     per-instance cache avoids re-reading the snapshot on every message.
//   • Sweep: idle presence rows older than 30s are evicted.
type WikiRealtimeService struct {
	snapRepo   interfaces.WikiRealtimeSnapshotRepository
	sessRepo   interfaces.WikiRealtimeSessionRepository
	authz      WikiRealtimeAuthorizer

	// Compaction thresholds (configurable in future via env vars).
	snapshotInterval time.Duration
	snapshotBytes    int
	presenceIdleTTL  time.Duration

	// In-memory hot doc cache. pageKey = "{tenant}:{kb}:{page}".
	cacheMu sync.RWMutex
	cache   map[string]*wikiRealtimeDocEntry

	// Metrics counters for observability.
	metricsMu      sync.Mutex
	updatesIn      int64
	updatesOut     int64
	snapshotsSaved int64
	sweepsRemoved  int64
}

// WikiRealtimeAuthorizer is the minimal ACL seam the realtime service
// depends on. WeKnora's AuthZ phase-3 already provides a richer Checker;
// this interface exists so the realtime service stays decoupled and
// unit-testable without standing up the full container.
type WikiRealtimeAuthorizer interface {
	CanRead(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error)
	CanWrite(ctx context.Context, tenantID, userID uint64, kbID, pageID string) (bool, error)
}

// wikiRealtimeDocEntry is the per-page hot cache record.
type wikiRealtimeDocEntry struct {
	mu             sync.RWMutex
	tenantID       uint64
	kbID           string
	pageID         string
	state          []byte // latest Yjs encoded state
	accumulated   int     // bytes accumulated since last snapshot
	lastSnapshotAt time.Time
	connections    int // active WS count
}

// NewWikiRealtimeService constructs the service with sensible defaults that
// match the v0.7.19 plan: 5min snapshot interval, 256KB threshold, 30s idle
// presence TTL. Container wiring overrides these in future iterations.
func NewWikiRealtimeService(
	snapRepo interfaces.WikiRealtimeSnapshotRepository,
	sessRepo interfaces.WikiRealtimeSessionRepository,
	authz WikiRealtimeAuthorizer,
) *WikiRealtimeService {
	return &WikiRealtimeService{
		snapRepo:         snapRepo,
		sessRepo:         sessRepo,
		authz:            authz,
		snapshotInterval: 5 * time.Minute,
		snapshotBytes:    256 * 1024,
		presenceIdleTTL:  30 * time.Second,
		cache:            make(map[string]*wikiRealtimeDocEntry),
	}
}

// pageKey returns the canonical cache key for a page.
func pageKey(tenantID uint64, kbID, pageID string) string {
	return fmt.Sprintf("%d:%s:%s", tenantID, kbID, pageID)
}

// LoadDoc returns the cached Yjs state for a page, lazily reading from the
// snapshot repo on cache miss. Returns (nil, nil) if the page has never
// been edited in realtime mode.
func (s *WikiRealtimeService) LoadDoc(ctx context.Context, tenantID uint64, kbID, pageID string) ([]byte, error) {
	key := pageKey(tenantID, kbID, pageID)

	s.cacheMu.RLock()
	if entry, ok := s.cache[key]; ok {
		s.cacheMu.RUnlock()
		entry.mu.RLock()
		defer entry.mu.RUnlock()
		// Return a copy so callers can't mutate the cache by accident.
		out := make([]byte, len(entry.state))
		copy(out, entry.state)
		return out, nil
	}
	s.cacheMu.RUnlock()

	// Cache miss → read from snapshot repo.
	snap, err := s.snapRepo.Get(ctx, tenantID, kbID, pageID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	s.cacheMu.Lock()
	entry := &wikiRealtimeDocEntry{
		tenantID:       tenantID,
		kbID:           kbID,
		pageID:         pageID,
		state:          snapYDocState(snap),
		lastSnapshotAt: time.Now().UTC(),
	}
	s.cache[key] = entry
	s.cacheMu.Unlock()
	if snap != nil {
		return append([]byte(nil), snap.YDocState...), nil
	}
	return nil, nil
}

// snapYDocState safely extracts the binary state, treating nil snapshots
// as "no prior state" → return nil.
func snapYDocState(snap *types.WikiRealtimeSnapshot) []byte {
	if snap == nil {
		return nil
	}
	return append([]byte(nil), snap.YDocState...)
}

// MergeUpdate applies a Yjs update delta to the cached doc and returns the
// (possibly unchanged) state. Compaction triggers a snapshot persist when
// accumulated bytes cross the threshold or the interval elapses.
func (s *WikiRealtimeService) MergeUpdate(ctx context.Context, tenantID uint64, kbID, pageID string, delta []byte) ([]byte, error) {
	if len(delta) == 0 {
		// Empty update — nothing to merge.
		return s.LoadDoc(ctx, tenantID, kbID, pageID)
	}

	key := pageKey(tenantID, kbID, pageID)
	s.cacheMu.Lock()
	entry, ok := s.cache[key]
	if !ok {
		entry = &wikiRealtimeDocEntry{
			tenantID: tenantID,
			kbID:     kbID,
			pageID:   pageID,
		}
		s.cache[key] = entry
	}
	s.cacheMu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Yjs binary updates are concatenative: appending delta to the existing
	// state produces a valid later state. Real CRDT merge semantics live
	// in the client; the server just appends the deltas each client emits.
	entry.state = append(entry.state, delta...)
	entry.accumulated += len(delta)
	entry.lastSnapshotAt = entry.lastSnapshotAt // marker

	s.metricsMu.Lock()
	s.updatesIn++
	s.metricsMu.Unlock()

	// Compact if threshold hit.
	if entry.accumulated >= s.snapshotBytes ||
		time.Since(entry.lastSnapshotAt) > s.snapshotInterval {
		if err := s.persistSnapshotLocked(ctx, entry); err != nil {
			logger.Errorf(ctx, "wiki realtime snapshot persist failed: tenant=%d page=%s err=%v",
				tenantID, pageID, err)
			// Best effort — return current state even if snapshot fails.
		}
	}

	out := make([]byte, len(entry.state))
	copy(out, entry.state)
	return out, nil
}

// persistSnapshotLocked writes the current Yjs state to the snapshot repo.
// Caller must hold entry.mu.
func (s *WikiRealtimeService) persistSnapshotLocked(ctx context.Context, entry *wikiRealtimeDocEntry) error {
	upsert := types.WikiRealtimeSnapshotUpsert{
		TenantID:    entry.tenantID,
		KBID:        entry.kbID,
		PageID:      entry.pageID,
		YDocState:   append([]byte(nil), entry.state...),
		VectorClock: []byte{}, // server-side vector clock TBD with Redis fan-out
		SizeBytes:   len(entry.state),
	}
	_, err := s.snapRepo.Upsert(ctx, upsert)
	if err != nil {
		return err
	}
	entry.accumulated = 0
	entry.lastSnapshotAt = time.Now().UTC()
	s.metricsMu.Lock()
	s.snapshotsSaved++
	s.metricsMu.Unlock()
	return nil
}

// TouchSession upserts a presence row for the (user, client) pair. Returns
// the row's UUID primary key for the awareness message header.
func (s *WikiRealtimeService) TouchSession(ctx context.Context, tenantID uint64, pageID string, userID, clientID uint64, color, displayName string) (string, error) {
	if tenantID == 0 || pageID == "" {
		return "", errors.New("touch session: missing tenant or page")
	}
	row := &types.WikiRealtimeSession{
		TenantID:    tenantID,
		PageID:      pageID,
		UserID:      userID,
		ClientID:    clientID,
		Color:       color,
		DisplayName: displayName,
	}
	if err := s.sessRepo.Upsert(ctx, row); err != nil {
		return "", err
	}
	return row.ID, nil
}

// ListPresence returns active presence rows for a page (last_heartbeat
// within presenceIdleTTL).
func (s *WikiRealtimeService) ListPresence(ctx context.Context, tenantID uint64, pageID string) ([]*types.WikiRealtimeSession, error) {
	cutoff := time.Now().UTC().Add(-s.presenceIdleTTL)
	return s.sessRepo.ListByPage(ctx, tenantID, pageID, cutoff)
}

// ForgetSession removes a single presence row on explicit disconnect.
func (s *WikiRealtimeService) ForgetSession(ctx context.Context, tenantID uint64, pageID string, clientID uint64) error {
	return s.sessRepo.DeleteByClient(ctx, tenantID, pageID, clientID)
}

// SweepIdle removes presence rows older than presenceIdleTTL. Returns the
// number evicted for observability.
func (s *WikiRealtimeService) SweepIdle(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-s.presenceIdleTTL)
	n, err := s.sessRepo.SweepOlderThan(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	s.metricsMu.Lock()
	s.sweepsRemoved += n
	s.metricsMu.Unlock()
	return n, nil
}

// Stats returns a point-in-time snapshot of counters for the /metrics
// endpoint and the v0.7.19 progress dashboard.
type WikiRealtimeStats struct {
	UpdatesIn      int64 `json:"updates_in"`
	UpdatesOut     int64 `json:"updates_out"`
	SnapshotsSaved int64 `json:"snapshots_saved"`
	SweepsRemoved  int64 `json:"sweeps_removed"`
	ActiveDocs     int   `json:"active_docs"`
}

// Stats returns a snapshot of the current counters.
func (s *WikiRealtimeService) Stats() WikiRealtimeStats {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return WikiRealtimeStats{
		UpdatesIn:      s.updatesIn,
		UpdatesOut:     s.updatesOut,
		SnapshotsSaved: s.snapshotsSaved,
		SweepsRemoved:  s.sweepsRemoved,
		ActiveDocs:     len(s.cache),
	}
}

// IncrementOut increments the updates_out counter (called when the WS
// handler fans a delta out to other clients).
func (s *WikiRealtimeService) IncrementOut() {
	s.metricsMu.Lock()
	s.updatesOut++
	s.metricsMu.Unlock()
}

// ValidateReadAccess is a convenience that defers to the authz interface
// and wraps the result for the WS handler.
func (s *WikiRealtimeService) ValidateReadAccess(ctx context.Context, tenantID, userID uint64, kbID, pageID string) error {
	if s.authz == nil {
		return errors.New("wiki realtime: authz not configured")
	}
	ok, err := s.authz.CanRead(ctx, tenantID, userID, kbID, pageID)
	if err != nil {
		return fmt.Errorf("authz check: %w", err)
	}
	if !ok {
		return errors.New("wiki realtime: read denied")
	}
	return nil
}

// ValidateWriteAccess is the write-side counterpart.
func (s *WikiRealtimeService) ValidateWriteAccess(ctx context.Context, tenantID, userID uint64, kbID, pageID string) error {
	if s.authz == nil {
		return errors.New("wiki realtime: authz not configured")
	}
	ok, err := s.authz.CanWrite(ctx, tenantID, userID, kbID, pageID)
	if err != nil {
		return fmt.Errorf("authz check: %w", err)
	}
	if !ok {
		return errors.New("wiki realtime: write denied")
	}
	return nil
}

// Ensure repo imports are referenced (compile-time smoke).
var _ = repository.NewWikiRealtimeSnapshotRepository
var _ = repository.NewWikiRealtimeSessionRepository
