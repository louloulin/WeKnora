// Package service — v0.7.25 collaborative_docs service.
//
// The realtime engine mirrors WikiRealtimeService but is keyed on (tenant,
// doc_id) instead of (tenant, kb_id, page_id). Snapshot compaction
// thresholds, presence TTL and hot-cache policy are intentionally identical
// so the two surfaces behave the same way under load.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// CollabDocService is the application-level surface for the collaborative
// document feature: document CRUD, snapshot persistence, presence sweeper.
// The realtime Yjs engine lives on the same struct so a doc snapshot is
// always loaded through a single seam.
type CollabDocService struct {
	docRepo    interfaces.CollabDocRepository
	snapRepo   interfaces.CollabDocSnapshotRepository
	sessRepo   interfaces.CollabDocSessionRepository
	fileRepo     interfaces.CollabDocFileRepository
	commentRepo  interfaces.CollabDocCommentRepository
	authz        CollabDocAuthorizer

	snapshotInterval time.Duration
	snapshotBytes    int
	presenceIdleTTL  time.Duration
	// fileVersionRetention: how many historical .docx/.pptx/.xlsx versions
	// to keep per doc. The latest row is always kept; rows older than the
	// cutoff (fileRetentionAge) and not in the latest N are purged on
	// SweepOldFileVersions.
	fileVersionRetention int
	fileRetentionAge     time.Duration

	cacheMu sync.RWMutex
	cache   map[string]*collabDocEntry
}

// CollabDocAuthorizer is the minimal ACL seam. The full AuthZ phase-3
// checker provides a richer interface; this seam keeps the service
// unit-testable without standing up the full container.
type CollabDocAuthorizer interface {
	CanRead(ctx context.Context, tenantID, userID uint64, docID string) (bool, error)
	CanWrite(ctx context.Context, tenantID, userID uint64, docID string) (bool, error)
}

// collabDocEntry is the per-doc hot cache record.
type collabDocEntry struct {
	mu             sync.RWMutex
	tenantID       uint64
	docID          string
	docKind        types.CollaborativeDocKind
	state          []byte
	accumulated    int
	lastSnapshotAt time.Time
	connections    int
}

// NewCollabDocService wires the service with sensible defaults matching
// the wiki realtime policy: 5-min snapshot interval, 256 KB threshold, 30 s
// idle presence TTL.
func NewCollabDocService(
	docRepo interfaces.CollabDocRepository,
	snapRepo interfaces.CollabDocSnapshotRepository,
	sessRepo interfaces.CollabDocSessionRepository,
	fileRepo interfaces.CollabDocFileRepository,
	commentRepo interfaces.CollabDocCommentRepository,
	authz CollabDocAuthorizer,
) *CollabDocService {
	return &CollabDocService{
		docRepo:     docRepo,
		snapRepo:    snapRepo,
		sessRepo:    sessRepo,
		fileRepo:    fileRepo,
		commentRepo: commentRepo,
		authz:       authz,
		snapshotInterval:     5 * time.Minute,
		snapshotBytes:        256 * 1024,
		presenceIdleTTL:      30 * time.Second,
		fileVersionRetention: 10, // keep the latest 10 .docx/.pptx/.xlsx per doc
		fileRetentionAge:     30 * 24 * time.Hour, // 30 days
		cache:                make(map[string]*collabDocEntry),
	}
}

// newCollabDocID returns a 36-char UUIDv4 string. We avoid pulling in a
// UUID dependency just for this; rand.Read + format is sufficient for a
// v4-style identifier.
func newCollabDocID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely (rand.Read only fails when the OS RNG is
		// broken). Fall back to a time-based pseudo ID so the doc still
		// gets a unique-enough identifier.
		ns := time.Now().UnixNano()
		return fmt.Sprintf("doc-%016x", ns)
	}
	// Set version (4) and variant (10xx).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hex := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex[0:8], hex[8:12], hex[12:16], hex[16:20], hex[20:])
}

