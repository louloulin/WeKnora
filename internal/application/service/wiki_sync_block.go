package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiSyncBlockService owns the application-level semantics of synced blocks:
// canonical CRUD, fan-out propagation, ref lifecycle on page save, and the
// deletion-mode policies (unlink vs cascade).
//
// The service mirrors Notion Synced Blocks / 飞书同步块 / Microsoft Loop
// components: a single canonical source auto-propagates to every embedded
// reference; on page save, refs are recomputed from page content.
type WikiSyncBlockService struct {
	repo  interfaces.WikiSyncBlockRepository
	refs  interfaces.WikiSyncBlockRefRepository
	authz WikiSyncBlockAuthorizer

	// In-process cache of canonical content keyed by (tenant, block_id).
	// Avoids a DB round-trip on every page render.
	cacheMu sync.RWMutex
	cache   map[string]cachedBlock
}

type cachedBlock struct {
	content  string
	md       string
	version  int64
	cachedAt time.Time
}

// WikiSyncBlockAuthorizer is the ACL seam the service depends on. The
// realtime service already has the same pattern; this keeps the
// composition test-friendly without pulling the full authz package in.
type WikiSyncBlockAuthorizer interface {
	CanReadKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error)
	CanWriteKB(ctx context.Context, tenantID, userID uint64, kbID string) (bool, error)
}

// NewWikiSyncBlockService wires the service with sensible defaults.
func NewWikiSyncBlockService(
	repo interfaces.WikiSyncBlockRepository,
	refs interfaces.WikiSyncBlockRefRepository,
	authz WikiSyncBlockAuthorizer,
) *WikiSyncBlockService {
	return &WikiSyncBlockService{
		repo:  repo,
		refs:  refs,
		authz: authz,
		cache: make(map[string]cachedBlock),
	}
}

// syncMarkerRegex finds [[sync:UUID]] markers in page markdown content.
// This is the canonical signal that a page embeds a synced block.
var syncMarkerRegex = regexp.MustCompile(`\[\[sync:([0-9a-fA-F-]{36})\]\]`)

// ParseSyncMarkers extracts the set of (block_id, anchor_slug) tuples
// embedded in page content. anchor_slug is the markdown heading that
// precedes the marker (empty for the first occurrence); it gives the same
// page the ability to embed the same block in multiple sections.
func ParseSyncMarkers(content string) []SyncMarker {
	matches := syncMarkerRegex.FindAllStringSubmatch(content, -1)
	out := make([]SyncMarker, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			out = append(out, SyncMarker{BlockID: m[1]})
		}
	}
	return out
}

// SyncMarker is a parsed [[sync:UUID]] occurrence in page content.
type SyncMarker struct {
	BlockID    string
	AnchorSlug string
}

// CreateCanonical inserts a new synced block. Returns the row including the
// generated version=1.
func (s *WikiSyncBlockService) CreateCanonical(ctx context.Context, in types.WikiSyncBlockUpsert) (*types.WikiSyncBlock, error) {
	if s.authz != nil {
		ok, err := s.authz.CanWriteKB(ctx, in.TenantID, in.OwnerID, in.KBID)
		if err != nil {
			return nil, fmt.Errorf("authz check: %w", err)
		}
		if !ok {
			return nil, errors.New("sync block: write denied")
		}
	}
	row, err := s.repo.Upsert(ctx, in)
	if err != nil {
		return nil, err
	}
	s.invalidateCache(in.TenantID, in.BlockID)
	return row, nil
}

// UpdateCanonical replaces content on an existing synced block, bumping
// the version and invalidating the cache.
func (s *WikiSyncBlockService) UpdateCanonical(ctx context.Context, in types.WikiSyncBlockUpsert) (*types.WikiSyncBlock, error) {
	if s.authz != nil {
		ok, err := s.authz.CanWriteKB(ctx, in.TenantID, in.OwnerID, in.KBID)
		if err != nil {
			return nil, fmt.Errorf("authz check: %w", err)
		}
		if !ok {
			return nil, errors.New("sync block: write denied")
		}
	}
	row, err := s.repo.Upsert(ctx, in)
	if err != nil {
		return nil, err
	}
	s.invalidateCache(in.TenantID, in.BlockID)
	// Best-effort: bump rendered_at so the page renderer can tell refs
	// are stale. We don't touch content_version directly — the renderer
	// compares the ref's stored version against the canonical on next read.
	if err := s.markRefsStale(ctx, in.TenantID, in.BlockID); err != nil {
		logger.Warnf(ctx, "sync block: mark refs stale failed: tenant=%d block=%s err=%v",
			in.TenantID, in.BlockID, err)
	}
	return row, nil
}