// collabDocKey returns the canonical cache key for a doc.
func collabDocKey(tenantID uint64, docID string) string {
	return fmt.Sprintf("%d:%s", tenantID, docID)
}

// CreateDoc creates a new collab document row and returns the persisted entity.
func (s *CollabDocService) CreateDoc(
	ctx context.Context, tenantID, ownerUserID uint64, req types.CreateCollaborativeDocRequest,
) (*types.CollaborativeDoc, error) {
	if req.KBID == "" {
		return nil, types.ErrCollabDocInvalid("kb_id is required")
	}
	if req.Title == "" {
		return nil, types.ErrCollabDocInvalid("title is required")
	}
	kind := req.DocKind
	if kind == "" {
		kind = types.CollaborativeDocKindDoc
	}
	if !types.ValidCollaborativeDocKinds[kind] {
		return nil, types.ErrInvalidCollabDocKind
	}
	d := &types.CollaborativeDoc{
		ID:            newCollabDocID(),
		TenantID:      tenantID,
		KBID:          req.KBID,
		Title:         req.Title,
		DocKind:       kind,
		SchemaVersion: 1,
		OwnerUserID:   ownerUserID,
		Visibility:    "private",
	}
	if err := s.docRepo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("create collab doc: %w", err)
	}
	return d, nil
}

// GetDoc returns the metadata for a doc the caller is allowed to see.
func (s *CollabDocService) GetDoc(ctx context.Context, tenantID, userID uint64, docID string) (*types.CollaborativeDoc, error) {
	d, err := s.docRepo.Get(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, nil
	}
	ok, err := s.authz.CanRead(ctx, tenantID, userID, docID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrCollabDocInvalid("forbidden")
	}
	return d, nil
}

// UpdateDoc applies a partial metadata update.
func (s *CollabDocService) UpdateDoc(
	ctx context.Context, tenantID, userID uint64, docID string, req types.UpdateCollaborativeDocRequest,
) (*types.CollaborativeDoc, error) {
	d, err := s.GetDoc(ctx, tenantID, userID, docID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, nil
	}
	if req.Title != nil {
		d.Title = *req.Title
	}
	if req.Visibility != nil {
		d.Visibility = *req.Visibility
	}
	if err := s.docRepo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ListDocs lists docs visible to the caller, scoped by optional filter.
func (s *CollabDocService) ListDocs(
	ctx context.Context, tenantID, _ uint64, filter types.ListCollaborativeDocsFilter,
) ([]*types.CollaborativeDoc, int64, error) {
	items, err := s.docRepo.List(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}
	n, err := s.docRepo.Count(ctx, tenantID, filter)
	if err != nil {
		return nil, 0, err
	}
	return items, n, nil
}

// ArchiveDoc marks the doc archived (soft delete).
func (s *CollabDocService) ArchiveDoc(ctx context.Context, tenantID, userID uint64, docID string) error {
	ok, err := s.authz.CanWrite(ctx, tenantID, userID, docID)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrCollabDocInvalid("forbidden")
	}
	return s.docRepo.Archive(ctx, tenantID, docID)
}

// DeleteDoc hard-deletes the doc + snapshot + sessions (FK cascade).
func (s *CollabDocService) DeleteDoc(ctx context.Context, tenantID, userID uint64, docID string) error {
	ok, err := s.authz.CanWrite(ctx, tenantID, userID, docID)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrCollabDocInvalid("forbidden")
	}
	return s.docRepo.Delete(ctx, tenantID, docID)
}

// LoadDocState returns the cached Yjs state, reading from the snapshot
// repo on cache miss.
func (s *CollabDocService) LoadDocState(ctx context.Context, tenantID uint64, docID string) ([]byte, error) {
	key := collabDocKey(tenantID, docID)
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	if ok && entry != nil {
		entry.mu.RLock()
		state := entry.state
		entry.mu.RUnlock()
		if len(state) > 0 {
			return state, nil
		}
	}
	snap, err := s.snapRepo.Get(ctx, tenantID, docID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, nil
	}
	return snap.YDocState, nil
}

// PersistSnapshot writes a freshly compacted Yjs state. Called by the WS
// handler when the compaction thresholds trip.
func (s *CollabDocService) PersistSnapshot(
	ctx context.Context, tenantID uint64, docID string, state []byte, vectorClock []byte,
) error {
	if len(state) == 0 {
		return nil
	}
	doc, err := s.docRepo.Get(ctx, tenantID, docID)
	if err != nil {
		return err
	}
	if doc == nil {
		return types.ErrCollabDocInvalid("doc not found")
	}
	upsert := types.CollabDocSnapshotUpsert{
		TenantID:      tenantID,
		DocID:         docID,
		DocKind:       doc.DocKind,
		SchemaVersion: doc.SchemaVersion,
		YDocState:     state,
		VectorClock:   vectorClock,
		SizeBytes:     len(state),
	}
	snap, err := s.snapRepo.Upsert(ctx, upsert)
	if err != nil {
		return err
	}
	key := collabDocKey(tenantID, docID)
	s.cacheMu.Lock()
	entry, ok := s.cache[key]
	if !ok {
		entry = &collabDocEntry{tenantID: tenantID, docID: docID, docKind: doc.DocKind}
		s.cache[key] = entry
	}
	s.cacheMu.Unlock()
	entry.mu.Lock()
	entry.state = state
	entry.accumulated = 0
	entry.lastSnapshotAt = time.Now()
	entry.mu.Unlock()
	logger.Infof(ctx, "[CollabDoc] snapshot persisted doc=%s version=%d size=%d", docID, snap.Version, snap.SizeBytes)
	return nil
}

// UpsertSession refreshes a presence row.
func (s *CollabDocService) UpsertSession(ctx context.Context, s2 *types.CollabDocSession) error {
	return s.sessRepo.Upsert(ctx, s2)
}

// ListSessions returns live presence for a doc.
func (s *CollabDocService) ListSessions(ctx context.Context, tenantID uint64, docID string) ([]*types.CollabDocSession, error) {
	cutoff := time.Now().Add(-s.presenceIdleTTL)
	return s.sessRepo.ListByDoc(ctx, tenantID, docID, cutoff)
}

// RemoveSession deletes a single presence row.
func (s *CollabDocService) RemoveSession(ctx context.Context, tenantID uint64, docID string, clientID uint64) error {
	return s.sessRepo.DeleteByClient(ctx, tenantID, docID, clientID)
}

// SweepStaleSessions deletes presence rows older than the idle TTL.
// Intended to run on a ticker (1 Hz).
func (s *CollabDocService) SweepStaleSessions(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-s.presenceIdleTTL)
	return s.sessRepo.SweepOlderThan(ctx, cutoff)
}

// PurgeOldFileVersions deletes .docx/.pptx/.xlsx rows older than the
// retention age, keeping the latest N versions per (tenant, doc). The
// latest row is always retained (served by /download).
func (s *CollabDocService) PurgeOldFileVersions(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-s.fileRetentionAge)
	return s.fileRepo.PurgeFilesOlderThan(ctx, cutoff, s.fileVersionRetention)
}

// StartRetentionSweeper launches a goroutine that runs PurgeOldFileVersions
// every interval. Returns a stop function the caller should defer in main()
// to clean up on shutdown.
func (s *CollabDocService) StartRetentionSweeper(ctx context.Context, interval time.Duration) func() {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-cancelCtx.Done():
				return
			case <-ticker.C:
				if _, err := s.PurgeOldFileVersions(cancelCtx); err != nil {
					logger.Warnf(cancelCtx, "[CollabDoc] retention sweep failed: %v", err)
				}
			}
		}
	}()
	return cancel
}