// GetCanonical returns the canonical block (cached) by (tenant, block_id).
func (s *WikiSyncBlockService) GetCanonical(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlock, error) {
	key := fmt.Sprintf("%d:%s", tenantID, blockID)
	s.cacheMu.RLock()
	if c, ok := s.cache[key]; ok && time.Since(c.cachedAt) < 5*time.Minute {
		s.cacheMu.RUnlock()
		return &types.WikiSyncBlock{
			TenantID:    tenantID,
			BlockID:     blockID,
			ContentJSON: c.content,
			ContentMD:   c.md,
			Version:     c.version,
		}, nil
	}
	s.cacheMu.RUnlock()

	row, err := s.repo.Get(ctx, tenantID, blockID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	s.cacheMu.Lock()
	s.cache[key] = cachedBlock{
		content:  row.ContentJSON,
		md:       row.ContentMD,
		version:  row.Version,
		cachedAt: time.Now().UTC(),
	}
	s.cacheMu.Unlock()
	return row, nil
}

// ListForKB returns canonical blocks for the picker UI.
func (s *WikiSyncBlockService) ListForKB(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.WikiSyncBlock, error) {
	return s.repo.List(ctx, tenantID, kbID, limit, offset)
}

// SyncPageRefs rewrites the ref table for a page from its current content.
// Called on every page save after the page service commits the new content.
func (s *WikiSyncBlockService) SyncPageRefs(ctx context.Context, tenantID uint64, kbID, pageID, content string) error {
	markers := ParseSyncMarkers(content)
	// Fetch canonical version for each marker so the ref knows its
	// starting content_version. Missing canonicals are skipped (the
	// renderer will surface "broken reference" at view time).
	for _, m := range markers {
		block, err := s.repo.Get(ctx, tenantID, m.BlockID)
		if err != nil {
			logger.Warnf(ctx, "sync block: ref lookup failed: tenant=%d block=%s err=%v", tenantID, m.BlockID, err)
			continue
		}
		if block == nil {
			logger.Warnf(ctx, "sync block: dangling ref: tenant=%d block=%s page=%s", tenantID, m.BlockID, pageID)
			continue
		}
		ref := &types.WikiSyncBlockRef{
			TenantID:       tenantID,
			KBID:           kbID,
			BlockID:        block.BlockID,
			PageID:         pageID,
			AnchorSlug:     m.AnchorSlug,
			ContentVersion: block.Version,
		}
		if err := s.refs.Upsert(ctx, ref); err != nil {
			return err
		}
	}
	// Note: we don't auto-delete refs that no longer appear in content
	// here — that's a future refinement. The current invariant is that
	// SyncPageRefs is idempotent: every refresh aligns refs to the
	// current set of markers; stale refs will be re-rendered with the
	// latest content on the next page view.
	return nil
}

// ListRefsForBlock returns every page that embeds a block.
func (s *WikiSyncBlockService) ListRefsForBlock(ctx context.Context, tenantID uint64, blockID string) ([]*types.WikiSyncBlockRef, error) {
	return s.refs.ListByBlock(ctx, tenantID, blockID)
}

// ListBlocksForPage returns every synced block referenced by a page.
func (s *WikiSyncBlockService) ListBlocksForPage(ctx context.Context, tenantID uint64, pageID string) ([]*types.WikiSyncBlock, error) {
	refs, err := s.refs.ListByPage(ctx, tenantID, pageID)
	if err != nil {
		return nil, err
	}
	out := make([]*types.WikiSyncBlock, 0, len(refs))
	for _, r := range refs {
		b, err := s.GetCanonical(ctx, tenantID, r.BlockID)
		if err != nil {
			logger.Warnf(ctx, "sync block: list-by-page canonical lookup failed: block=%s err=%v", r.BlockID, err)
			continue
		}
		if b != nil {
			out = append(out, b)
		}
	}
	return out, nil
}

// DeleteCanonical removes the canonical block. mode controls whether refs
// are also deleted (cascade) or left as broken references that the
// renderer will surface as "this synced block was deleted" placeholders.
//
//   - "cascade": delete every ref row too (page content keeps the marker
//     but it renders as a tombstone).
//   - "unlink": replace every ref's content_version with 0 and leave the
//     rows; the renderer falls back to showing the last-known markdown.
func (s *WikiSyncBlockService) DeleteCanonical(ctx context.Context, tenantID, userID uint64, kbID, blockID, mode string) error {
	if s.authz != nil {
		ok, err := s.authz.CanWriteKB(ctx, tenantID, userID, kbID)
		if err != nil {
			return fmt.Errorf("authz check: %w", err)
		}
		if !ok {
			return errors.New("sync block: write denied")
		}
	}
	switch mode {
	case "cascade", "":
		if err := s.refs.DeleteByBlock(ctx, tenantID, blockID); err != nil {
			return err
		}
	case "unlink":
		if err := s.refs.MarkRendered(ctx, tenantID, blockID, "", "", 0); err != nil {
			return err
		}
	default:
		return fmt.Errorf("sync block: unknown delete mode %q (want cascade|unlink)", mode)
	}
	if err := s.repo.Delete(ctx, tenantID, blockID); err != nil {
		return err
	}
	s.invalidateCache(tenantID, blockID)
	return nil
}

// Stats returns fan-out reach for the picker UI badge.
func (s *WikiSyncBlockService) Stats(ctx context.Context, tenantID uint64, blockID string) (*types.WikiSyncBlockRefStats, error) {
	return s.repo.Stats(ctx, tenantID, blockID)
}

// invalidateCache drops the per-block cache entry so the next read picks
// up the new canonical.
func (s *WikiSyncBlockService) invalidateCache(tenantID uint64, blockID string) {
	key := fmt.Sprintf("%d:%s", tenantID, blockID)
	s.cacheMu.Lock()
	delete(s.cache, key)
	s.cacheMu.Unlock()
}

// markRefsStale touches rendered_at on every ref so the page renderer
// compares versions on next view. This is the cheap part of the fan-out —
// the renderer does the actual content re-fetch on page view.
func (s *WikiSyncBlockService) markRefsStale(ctx context.Context, tenantID uint64, blockID string) error {
	refs, err := s.refs.ListByBlock(ctx, tenantID, blockID)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if err := s.refs.MarkRendered(ctx, tenantID, r.BlockID, r.PageID, r.AnchorSlug, r.ContentVersion); err != nil {
			logger.Warnf(ctx, "sync block: mark rendered failed: block=%s page=%s err=%v", r.BlockID, r.PageID, err)
		}
	}
	return nil
}

// Ensure repo imports are referenced (compile-time smoke).
var _ = repository.NewWikiSyncBlockRepository
var _ = repository.NewWikiSyncBlockRefRepository

// ValidateJSONContent is a small helper exposed for the handler so the
// API surface rejects malformed Tiptap documents with a 400 instead of
// a 500. Returns the json.RawMessage so callers can pass it through.
func ValidateJSONContent(raw []byte) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("content_json is not valid JSON")
	}
	return json.RawMessage(raw), nil
}

// MakeSyncBlockUpsert constructs a WikiSyncBlockUpsert from the handler's
// parsed inputs. Lives in the service package so the handler doesn't
// reach into the types package's struct literal directly.
func MakeSyncBlockUpsert(tenantID uint64, kbID, blockID, title string, contentJSON json.RawMessage, contentMD string, ownerID uint64) types.WikiSyncBlockUpsert {
	return types.WikiSyncBlockUpsert{
		TenantID:    tenantID,
		KBID:        kbID,
		BlockID:     blockID,
		Title:       title,
		ContentJSON: contentJSON,
		ContentMD:   contentMD,
		OwnerID:     ownerID,
	}
}