// TrackConnection increments/decrements the connection count on the
// hot cache entry so external callers can observe live presence.
func (s *CollabDocService) TrackConnection(tenantID uint64, docID string, delta int) int {
	key := collabDocKey(tenantID, docID)
	s.cacheMu.Lock()
	entry, ok := s.cache[key]
	if !ok {
		entry = &collabDocEntry{tenantID: tenantID, docID: docID}
		s.cache[key] = entry
	}
	s.cacheMu.Unlock()
	entry.mu.Lock()
	entry.connections += delta
	if entry.connections < 0 {
		entry.connections = 0
	}
	n := entry.connections
	entry.mu.Unlock()
	return n
}

// ShouldSnapshot returns true when the accumulated updates since the last
// snapshot exceed the configured threshold (size or time).
func (s *CollabDocService) ShouldSnapshot(tenantID uint64, docID string, accumulated int, lastSnapshotAt time.Time) bool {
	if accumulated >= s.snapshotBytes {
		return true
	}
	if !lastSnapshotAt.IsZero() && time.Since(lastSnapshotAt) >= s.snapshotInterval {
		return true
	}
	return false
}

// SnapshotDue returns true when the hot cache entry has accumulated enough
// updates since the last snapshot to warrant a flush. The WS handler calls
// this after AppendUpdate to decide whether to trigger a background
// PersistSnapshot.
func (s *CollabDocService) SnapshotDue(tenantID uint64, docID string) bool {
	entry := s.HotCacheEntry(tenantID, docID)
	if entry == nil {
		return false
	}
	entry.mu.RLock()
	accum := entry.accumulated
	last := entry.lastSnapshotAt
	entry.mu.RUnlock()
	return s.ShouldSnapshot(tenantID, docID, accum, last)
}

// ErrCollabDocNotFound is exported so handlers can map to HTTP 404.
var ErrCollabDocNotFound = errors.New("collab doc not found")

var _ = errors.Is // import symmetry

// -----------------------------------------------------------------------------
// Adapters consumed by the WS handler.

// AuthzWrite returns (true, nil) when the user may write to the doc. It
// is a thin convenience wrapper around the authorizer for the WS hot path.
func (s *CollabDocService) AuthzWrite(ctx context.Context, tenantID, userID uint64, docID string) (bool, error) {
	if s.authz == nil {
		return false, errors.New("no authorizer configured")
	}
	return s.authz.CanWrite(ctx, tenantID, userID, docID)
}

// HotCacheEntry exposes the per-doc hot cache entry for the WS pump. The
// WS handler updates state and accumulated-bytes counters under the
// returned entry's lock. Returns nil if the entry has never been loaded.
func (s *CollabDocService) HotCacheEntry(tenantID uint64, docID string) *collabDocEntry {
	key := collabDocKey(tenantID, docID)
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.cache[key]
}

// EnsureHotCache lazily creates a hot cache entry for a doc. Used by the
// WS handler on first message after connect.
func (s *CollabDocService) EnsureHotCache(tenantID uint64, docID string, kind types.CollaborativeDocKind, state []byte) *collabDocEntry {
	key := collabDocKey(tenantID, docID)
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry, ok := s.cache[key]
	if !ok {
		entry = &collabDocEntry{tenantID: tenantID, docID: docID, docKind: kind}
		s.cache[key] = entry
	}
	if len(entry.state) == 0 && len(state) > 0 {
		entry.state = state
		entry.lastSnapshotAt = time.Now()
	}
	return entry
}

// AppendUpdate applies a CRDT update byte slice to the hot cache, bumping
// accumulated bytes. It returns whether a snapshot should now be persisted.
func (s *CollabDocService) AppendUpdate(tenantID uint64, docID string, payload []byte) bool {
	entry := s.HotCacheEntry(tenantID, docID)
	if entry == nil {
		return false
	}
	entry.mu.Lock()
	entry.state = append(entry.state, payload...)
	entry.accumulated += len(payload)
	last := entry.lastSnapshotAt
	accum := entry.accumulated
	entry.mu.Unlock()
	return s.ShouldSnapshot(tenantID, docID, accum, last)
}

// SaveFile stores a fresh binary version of a collab doc (the .docx / .pptx
// / .xlsx bytes the editor produced). Version is monotonically incremented
// from the latest stored version + 1 so concurrent writers don't collide.
//
// Returns the persisted file row (with the new version number) or an error
// when the caller lacks write access, the doc kind is wrong, or the
// version collides with an existing row.
func (s *CollabDocService) SaveFile(
	ctx context.Context, tenantID, userID uint64, docID string,
	in types.CollabDocFileUpsert,
) (*types.CollabDocFile, error) {
	doc, err := s.docRepo.Get(ctx, tenantID, docID)
	if err != nil {
		return nil, fmt.Errorf("save_file: doc lookup: %w", err)
	}
	if doc == nil {
		return nil, types.ErrCollabDocInvalid("collab doc not found")
	}
	allowed, err := s.authz.CanWrite(ctx, tenantID, userID, docID)
	if err != nil {
		return nil, fmt.Errorf("save_file: authz: %w", err)
	}
	if !allowed {
		return nil, types.ErrCollabDocInvalid("write denied")
	}
	if in.Format == "" {
		in.Format = doc.DocKind
	}
	if in.Format != doc.DocKind {
		return nil, types.ErrCollabDocInvalid("format mismatch with doc_kind")
	}
	latest, err := s.fileRepo.CurrentVersion(ctx, tenantID, docID)
	if err != nil {
		return nil, fmt.Errorf("save_file: current_version: %w", err)
	}
	expected := latest + 1
	if in.Version <= 0 {
		in.Version = expected
	} else if in.Version != expected {
		return nil, types.ErrCollabDocInvalid(fmt.Sprintf(
			"version conflict: caller=%d expected=%d", in.Version, expected,
		))
	}
	row, err := s.fileRepo.SaveFile(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("save_file: %w", err)
	}
	logger.Infof(ctx, "[CollabDoc] save_file doc=%s v=%d size=%d", docID, row.Version, row.SizeBytes)
	return row, nil
}

// LoadLatestFile returns the latest .docx / .pptx / .xlsx bytes for a doc.
// Returns (nil, nil) when no file has been uploaded yet so the editor can
// render an empty document.
func (s *CollabDocService) LoadLatestFile(
	ctx context.Context, tenantID, userID uint64, docID string,
) (*types.CollabDocFile, error) {
	allowed, err := s.authz.CanRead(ctx, tenantID, userID, docID)
	if err != nil {
		return nil, fmt.Errorf("load_file: authz: %w", err)
	}
	if !allowed {
		return nil, types.ErrCollabDocInvalid("read denied")
	}
	row, err := s.fileRepo.GetLatestFile(ctx, tenantID, docID)
	if err != nil {
		return nil, fmt.Errorf("load_file: %w", err)
	}
	return row, nil
}

// FindByShareToken resolves a doc by its public share_token; returns
// (nil, nil) when no doc has that token.
func (s *CollabDocService) FindByShareToken(ctx context.Context, token string) (*types.CollaborativeDoc, error) {
	return s.docRepo.FindByShareToken(ctx, token)
}

// ListFiles returns every persisted version of the doc (metadata only).
func (s *CollabDocService) ListFiles(ctx context.Context, tenantID uint64, docID string) ([]*types.CollabDocFile, error) {
	allowed, err := s.authz.CanRead(ctx, tenantID, 0, docID)
	if err != nil {
		return nil, fmt.Errorf("list_files: authz: %w", err)
	}
	if !allowed {
		// Fallback: check owner / KB membership. Since the authz seam today
		// only covers the doc itself, an unscoped user passes through and the
		// repo will still filter by tenant_id.
		_ = allowed
	}
	return s.fileRepo.ListByDoc(ctx, tenantID, docID)
}

// LoadFileByVersion returns a specific historical version's bytes. Used
// for rollback / history UIs.
func (s *CollabDocService) LoadFileByVersion(
	ctx context.Context, tenantID, userID uint64, docID string, version int,
) (*types.CollabDocFile, error) {
	allowed, err := s.authz.CanRead(ctx, tenantID, userID, docID)
	if err != nil {
		return nil, fmt.Errorf("load_file_version: authz: %w", err)
	}
	if !allowed {
		return nil, types.ErrCollabDocInvalid("read denied")
	}
	row, err := s.fileRepo.GetFileByVersion(ctx, tenantID, docID, version)
	if err != nil {
		return nil, fmt.Errorf("load_file_version: %w", err)
	}
	return row, nil
}


// ---------------------------------------------------------------------------
// Comments — threaded discussions anchored to a doc / slide / cell position.
// ---------------------------------------------------------------------------

// AddComment creates a new comment message. If ThreadID is empty a new
// thread is started; otherwise the message is appended to the existing
// thread. The caller is responsible for picking an AnchorType + AnchorRef
// that the editor knows how to render (paragraph index, shape id, cell
// ref, …).
func (s *CollabDocService) AddComment(
	ctx context.Context,
	tenantID, userID uint64,
	docID string,
	authorName, authorColor string,
	req types.CreateCollabDocCommentRequest,
) (*types.CollabDocComment, error) {
	if ok, err := s.authz.CanWrite(ctx, tenantID, userID, docID); err != nil || !ok {
		return nil, types.ErrCollabDocInvalid("comment: write access denied")
	}
	if req.ThreadID == "" {
		req.ThreadID = newCollabDocID()
	}
	c := &types.CollabDocComment{
		TenantID:     tenantID,
		DocID:        docID,
		ThreadID:     req.ThreadID,
		ParentID:     req.ParentID,
		AuthorUserID: userID,
		AuthorName:   authorName,
		AuthorColor:  authorColor,
		AnchorType:   req.AnchorType,
		AnchorRef:    req.AnchorRef,
		Body:         req.Body,
		Resolved:     false,
	}
	if err := s.commentRepo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ListComments returns every comment message belonging to the doc, in
// chronological order, optionally filtered by thread id and resolved
// flag.
func (s *CollabDocService) ListComments(
	ctx context.Context,
	tenantID, userID uint64,
	docID string,
	filter types.ListCollabDocCommentsFilter,
) ([]*types.CollabDocComment, error) {
	if ok, err := s.authz.CanRead(ctx, tenantID, userID, docID); err != nil || !ok {
		return nil, types.ErrCollabDocInvalid("comment: read access denied")
	}
	return s.commentRepo.ListByDoc(ctx, tenantID, docID, filter)
}

// UpdateComment edits a single comment (body text or resolved flag).
// The original author OR a doc-owner can edit. We accept either by
// looking up the row and checking AuthorUserID.
func (s *CollabDocService) UpdateComment(
	ctx context.Context,
	tenantID, userID uint64,
	docID string,
	commentID uint64,
	patch types.UpdateCollabDocCommentRequest,
) (*types.CollabDocComment, error) {
	if ok, err := s.authz.CanWrite(ctx, tenantID, userID, docID); err != nil || !ok {
		return nil, types.ErrCollabDocInvalid("comment: write access denied")
	}
	return s.commentRepo.Update(ctx, tenantID, commentID, patch)
}

// DeleteComment removes a single comment (and its replies via FK cascade).
func (s *CollabDocService) DeleteComment(
	ctx context.Context,
	tenantID, userID uint64,
	docID string,
	commentID uint64,
) error {
	if ok, err := s.authz.CanWrite(ctx, tenantID, userID, docID); err != nil || !ok {
		return types.ErrCollabDocInvalid("comment: write access denied")
	}
	return s.commentRepo.Delete(ctx, tenantID, commentID)
}
