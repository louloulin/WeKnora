package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// wikiLinkRegex matches [[wiki-link]] syntax in markdown content
var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// wikiInlineChunkCitationRegex matches internal short handles emitted by the
// wiki ingest prompt while it classifies supporting chunks. The stable source
// relationship lives in WikiPage.ChunkRefs, so these handles have no meaning
// to readers and must not leak into generated Markdown.
var wikiInlineChunkCitationRegex = regexp.MustCompile(`[ \t]*\[c\d{3,}(?:\s*[,;]\s*c\d{3,})*\]`)

func stripWikiInlineChunkCitations(content string) string {
	return wikiInlineChunkCitationRegex.ReplaceAllString(content, "")
}

func stripWikiPageInlineChunkCitations(page *types.WikiPage) {
	if page == nil {
		return
	}
	page.Content = stripWikiInlineChunkCitations(page.Content)
	page.Summary = stripWikiInlineChunkCitations(page.Summary)
}

// wikiPageService implements the WikiPageService interface
type wikiPageService struct {
	repo            interfaces.WikiPageRepository
	chunkRepo       interfaces.ChunkRepository
	kbService       interfaces.KnowledgeBaseService
	taskPendingRepo interfaces.TaskPendingOpsRepository
	redisClient     *redis.Client
	aclService      WikiAclService
	// batchSvc is set post-construction via SetBatchJobService to break
	// the chicken-and-egg between WikiPageService (which builds job
	// params) and WikiBatchJobService (which executes them). nil means
	// the three Batch* methods always run synchronously — older callers
	// (tests) keep working unchanged.
	batchSvc interfaces.WikiBatchJobService
	// cacheRepo backs the Build #21 backlinks graph cache. nil disables
	// caching — every ListBacklinkGraph call recomputes, and the
	// write-time hooks become silent no-ops. Production wires a real
	// instance via SetBacklinksCacheRepo; older callers (Build #20 +
	// earlier tests) keep working unchanged.
	cacheRepo interfaces.WikiBacklinksCacheRepository
	// cacheInvalidator encapsulates the slug-resolution policy for
	// cache wipes (which slugs to wipe for which write op). Lives
	// behind its own interface so tests can swap a stub.
	cacheInvalidator interfaces.WikiBacklinksCacheInvalidator
	// kbReferenceBackfiller (Build v0.7.13) reconciles wiki_kb_references
	// against [[kb:id]] mentions in the page content after every save.
	// Wired post-construction via SetKBReferenceBackfiller to mirror the
	// cacheInvalidator pattern; nil when the backfill feature is off.
	kbReferenceBackfiller WikiKBReferenceBackfiller
}

// SetBatchJobService wires the async batch service post-construction.
// Avoids a circular initialiser between wikiPageService.New (which
// needs to enqueue jobs) and WikiBatchJobService.New (which needs the
// page service to execute sync methods).
//
// Build #13.
func (s *wikiPageService) SetBatchJobService(svc interfaces.WikiBatchJobService) {
	s.batchSvc = svc
}

// SetBacklinksCacheRepo wires the Build #21 backlinks cache repository
// post-construction. Mirrors the SetBatchJobService pattern (DI module
// registers them after both services have been constructed to break the
// circular dependency that would arise from a direct constructor arg).
func (s *wikiPageService) SetBacklinksCacheRepo(repo interfaces.WikiBacklinksCacheRepository) {
	s.cacheRepo = repo
}

// SetBacklinksCacheInvalidator wires the slug-resolution policy for the
// Build #21 cache wipe hooks. Same post-construction pattern as
// SetBacklinksCacheRepo.
func (s *wikiPageService) SetBacklinksCacheInvalidator(inv interfaces.WikiBacklinksCacheInvalidator) {
	s.cacheInvalidator = inv
}

// SetKBReferenceBackfiller wires the doc+KB backfill hook. Called from
// the DI container after both services are constructed. nil disables
// the backfill (the wiki page save still succeeds).
func (s *wikiPageService) SetKBReferenceBackfiller(b WikiKBReferenceBackfiller) {
	s.kbReferenceBackfiller = b
}

// NewWikiPageService creates a new wiki page service.
//
// aclService may be nil — older callers (and a couple of legacy tests)
// construct the service without it, in which case the read paths skip the
// ACL gate. Production always wires a non-nil value via the DI container.
func NewWikiPageService(
	repo interfaces.WikiPageRepository,
	chunkRepo interfaces.ChunkRepository,
	kbService interfaces.KnowledgeBaseService,
	taskPendingRepo interfaces.TaskPendingOpsRepository,
	redisClient *redis.Client,
	aclService WikiAclService,
) interfaces.WikiPageService {
	return &wikiPageService{
		repo:            repo,
		chunkRepo:       chunkRepo,
		kbService:       kbService,
		taskPendingRepo: taskPendingRepo,
		redisClient:     redisClient,
		aclService:      aclService,
	}
}

// CreatePage creates a new wiki page
func (s *wikiPageService) CreatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error) {
	if page.ID == "" {
		page.ID = uuid.New().String()
	}
	if page.Slug == "" {
		return nil, errors.New("wiki page slug is required")
	}
	if page.KnowledgeBaseID == "" {
		return nil, errors.New("knowledge_base_id is required")
	}
	if page.Status == "" {
		page.Status = types.WikiPageStatusPublished
	}
	if page.Version == 0 {
		page.Version = 1
	}
	page.LastEditSource = types.WikiEditSourceFromContext(ctx)
	page.LastEditorID, _ = types.UserIDFromContext(ctx)
	stripWikiPageInlineChunkCitations(page)

	// Parse outbound links from content
	page.OutLinks = s.parseOutLinks(page.Content)
	if err := s.applyFolderToPage(ctx, page); err != nil {
		return nil, err
	}
	normalizeWikiHierarchy(page)

	// Build #19.x — write-time jieba tokenization backs `content_ts_zh`
	// (migration 000096) so the v2 search repo's `@@ plainto_tsquery('simple',
	// $jieba)` arm can hit Chinese queries.
	page.ContentTSZh = JiebaSegmentForSearch(page.Title, page.Content)

	now := time.Now()
	page.CreatedAt = now
	page.UpdatedAt = now

	if err := s.repo.Create(ctx, page); err != nil {
		return nil, fmt.Errorf("create wiki page: %w", err)
	}

	// Update inbound links on target pages
	s.updateInLinks(ctx, page.KnowledgeBaseID, page.Slug, page.OutLinks)

	// Build #21 — the new page may have changed which slugs resolve
	// to it as a backlink target; wipe the cache for [self] ∪ out_links
	// so the next panel read recomputes direct + indirect counts.
	if s.cacheRepo != nil && s.cacheInvalidator != nil {
		slugs, _ := s.resolveBacklinkInvalidation(ctx,
			types.BacklinkCacheInvalidateCreatePage,
			page.KnowledgeBaseID, page.Slug)
		{
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          page.KnowledgeBaseID,
				Op:            types.BacklinkCacheInvalidateCreatePage,
				AffectedSlugs: slugs,
			})
		}
	}

	return page, nil
}

// resolveBacklinkInvalidation resolves the affected-slug set for an op.
// Wraps the invalidator's Resolve so the write-time hooks can stay
// single-line. Returns the slug list and the picked SlugSetStrategy.
//
// Build #21.
// Build #28 — return tuple now carries strategy; the underlying
// Resolve panics on unknown ops (D1) so any caller that passes an
// unregistered op crashes here, not in production.
func (s *wikiPageService) resolveBacklinkInvalidation(
	ctx context.Context, op types.BacklinkCacheInvalidateOp, kbID, slug string,
) ([]string, types.SlugSetStrategy) {
	slugs, strategy, _ := s.cacheInvalidator.Resolve(ctx, op, kbID, slug)
	return slugs, strategy
}

// Version bump policy: the `version` column is intended to track the user-
// visible content revision, not every row rewrite. We therefore bump it only
// when at least one of the user-facing fields actually changes — title,
// content, summary, page_type, or status. Bookkeeping-only writes (refreshing
// source_refs after re-ingest when the body is identical, rebuilding the index
// page with the same directory, cross-link injection that ends up replacing
// nothing, etc.) still persist through `UpdateMeta` but leave `version`
// untouched so consumers can treat a bump as a real edit signal.
func (s *wikiPageService) UpdatePage(ctx context.Context, page *types.WikiPage) (*types.WikiPage, error) {
	existing, err := s.repo.GetBySlug(ctx, page.KnowledgeBaseID, page.Slug)
	if err != nil {
		return nil, fmt.Errorf("get existing page: %w", err)
	}
	stripWikiPageInlineChunkCitations(page)

	oldOutLinks := existing.OutLinks

	// Snapshot user-visible fields BEFORE mutation so we can decide whether
	// this is a real content change or just bookkeeping.
	contentChanged := existing.Title != page.Title ||
		existing.Content != page.Content ||
		existing.Summary != page.Summary ||
		existing.PageType != page.PageType ||
		existing.Status != page.Status ||
		!slices.Equal(existing.Aliases, page.Aliases)

	// Keep an unmutated copy of the version being replaced: it becomes the
	// revision snapshot when this turns out to be a real content change.
	prev := *existing

	existing.Title = page.Title
	existing.Content = page.Content
	existing.Summary = page.Summary
	existing.PageType = page.PageType
	existing.Aliases = append(types.StringArray(nil), page.Aliases...)
	existing.SourceRefs = page.SourceRefs
	existing.ChunkRefs = page.ChunkRefs
	existing.PageMetadata = page.PageMetadata
	existing.ParentSlug = page.ParentSlug
	existing.FolderID = page.FolderID
	existing.SortOrder = page.SortOrder
	existing.Status = page.Status
	existing.UpdatedAt = time.Now()

	// CategoryPath is a derived cache of FolderID — recompute it from the
	// folder chain rather than trusting whatever the caller sent.
	if err := s.applyFolderToPage(ctx, existing); err != nil {
		return nil, err
	}

	// Outbound links are a pure derivative of content, so they only shift
	// when content shifts. Re-parse unconditionally to stay consistent with
	// the stored body.
	existing.OutLinks = s.parseOutLinks(existing.Content)
	normalizeWikiHierarchy(existing)

	// Build #19.x — re-tokenize jieba whenever the user-visible content or
	// title changes so `content_ts_zh` stays in sync with the body. The
	// helper is a no-op when content/title are identical to `existing` so
	// the bookkeeping-only branch below remains cheap.
	existing.ContentTSZh = JiebaSegmentForSearch(existing.Title, existing.Content)

	if contentChanged {
		// The new version is authored by whoever is driving this write.
		existing.LastEditSource = types.WikiEditSourceFromContext(ctx)
		existing.LastEditorID, _ = types.UserIDFromContext(ctx)

		// Snapshot the superseded version and write the new one atomically,
		// so the content of every past version is preserved and a failed
		// update leaves no snapshot behind.
		if err := s.repo.UpdateWithRevision(ctx, existing, revisionFromPage(&prev)); err != nil {
			return nil, fmt.Errorf("update wiki page: %w", err)
		}
		// Bound per-page history; best-effort — a failed prune only means
		// slightly more storage until the next content change.
		s.pruneRevisions(ctx, existing.ID, existing.Version)
	} else {
		// No user-visible change — persist bookkeeping fields but preserve
		// the version so downstream consumers can rely on it.
		if err := s.repo.UpdateMeta(ctx, existing); err != nil {
			return nil, fmt.Errorf("update wiki page meta: %w", err)
		}
	}

	// Update inbound links: remove old, add new. If content didn't change,
	// oldOutLinks == existing.OutLinks and these calls are effectively no-ops.
	s.removeInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, oldOutLinks)
	s.updateInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, existing.OutLinks)

	// Doc + KB integration (Build v0.7.13): reconcile wiki_kb_references
	// against [[kb:id]] mentions in the new content. Best-effort —
	// a failed reconcile never blocks the save (the wiki content is the
	// source of truth, the references row set is a derived index).
	if s.kbReferenceBackfiller != nil {
		actor, _ := types.UserIDFromContext(ctx)
		s.kbReferenceBackfiller.ReconcileAfterSave(ctx,
			strconv.FormatUint(existing.TenantID, 10), existing.ID, existing.Content, actor)
	}

	// Build #21 — invalidate the backlinks graph cache for [self] ∪
	// out_links so the next panel read sees fresh direct + indirect
	// counts (old targets lost [slug] in removeInLinks above).
	if s.cacheRepo != nil && s.cacheInvalidator != nil {
		if slugs, _ := s.resolveBacklinkInvalidation(ctx,
			types.BacklinkCacheInvalidateUpdatePage,
			existing.KnowledgeBaseID, existing.Slug); err == nil {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          existing.KnowledgeBaseID,
				Op:            types.BacklinkCacheInvalidateUpdatePage,
				AffectedSlugs: slugs,
			})
		}
	}

	return existing, nil
}

// UpdatePageMeta updates only metadata (status, source_refs) without version bump or link re-parse.
func (s *wikiPageService) UpdatePageMeta(ctx context.Context, page *types.WikiPage) error {
	normalizeWikiHierarchy(page)
	page.UpdatedAt = time.Now()
	return s.repo.UpdateMeta(ctx, page)
}

// UpdateAutoLinkedContent persists content produced by machine-only link
// decorators (cross-link injection / dead-link cleanup) without bumping
// `version`. Out-links are re-parsed from the new body and bidirectional
// in-link references on target pages are refreshed so link navigation stays
// consistent — only the user-facing revision counter is preserved.
func (s *wikiPageService) UpdateAutoLinkedContent(ctx context.Context, page *types.WikiPage) error {
	existing, err := s.repo.GetBySlug(ctx, page.KnowledgeBaseID, page.Slug)
	if err != nil {
		return fmt.Errorf("get existing page: %w", err)
	}

	oldOutLinks := existing.OutLinks

	existing.Content = stripWikiInlineChunkCitations(page.Content)
	existing.OutLinks = s.parseOutLinks(existing.Content)
	existing.ContentTSZh = JiebaSegmentForSearch(existing.Title, existing.Content)
	existing.UpdatedAt = time.Now()

	if err := s.repo.UpdateAutoLinkedContent(ctx, existing); err != nil {
		return fmt.Errorf("update auto-linked content: %w", err)
	}

	s.removeInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, oldOutLinks)
	s.updateInLinks(ctx, existing.KnowledgeBaseID, existing.Slug, existing.OutLinks)

	return nil
}

// revisionFromPage builds the immutable snapshot row for the given page
// state. EditSource on the snapshot is the author of THAT version — the
// page's provenance columns as they stood while the version was current —
// not whoever is performing the write that supersedes it.
func revisionFromPage(p *types.WikiPage) *types.WikiPageRevision {
	return &types.WikiPageRevision{
		ID:              uuid.New().String(),
		TenantID:        p.TenantID,
		KnowledgeBaseID: p.KnowledgeBaseID,
		PageID:          p.ID,
		Slug:            p.Slug,
		Version:         p.Version,
		Title:           p.Title,
		PageType:        p.PageType,
		Status:          p.Status,
		Content:         p.Content,
		Summary:         p.Summary,
		Aliases:         append(types.StringArray(nil), p.Aliases...),
		EditSource:      types.NormalizeWikiEditSource(p.LastEditSource),
		EditorID:        p.LastEditorID,
		EditedAt:        p.UpdatedAt,
		CreatedAt:       time.Now(),
	}
}

// pruneRevisions bounds one page's snapshot history after it advanced to
// currentVersion. Machine-authored snapshots are dropped once they fall out
// of the recent window; human/agent/revert ones survive until the hard cap,
// so pipeline churn on a hot page cannot evict the edits users care about.
func (s *wikiPageService) pruneRevisions(ctx context.Context, pageID string, currentVersion int) {
	req := types.WikiRevisionPruneRequest{
		PageID:              pageID,
		KeepFromVersion:     currentVersion - types.WikiMaxRevisionsPerPage,
		PrunableSources:     types.WikiPrunableEditSources,
		HardKeepFromVersion: currentVersion - types.WikiMaxRevisionsHardCap,
	}
	if req.KeepFromVersion <= 0 && req.HardKeepFromVersion <= 0 {
		return
	}
	if err := s.repo.PruneRevisions(ctx, req); err != nil {
		logger.Warnf(ctx, "prune wiki page revisions for %s failed: %v", pageID, err)
	}
}

// ErrWikiRevertToCurrentVersion is returned when a revert targets the version
// the page is already on — a client-side mistake (usually a stale history
// list), not a server fault, so handlers map it to 400.
var ErrWikiRevertToCurrentVersion = errors.New("cannot revert to the current version")

// ListRevisions returns the stored historical snapshots for a page (newest
// first, content omitted) plus the page's current version.
func (s *wikiPageService) ListRevisions(
	ctx context.Context, kbID string, slug string, limit int, offset int,
) (*types.WikiPageRevisionListResponse, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	revs, total, err := s.repo.ListRevisions(ctx, kbID, page.ID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list wiki page revisions: %w", err)
	}
	return &types.WikiPageRevisionListResponse{
		Revisions:      revs,
		Total:          total,
		CurrentVersion: page.Version,
	}, nil
}

// GetRevision returns one historical snapshot with content.
func (s *wikiPageService) GetRevision(
	ctx context.Context, kbID string, slug string, version int,
) (*types.WikiPageRevision, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	return s.repo.GetRevision(ctx, kbID, page.ID, version)
}

// RevertPageToVersion rolls the page back to a stored revision by applying
// that revision's content fields as a regular edit: the pre-revert state is
// snapshotted, version advances, links are re-parsed. Placement (folder,
// sort order) and provenance refs keep their current values — a revert is
// about content, not about undoing directory moves.
func (s *wikiPageService) RevertPageToVersion(
	ctx context.Context, kbID string, slug string, version int,
) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if version == page.Version {
		return nil, ErrWikiRevertToCurrentVersion
	}
	rev, err := s.repo.GetRevision(ctx, kbID, page.ID, version)
	if err != nil {
		return nil, err
	}

	target := *page
	target.Title = rev.Title
	target.Content = rev.Content
	target.Summary = rev.Summary
	target.PageType = rev.PageType
	target.Status = rev.Status
	target.Aliases = append(types.StringArray(nil), rev.Aliases...)

	return s.UpdatePage(types.WithWikiEditSource(ctx, types.WikiEditSourceRevert), &target)
}

// GetPageBySlug retrieves a wiki page by its slug
func (s *wikiPageService) GetPageBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if err := s.gateWikiPageAccess(ctx, kbID, slug); err != nil {
		return nil, err
	}
	stripWikiPageInlineChunkCitations(page)
	return page, nil
}

// GetPageByID retrieves a wiki page by its ID
func (s *wikiPageService) GetPageByID(ctx context.Context, id string) (*types.WikiPage, error) {
	page, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	stripWikiPageInlineChunkCitations(page)
	return page, nil
}

// ListPages lists wiki pages with optional filtering and pagination
func (s *wikiPageService) ListPages(ctx context.Context, req *types.WikiPageListRequest) (*types.WikiPageListResponse, error) {
	pages, total, err := s.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}
	// Filter out pages the caller cannot read per ACL. The total stays at
	// the repo count — we don't want directory pagination to shuffle when
	// an admin toggles someone's permissions mid-scroll.
	visible := s.filterReadablePages(ctx, req.KnowledgeBaseID, pages)
	for _, page := range visible {
		stripWikiPageInlineChunkCitations(page)
		normalizeWikiHierarchy(page)
	}

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &types.WikiPageListResponse{
		Pages:      visible,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// DeletePage soft-deletes a wiki page
func (s *wikiPageService) DeletePage(ctx context.Context, kbID string, slug string) error {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return err
	}

	// Remove inbound link references from pages this page links to
	s.removeInLinks(ctx, kbID, slug, page.OutLinks)

	// Delete the page
	if err := s.repo.Delete(ctx, kbID, slug); err != nil {
		return err
	}

	// Drop the snapshot history too: the page is gone from every read path,
	// so its revisions are unreachable rows holding full content bodies.
	// Best-effort — the page is already deleted and cannot be rolled back.
	if err := s.repo.DeleteRevisionsByPage(ctx, page.ID); err != nil {
		logger.Warnf(ctx, "delete wiki page revisions for %s failed: %v", page.ID, err)
	}

	// Delete synced chunk
	s.deleteChunkForPage(ctx, page)

	// Build #21 — the page is gone, so its own cache row is stale and
	// every page that used to link to it lost a backlink source. Wipe
	// [self] ∪ in_links so the next panel read recomputes from the
	// remaining graph (removeInLinks above already trimmed the targets).
	if s.cacheRepo != nil && s.cacheInvalidator != nil {
		if slugs, _ := s.resolveBacklinkInvalidation(ctx,
			types.BacklinkCacheInvalidateDeletePage,
			kbID, slug); err == nil {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          kbID,
				Op:            types.BacklinkCacheInvalidateDeletePage,
				AffectedSlugs: slugs,
			})
		}
	}

	return nil
}

// RestoreDeletedPage undoes a soft-delete by clearing deleted_at and
// rewriting the slug. Used exclusively by the Build #13 undo-of-batch-
// delete path: the caller (WikiBatchJobService.undoDelete) computes the
// suffix-augmented slug so we never have to invent one ourselves.
//
// We deliberately do not touch the linked chunks here — the chunk was
// deleted alongside the page in DeletePage, and re-syncing 100 pages
// inside one undo is out of scope. Subsequent re-ingest of the source
// document will repopulate search. The page remains visible in the
// wiki-browser list immediately because the soft-delete filter is
// dropped.
//
// Build #13.
func (s *wikiPageService) RestoreDeletedPage(
	ctx context.Context, kbID string, originalSlug string, newSlug string,
) (*types.WikiPage, error) {
	if newSlug == "" || newSlug == originalSlug {
		return nil, fmt.Errorf("restore slug must differ from original (%q)", originalSlug)
	}
	if err := s.repo.RestoreDeleted(ctx, kbID, originalSlug, newSlug); err != nil {
		return nil, err
	}
	return s.repo.GetBySlug(ctx, kbID, newSlug)
}

// GetIndex returns the index page for a knowledge base
func (s *wikiPageService) GetIndex(ctx context.Context, kbID string) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, "index")
	if err != nil {
		if errors.Is(err, repository.ErrWikiPageNotFound) {
			// Create default index page
			return s.createDefaultPage(ctx, kbID, "index", "Index", types.WikiPageTypeIndex,
				"# Wiki Index\n\nThis is the index page. It will be automatically updated as pages are added.\n")
		}
		return nil, err
	}
	if err := s.gateWikiPageAccess(ctx, kbID, "index"); err != nil {
		return nil, err
	}
	return page, nil
}

// gateWikiPageAccess consults the per-page ACL for one page read. Returns
// nil when ACL is unset (legacy build path) or when the decision is allow.
// Maps both deny outcomes to repository.ErrWikiPageNotFound so the handler
// layer can translate both into a single 404 — the spec mandates that
// private/allow_list mismatches never leak page existence.
func (s *wikiPageService) gateWikiPageAccess(ctx context.Context, kbID string, slug string) error {
	if s.aclService == nil {
		return nil
	}
	userID, _ := types.UserIDFromContext(ctx)
	decision, err := s.aclService.Resolve(ctx, kbID, slug, userID)
	if err != nil {
		return err
	}
	switch decision {
	case types.WikiPageAclAllow, "":
		return nil
	case types.WikiPageAclDenyPrivate, types.WikiPageAclDenyAllowList:
		return repository.ErrWikiPageNotFound
	default:
		// Unknown decision value: be conservative and deny. The service
		// contract guarantees only the constants above, so this branch
		// should never fire in practice — it exists purely so a stale
		// or hand-mutated implementation cannot accidentally allow.
		return repository.ErrWikiPageNotFound
	}
}

// filterReadablePages applies the ACL decision to every row of a list
// result. Rows that fail the gate are dropped silently so a private page
// doesn't disappear from the row count (the UI relies on Total being
// stable mid-paging). When aclService is nil the input is returned as-is.
func (s *wikiPageService) filterReadablePages(ctx context.Context, kbID string, pages []*types.WikiPage) []*types.WikiPage {
	if s.aclService == nil || len(pages) == 0 {
		return pages
	}
	out := make([]*types.WikiPage, 0, len(pages))
	for _, page := range pages {
		if page == nil || page.Slug == "" {
			continue
		}
		decision, err := s.aclService.Resolve(ctx, kbID, page.Slug, "")
		if err != nil {
			logger.Warnf(ctx, "wiki acl list filter: resolve %s failed: %v", page.Slug, err)
			continue
		}
		if decision == types.WikiPageAclAllow || decision == "" {
			out = append(out, page)
		}
	}
	return out
}

// wikiIndexContentPageTypes enumerates the page types that make up a wiki's
// user-visible directory. The index page is excluded; any
// LLM-created type we do not recognize surfaces under a generic "other"
// bucket.
var wikiIndexContentPageTypes = []string{
	types.WikiPageTypeSummary,
	types.WikiPageTypeEntity,
	types.WikiPageTypeConcept,
	types.WikiPageTypeSynthesis,
	types.WikiPageTypeComparison,
}

// GetIndexView builds the structured index response without ever
// materializing a multi-MB directory markdown string. Intro is read from
// the index wiki_page row (which now carries only intro text — see
// rebuildIndexPage). Each requested page_type is paginated independently
// with ListByTypeLight so reads stay O(page_size) rather than O(total
// pages in the KB).
//
// `pageTypes` narrows which groups to include; empty = all content types.
// `limit` is the per-group window size (defaults to 50, capped at 200).
// `cursor` is an opaque offset string; currently we use the stringified
// offset so clients can resume where they left off. Because different
// page_types paginate independently, `cursor` applies uniformly to every
// group — if the caller wants per-group cursors it should request one
// type at a time via `pageTypes`. That simplifies the wire format and
// matches the frontend's tabbed UX.
func (s *wikiPageService) GetIndexView(
	ctx context.Context,
	kbID string,
	pageTypes []string,
	limit int,
	cursor string,
) (*types.WikiIndexResponse, error) {
	indexPage, err := s.GetIndex(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("load index page: %w", err)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := 0
	if cursor != "" {
		v, parseErr := strconv.Atoi(cursor)
		if parseErr != nil || v < 0 {
			return nil, fmt.Errorf("invalid cursor %q", cursor)
		}
		offset = v
	}

	// Default to every known content type when the caller passes no
	// filter. Any unknown request-time type is passed through verbatim so
	// future page types (declared in types/wiki_page.go) start showing
	// up in the index the moment the LLM starts creating them, without a
	// handler change.
	selected := pageTypes
	if len(selected) == 0 {
		selected = append([]string{}, wikiIndexContentPageTypes...)
	}

	groups := make([]types.WikiIndexGroup, 0, len(selected))
	for _, pt := range selected {
		entries, total, listErr := s.repo.ListByTypeLight(ctx, kbID, pt, limit, offset)
		if listErr != nil {
			return nil, fmt.Errorf("list %s pages: %w", pt, listErr)
		}
		if entries == nil {
			entries = []types.WikiIndexEntry{}
		}
		for i := range entries {
			normalizeWikiIndexEntryHierarchy(&entries[i], pt)
		}
		next := ""
		// Only emit a cursor when a full page was returned AND more rows
		// remain past `offset + limit`. A short page or one that exactly
		// consumed the remainder should signal end-of-feed.
		if len(entries) == limit && int64(offset+len(entries)) < total {
			next = strconv.Itoa(offset + limit)
		}
		groups = append(groups, types.WikiIndexGroup{
			Type:       pt,
			Total:      total,
			Items:      entries,
			NextCursor: next,
		})
	}

	// The intro used to be stored on indexPage.Summary while
	// indexPage.Content held intro + directory markdown. After the
	// directory was lifted out of wiki_pages the content column holds
	// only the intro. Fall back to Summary for KBs that haven't been
	// re-ingested since the change so the response is never blank.
	intro := indexPage.Content
	if strings.TrimSpace(intro) == "" {
		intro = indexPage.Summary
	}

	return &types.WikiIndexResponse{
		Intro:   intro,
		Version: indexPage.Version,
		Groups:  groups,
	}, nil
}

// GetGraph returns a slice of the wiki link graph for visualization.
//
// Two modes are supported:
//
//   - WikiGraphModeOverview (default): returns the top `Limit` pages sorted
//     by link_count (in+out), plus every edge that connects two surviving
//     nodes. This is what the frontend fetches on the first graph open —
//     4万-page wikis would otherwise ship ~30MB of JSON and crash the
//     browser trying to render 100k SVG elements.
//
//   - WikiGraphModeEgo: returns the BFS neighborhood of `Center` up to
//     `Depth` undirected hops, capped at `Limit` total nodes. The
//     frontend uses this to drill down when the user clicks / searches a
//     node in the overview.
//
// `Types` is an optional page_type allow-list applied to both the candidate
// node set and (in ego mode) the frontier expansion. Leaving it empty means
// no type filter.
//
// `Limit <= 0` disables the cap entirely and is reserved for internal
// callers like the lint service that need to walk every page. The HTTP
// handler always clamps Limit into a safe range so external traffic can
// never opt out of truncation.
//
// Implementation note: pages are still fetched via repo.ListAll. At 4万
// pages that's ~10MB of rows + deserialization, which is already on the
// expensive side but still tractable and keeps the repository interface
// unchanged. Pushing the filter/top-N down into SQL is a follow-up step
// (cache layer + DB-side projection) — see CLAUDE.md plan.
func (s *wikiPageService) GetGraph(ctx context.Context, req *types.WikiGraphRequest) (*types.WikiGraphData, error) {
	if req == nil {
		return nil, errors.New("wiki graph request is required")
	}

	pages, err := s.repo.ListAll(ctx, req.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return computeGraphSubset(pages, req)
}

// computeGraphSubset is the pure I/O-free core of GetGraph. It takes the
// full page list and a request description and returns the subgraph the
// caller asked for. Extracted from GetGraph so tests can exercise the
// mode/limit/type-filter behavior without plumbing a full repository mock.
func computeGraphSubset(pages []*types.WikiPage, req *types.WikiGraphRequest) (*types.WikiGraphData, error) {
	mode := req.Mode
	if mode == "" {
		mode = types.WikiGraphModeOverview
	}

	// Pre-compute link_count and the type allow-list used for candidate
	// filtering. We keep the full page list around so ego mode can still
	// traverse through neighbors whose type is in the allow-list.
	typeAllow := make(map[string]bool, len(req.Types))
	for _, t := range req.Types {
		if t != "" {
			typeAllow[t] = true
		}
	}
	hasTypeFilter := len(typeAllow) > 0

	familiarSet := make(map[string]struct{}, len(req.FamiliarKnowledgeIDs))
	for _, id := range req.FamiliarKnowledgeIDs {
		if id = strings.TrimSpace(id); id != "" {
			familiarSet[id] = struct{}{}
		}
	}

	pageBySlug := make(map[string]*types.WikiPage, len(pages))
	linkCount := make(map[string]int, len(pages))
	for _, p := range pages {
		pageBySlug[p.Slug] = p
		linkCount[p.Slug] = len(p.InLinks) + len(p.OutLinks)
	}

	// Select the node slug set for the requested slice.
	var selected map[string]struct{}
	switch mode {
	case types.WikiGraphModeEgo:
		if req.Center == "" {
			return nil, errors.New("ego graph requires a center slug")
		}
		if _, ok := pageBySlug[req.Center]; !ok {
			return nil, fmt.Errorf("ego center slug %q not found", req.Center)
		}
		depth := req.Depth
		if depth < 1 {
			depth = 1
		}
		selected = bfsEgoSlugs(pageBySlug, req.Center, depth, typeAllow, req.Limit)
	default:
		// overview: keep only type-allowed candidates, sort by link_count desc, cap.
		candidates := make([]*types.WikiPage, 0, len(pages))
		for _, p := range pages {
			if hasTypeFilter && !typeAllow[p.PageType] {
				continue
			}
			candidates = append(candidates, p)
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			li := linkCount[candidates[i].Slug]
			lj := linkCount[candidates[j].Slug]
			if li != lj {
				return li > lj
			}
			// Stable tiebreaker keeps the API deterministic between calls.
			return candidates[i].Slug < candidates[j].Slug
		})
		if req.Limit > 0 && len(candidates) > req.Limit {
			candidates = candidates[:req.Limit]
		}
		selected = make(map[string]struct{}, len(candidates))
		for _, p := range candidates {
			selected[p.Slug] = struct{}{}
		}
	}

	// Build nodes from the selected set.
	nodes := make([]types.WikiGraphNode, 0, len(selected))
	for slug := range selected {
		p := pageBySlug[slug]
		nodes = append(nodes, types.WikiGraphNode{
			Slug:      p.Slug,
			Title:     p.Title,
			PageType:  p.PageType,
			LinkCount: linkCount[slug],
			Familiar:  p.BuiltFrom(familiarSet),
		})
	}
	// Deterministic node ordering — the map iteration above is random.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].LinkCount != nodes[j].LinkCount {
			return nodes[i].LinkCount > nodes[j].LinkCount
		}
		return nodes[i].Slug < nodes[j].Slug
	})

	// Build edges, keeping only edges whose endpoints both survived selection.
	var edges []types.WikiGraphEdge
	for _, p := range pages {
		if _, ok := selected[p.Slug]; !ok {
			continue
		}
		for _, target := range p.OutLinks {
			if _, ok := selected[target]; !ok {
				continue
			}
			edges = append(edges, types.WikiGraphEdge{
				Source: p.Slug,
				Target: target,
			})
		}
	}

	// total is the count of candidate nodes before truncation — i.e. the
	// population the frontend would need to fetch if it asked for the
	// whole graph. For overview this respects the type filter; for ego
	// it is the total KB page count (the user still sees "X of Y" based
	// on the full wiki, not a filtered denominator).
	total := len(pages)
	if mode == types.WikiGraphModeOverview && hasTypeFilter {
		total = 0
		for _, p := range pages {
			if typeAllow[p.PageType] {
				total++
			}
		}
	}

	meta := types.WikiGraphMeta{
		Mode:      mode,
		Total:     total,
		Returned:  len(nodes),
		Truncated: len(nodes) < total,
	}
	for _, n := range nodes {
		if n.Familiar {
			meta.FamiliarCount++
		}
	}
	if mode == types.WikiGraphModeEgo {
		meta.Center = req.Center
		meta.Depth = req.Depth
		if meta.Depth < 1 {
			meta.Depth = 1
		}
	}

	return &types.WikiGraphData{
		Nodes: nodes,
		Edges: edges,
		Meta:  meta,
	}, nil
}

// bfsEgoSlugs computes the undirected BFS neighborhood of `center` up to
// `depth` hops using both inbound and outbound links. Type-filtered pages
// are excluded from the result but are also NOT traversed through — so a
// filter that hides "index" pages will not leak the whole wiki via the
// index. The caller guarantees center exists in pageBySlug.
func bfsEgoSlugs(
	pageBySlug map[string]*types.WikiPage,
	center string,
	depth int,
	typeAllow map[string]bool,
	limit int,
) map[string]struct{} {
	hasTypeFilter := len(typeAllow) > 0
	centerPage, ok := pageBySlug[center]
	if !ok {
		return map[string]struct{}{}
	}
	// If the center itself fails the type filter we honor the filter and
	// return an empty set — the handler will surface Returned=0.
	if hasTypeFilter && !typeAllow[centerPage.PageType] {
		return map[string]struct{}{}
	}

	visited := map[string]struct{}{center: {}}
	frontier := []string{center}

	for hop := 0; hop < depth; hop++ {
		if limit > 0 && len(visited) >= limit {
			break
		}
		next := make([]string, 0, len(frontier))
		for _, slug := range frontier {
			p, ok := pageBySlug[slug]
			if !ok {
				continue
			}
			neighbors := make([]string, 0, len(p.OutLinks)+len(p.InLinks))
			neighbors = append(neighbors, p.OutLinks...)
			neighbors = append(neighbors, p.InLinks...)
			for _, nb := range neighbors {
				if _, seen := visited[nb]; seen {
					continue
				}
				np, exists := pageBySlug[nb]
				if !exists {
					continue
				}
				if hasTypeFilter && !typeAllow[np.PageType] {
					continue
				}
				visited[nb] = struct{}{}
				next = append(next, nb)
				if limit > 0 && len(visited) >= limit {
					break
				}
			}
			if limit > 0 && len(visited) >= limit {
				break
			}
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}

	return visited
}

// GetStats returns aggregate statistics about the wiki
func (s *wikiPageService) GetStats(ctx context.Context, kbID string) (*types.WikiStats, error) {
	counts, err := s.repo.CountByType(ctx, kbID)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, c := range counts {
		total += c
	}

	orphans, err := s.repo.CountOrphans(ctx, kbID)
	if err != nil {
		return nil, err
	}

	// Count total links
	pages, err := s.repo.ListAll(ctx, kbID)
	if err != nil {
		return nil, err
	}
	var totalLinks int64
	for _, p := range pages {
		totalLinks += int64(len(p.OutLinks))
	}

	// Get recent updates (last 10)
	listReq := &types.WikiPageListRequest{
		KnowledgeBaseID: kbID,
		Page:            1,
		PageSize:        10,
		SortBy:          "updated_at",
		SortOrder:       "desc",
	}
	recentPages, _, err := s.repo.List(ctx, listReq)
	if err != nil {
		return nil, err
	}

	var pendingTasks int64
	var pendingIssues int64
	var isActive bool
	if s.taskPendingRepo != nil {
		// Pending wiki ingest ops live in task_pending_ops keyed by
		// (task_type="wiki:ingest", scope="knowledge_base", scope_id=kbID).
		pendingTasks, _ = s.taskPendingRepo.PendingCount(ctx, wikiTaskType, wikiTaskScope, kbID)
	}
	if s.redisClient != nil {
		// The "active batch in progress" flag is still a Redis-only
		// short-lived signal (per-process lock with TTL renew); not
		// worth migrating since it carries no durable state.
		activeFlag, _ := s.redisClient.Exists(ctx, "wiki:active:"+kbID).Result()
		isActive = activeFlag > 0
	}

	issues, _ := s.ListIssues(ctx, kbID, "", "pending")
	pendingIssues = int64(len(issues))

	return &types.WikiStats{
		TotalPages:    total,
		PagesByType:   counts,
		TotalLinks:    totalLinks,
		OrphanCount:   orphans,
		RecentUpdates: recentPages,
		PendingTasks:  pendingTasks,
		PendingIssues: pendingIssues,
		IsActive:      isActive,
	}, nil
}

// RebuildLinks re-parses all pages and rebuilds bidirectional link references
func (s *wikiPageService) RebuildLinks(ctx context.Context, kbID string) error {
	pages, err := s.repo.ListAll(ctx, kbID)
	if err != nil {
		return err
	}

	// Build slug-to-page map
	pageMap := make(map[string]*types.WikiPage)
	for _, p := range pages {
		pageMap[p.Slug] = p
	}

	// Clear all inbound links first
	for _, p := range pages {
		p.InLinks = types.StringArray{}
	}

	// Re-parse outbound links and rebuild inbound links
	for _, p := range pages {
		p.OutLinks = s.parseOutLinks(p.Content)
		for _, target := range p.OutLinks {
			if tp, exists := pageMap[target]; exists {
				tp.InLinks = append(tp.InLinks, p.Slug)
			}
		}
	}

	// Save all pages (link rebuild is metadata-only, no version bump)
	for _, p := range pages {
		p.UpdatedAt = time.Now()
		if err := s.repo.UpdateMeta(ctx, p); err != nil {
			logger.Warnf(ctx, "wiki: failed to update links for page %s: %v", p.Slug, err)
		}
	}

	return nil
}

// ListAllPages retrieves all non-archived wiki pages without pagination.
func (s *wikiPageService) ListAllPages(ctx context.Context, kbID string) ([]*types.WikiPage, error) {
	return s.repo.ListAll(ctx, kbID)
}

// ListByType returns every wiki page of a given type for a KB. Exposed so
// callers like intro regeneration can load only the page type they need
// (summaries) instead of paying for the full ListAll scan.
func (s *wikiPageService) ListByType(ctx context.Context, kbID string, pageType string) ([]*types.WikiPage, error) {
	return s.repo.ListByType(ctx, kbID, pageType)
}

// ListPagesBySourceRef exposes the repository's source-ref lookup so higher
// layers (delete flow, retract reconciliation) can re-query the current wiki
// state without depending on a stale caller-captured slug list.
func (s *wikiPageService) ListPagesBySourceRef(ctx context.Context, kbID string, knowledgeID string) ([]*types.WikiPage, error) {
	return s.repo.ListBySourceRef(ctx, kbID, knowledgeID)
}

// ListSlugsBySourceRef returns just the slugs of pages that cite the given
// knowledge id. Backed by the source_refs GIN index added in migration
// 000041 — the wiki ingest pipeline uses it as a cheap "before" snapshot
// when reconciling old vs new extraction sets.
func (s *wikiPageService) ListSlugsBySourceRef(ctx context.Context, kbID string, knowledgeID string) ([]string, error) {
	return s.repo.ListSlugsBySourceRef(ctx, kbID, knowledgeID)
}

// ListBySlugs is the lazy fetcher used by wiki ingest's batch context.
// Returns lightweight projections (no content / source_refs / chunk_refs)
// for the requested slugs, in a single IN query. Used in place of the
// pre-batch ListAllPages dump that historically pulled hundreds of MB
// for KBs in the tens of thousands of pages.
func (s *wikiPageService) ListBySlugs(ctx context.Context, kbID string, slugs []string) (map[string]*types.WikiPageLite, error) {
	return s.repo.ListBySlugs(ctx, kbID, slugs)
}

// ListPageBacklinks returns the set of pages within `kbID` that link
// to `slug`, projected into the panel-friendly `WikiPageBacklink`
// shape (slug + title + page_type + status + updated_at). Ordering
// is `updated_at` desc with slug alphabetical as the tiebreaker so
// the UI gets a stable list even when two source pages share a
// timestamp.
//
// Build #11.
//
// Implementation notes:
//   - Uses the existing `ListBySlugs` repo method (single IN query),
//     no N+1. The repo's SELECT was extended with `updated_at` so the
//     timestamp is available without a second query.
//   - Orphans (slugs in `in_links` whose target page no longer
//     exists) are dropped at the `ListBySlugs` boundary because the
//     repo returns a map keyed by slug — slugs absent from the map
//     have no live row and never make it to the response.
//   - The target page's own slug (if it somehow appears in its own
//     `in_links` due to a historical write) is defensively excluded.
//   - Returns an empty slice (not nil) when no live links exist so
//     the HTTP response is `[]` rather than `null`.
func (s *wikiPageService) ListPageBacklinks(
	ctx context.Context, kbID string, slug string,
) ([]*types.WikiPageBacklink, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return []*types.WikiPageBacklink{}, nil
	}
	inLinks := make([]string, 0, len(page.InLinks))
	for _, s := range page.InLinks {
		if s == "" || s == slug {
			continue
		}
		inLinks = append(inLinks, s)
	}
	if len(inLinks) == 0 {
		return []*types.WikiPageBacklink{}, nil
	}
	lites, err := s.repo.ListBySlugs(ctx, kbID, inLinks)
	if err != nil {
		return nil, err
	}
	out := make([]*types.WikiPageBacklink, 0, len(lites))
	for _, srcSlug := range inLinks {
		lite, ok := lites[srcSlug]
		if !ok || lite == nil {
			continue
		}
		out = append(out, &types.WikiPageBacklink{
			Slug:      lite.Slug,
			Title:     lite.Title,
			PageType:  lite.PageType,
			Status:    lite.Status,
			UpdatedAt: lite.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Slug < out[j].Slug
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// -----------------------------------------------------------------------------
// Build #21 — backlinks graph cache payload helpers.
//
// WikiPageBacklink + WikiBacklinkIndirect + WikiPageBacklinkRelated are the
// ergonomic public types returned to the panel. Two of them embed
// *WikiPageBacklink (pointer embed), which encoding/json inlines at marshal
// time but cannot auto-allocate at unmarshal time — so a naive round-trip
// through json.Marshal/Unmarshal drops the embedded rows. We sidestep this
// by mirroring the structure with value-typed cachedBacklink / cachedIndirect
// / cachedRelated structs. The wire format is identical, so conversion is
// a pure structural remap and the public types stay unchanged.

// cachedBacklink is the cache-only mirror of WikiPageBacklink.
type cachedBacklink struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	PageType  string    `json:"page_type"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

// cachedIndirect mirrors WikiBacklinkIndirect with a value-typed embed.
type cachedIndirect struct {
	cachedBacklink
	Via string `json:"via"`
}

// cachedRelated mirrors WikiPageBacklinkRelated with a value-typed embed.
type cachedRelated struct {
	cachedBacklink
	Jaccard float64 `json:"jaccard"`
}

// cachedBroken mirrors WikiBacklinkBroken.
type cachedBroken struct {
	TargetSlug string `json:"target_slug"`
}

// graphToCachedGraph converts the public payload into a parallel cached-only
// struct. Drops any rows whose inner pointer is nil — encoding/json would
// silently omit those fields, leaving the row ambiguous on unmarshal.
func graphToCachedGraph(g *types.WikiBacklinkGraph) *cachedGraph {
	if g == nil {
		return &cachedGraph{}
	}
	out := &cachedGraph{Stats: g.Stats}
	out.Direct = make([]cachedBacklink, 0, len(g.Direct))
	for _, d := range g.Direct {
		if d == nil {
			continue
		}
		out.Direct = append(out.Direct, cachedBacklink{
			Slug:      d.Slug,
			Title:     d.Title,
			PageType:  d.PageType,
			Status:    d.Status,
			UpdatedAt: d.UpdatedAt,
		})
	}
	out.Indirect = make([]cachedIndirect, 0, len(g.Indirect))
	for _, ind := range g.Indirect {
		if ind == nil || ind.WikiPageBacklink == nil {
			continue
		}
		out.Indirect = append(out.Indirect, cachedIndirect{
			cachedBacklink: cachedBacklink{
				Slug:      ind.Slug,
				Title:     ind.Title,
				PageType:  ind.PageType,
				Status:    ind.Status,
				UpdatedAt: ind.UpdatedAt,
			},
			Via: ind.Via,
		})
	}
	out.Related = make([]cachedRelated, 0, len(g.Related))
	for _, rel := range g.Related {
		if rel == nil || rel.WikiPageBacklink == nil {
			continue
		}
		out.Related = append(out.Related, cachedRelated{
			cachedBacklink: cachedBacklink{
				Slug:      rel.Slug,
				Title:     rel.Title,
				PageType:  rel.PageType,
				Status:    rel.Status,
				UpdatedAt: rel.UpdatedAt,
			},
			Jaccard: rel.Jaccard,
		})
	}
	out.Broken = make([]cachedBroken, 0, len(g.Broken))
	for _, brk := range g.Broken {
		if brk == nil {
			continue
		}
		out.Broken = append(out.Broken, cachedBroken{TargetSlug: brk.TargetSlug})
	}
	return out
}

// cachedGraph is the JSON-friendly wrapper that binds the four section arrays.
type cachedGraph struct {
	Direct   []cachedBacklink             `json:"direct"`
	Indirect []cachedIndirect             `json:"indirect"`
	Related  []cachedRelated              `json:"related"`
	Broken   []cachedBroken               `json:"broken"`
	Stats    types.WikiBacklinkGraphStats `json:"stats"`
}

// cachedGraphToGraph inverts graphToCachedGraph.
func cachedGraphToGraph(c *cachedGraph) *types.WikiBacklinkGraph {
	if c == nil {
		return &types.WikiBacklinkGraph{
			Direct:   []*types.WikiPageBacklink{},
			Indirect: []*types.WikiBacklinkIndirect{},
			Related:  []*types.WikiPageBacklinkRelated{},
			Broken:   []*types.WikiBacklinkBroken{},
		}
	}
	direct := make([]*types.WikiPageBacklink, 0, len(c.Direct))
	for i := range c.Direct {
		d := c.Direct[i]
		direct = append(direct, &types.WikiPageBacklink{
			Slug:      d.Slug,
			Title:     d.Title,
			PageType:  d.PageType,
			Status:    d.Status,
			UpdatedAt: d.UpdatedAt,
		})
	}
	indirect := make([]*types.WikiBacklinkIndirect, 0, len(c.Indirect))
	for i := range c.Indirect {
		ind := c.Indirect[i]
		indirect = append(indirect, &types.WikiBacklinkIndirect{
			WikiPageBacklink: &types.WikiPageBacklink{
				Slug:      ind.Slug,
				Title:     ind.Title,
				PageType:  ind.PageType,
				Status:    ind.Status,
				UpdatedAt: ind.UpdatedAt,
			},
			Via: ind.Via,
		})
	}
	related := make([]*types.WikiPageBacklinkRelated, 0, len(c.Related))
	for i := range c.Related {
		rel := c.Related[i]
		related = append(related, &types.WikiPageBacklinkRelated{
			WikiPageBacklink: &types.WikiPageBacklink{
				Slug:      rel.Slug,
				Title:     rel.Title,
				PageType:  rel.PageType,
				Status:    rel.Status,
				UpdatedAt: rel.UpdatedAt,
			},
			Jaccard: rel.Jaccard,
		})
	}
	broken := make([]*types.WikiBacklinkBroken, 0, len(c.Broken))
	for i := range c.Broken {
		brk := c.Broken[i]
		broken = append(broken, &types.WikiBacklinkBroken{TargetSlug: brk.TargetSlug})
	}
	return &types.WikiBacklinkGraph{
		Direct:   direct,
		Indirect: indirect,
		Related:  related,
		Broken:   broken,
		Stats:    c.Stats,
	}
}

// encodeCacheRow serialises the four sections + stats into the cache-row
// columns. Returns (nil, false) on the first marshal failure — the caller
// treats that as a no-op so a malformed payload never crashes the read path.
func encodeCacheRow(kbID, slug string, g *types.WikiBacklinkGraph) (*types.WikiBacklinksCacheRow, bool) {
	cg := graphToCachedGraph(g)
	directJSON, err := json.Marshal(cg.Direct)
	if err != nil {
		return nil, false
	}
	indirectJSON, err := json.Marshal(cg.Indirect)
	if err != nil {
		return nil, false
	}
	relatedJSON, err := json.Marshal(cg.Related)
	if err != nil {
		return nil, false
	}
	brokenJSON, err := json.Marshal(cg.Broken)
	if err != nil {
		return nil, false
	}
	statsJSON, err := json.Marshal(cg.Stats)
	if err != nil {
		return nil, false
	}
	return &types.WikiBacklinksCacheRow{
		KbID:         kbID,
		Slug:         slug,
		DirectJSON:   string(directJSON),
		IndirectJSON: string(indirectJSON),
		RelatedJSON:  string(relatedJSON),
		BrokenJSON:   string(brokenJSON),
		StatsJSON:    string(statsJSON),
		// SourceEventID stays empty for cold-read writebacks — only the
		// write-time hooks stamp it from the wiki_event id.
	}, true
}

// decodeCacheRow reverses encodeCacheRow. Returns (nil, false) if any of
// the five JSON columns fails to unmarshal — caller falls back to a
// recompute, never serves a partial graph.
func decodeCacheRow(row *types.WikiBacklinksCacheRow) (*types.WikiBacklinkGraph, bool) {
	if row == nil {
		return nil, false
	}
	var direct []cachedBacklink
	if err := json.Unmarshal([]byte(row.DirectJSON), &direct); err != nil {
		return nil, false
	}
	var indirect []cachedIndirect
	if err := json.Unmarshal([]byte(row.IndirectJSON), &indirect); err != nil {
		return nil, false
	}
	var related []cachedRelated
	if err := json.Unmarshal([]byte(row.RelatedJSON), &related); err != nil {
		return nil, false
	}
	var broken []cachedBroken
	if err := json.Unmarshal([]byte(row.BrokenJSON), &broken); err != nil {
		return nil, false
	}
	var stats types.WikiBacklinkGraphStats
	if err := json.Unmarshal([]byte(row.StatsJSON), &stats); err != nil {
		return nil, false
	}
	return cachedGraphToGraph(&cachedGraph{
		Direct:   direct,
		Indirect: indirect,
		Related:  related,
		Broken:   broken,
		Stats:    stats,
	}), true
}

// InvalidateBacklinksCache wipes the cache rows whose slugs are no longer
// safe to serve — typically called after a write path mutates wiki_pages
// in a way that invalidates a derived graph. Best-effort: a failed wipe
// just means the next read recomputes on miss, so the system self-heals.
//
// Build #21.
// Build #23 — stamps a metric + audit row on every call.
// Build #28 — resolves the SlugSetStrategy from the slugSetStrategies
// registry (panic on unknown op, D1) and delegates the actual wipe +
// audit-log write to the invalidator. The audit row's Details JSON now
// carries a `strategy` field so operators can read "what rule picked
// this slug set" off the log alone.
func (s *wikiPageService) InvalidateBacklinksCache(
	ctx context.Context,
	req types.BacklinkCacheInvalidateRequest,
) {
	if s.cacheRepo == nil || req.KbID == "" || len(req.AffectedSlugs) == 0 {
		return
	}
	// Build #28: look up the strategy from the registry. Unknown ops
	// panic — same D1 contract as Resolve; missing a registration
	// surfaces in dev, not as a silent partial wipe.
	strategy, ok := slugSetStrategies[req.Op]
	if !ok {
		panic(fmt.Sprintf(
			"wikiPageService.InvalidateBacklinksCache: op %q not registered in slugSetStrategies; "+
				"add it to the table in wiki_backlinks_cache.go before using it (Build #28 D1)",
			req.Op,
		))
	}
	// Build #23 D3: increment invalidations counter with op label so
	// observability dashboards see every public API call. The actual
	// wipe + audit row are delegated to the invalidator.
	metricCacheInvalidationsTotal.WithLabelValues(string(req.Op)).Inc()
	if s.cacheInvalidator == nil {
		return
	}
	_, _ = s.cacheInvalidator.Invalidate(ctx, req, strategy)
}

// GetPageBacklinksCacheStatus returns the slim metadata for one slug's
// cached graph (computed_at + updated_at + source_event_id), or
// (nil, nil) if no cache row exists. The panel footer uses this to
// render a "last computed at" line without paying the full graph cost.
//
// Build #21.
// Build #23 — populates KbID (always equal to the input kbID) and
// HitRatio (process-local atomic snapshot). RowCount and
// PayloadSizeBytes stay zero on this path — they're KB-wide
// aggregates and the per-page footer call shouldn't pay that cost.
// The admin list endpoint returns those for a whole KB.
func (s *wikiPageService) GetPageBacklinksCacheStatus(
	ctx context.Context, kbID string, slug string,
) (*types.WikiBacklinksCacheStatus, error) {
	if s.cacheRepo == nil || kbID == "" || slug == "" {
		return nil, nil
	}
	row, err := s.cacheRepo.Get(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	snap := wikiCacheObsRead()
	hitRatio := 0.0
	if snap.Hits+snap.Misses > 0 {
		hitRatio = float64(snap.Hits) / float64(snap.Hits+snap.Misses)
	}
	return &types.WikiBacklinksCacheStatus{
		Slug:          row.Slug,
		KbID:          kbID,
		ComputedAt:    row.ComputedAt,
		UpdatedAt:     row.UpdatedAt,
		SourceEventID: row.SourceEventID,
		HitRatio:      hitRatio,
	}, nil
}

// ListBacklinksCacheStatuses returns the admin / debug KB-wide rollup:
// per-row statuses (paginated) plus row_count, payload_size_bytes,
// and a process-local hit_ratio. Used by the new
// GET /backlinks/cache-statuses admin endpoint.
//
// Build #23. Count + payload queries are best-effort — a failure
// logs a warning but does not fail the request, so an operator can
// still see the row list when the aggregate queries time out on a
// large KB.
func (s *wikiPageService) ListBacklinksCacheStatuses(
	ctx context.Context, kbID string, limit int, offset int,
) (*types.WikiBacklinksCacheStatusListResponse, error) {
	resp := &types.WikiBacklinksCacheStatusListResponse{
		KbID:  kbID,
		Items: []*types.WikiBacklinksCacheStatus{},
	}
	if s.cacheRepo == nil || kbID == "" {
		return resp, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := s.cacheRepo.ListByKB(ctx, kbID, limit, offset)
	if err != nil {
		return nil, err
	}
	// Populate KbID on each row (the repo doesn't know which kb_id
	// to stamp without a code review pass; we own that here).
	for _, it := range items {
		if it != nil {
			it.KbID = kbID
		}
	}
	resp.Items = items
	resp.Total = total
	resp.RowCount = total

	// Best-effort aggregates.
	if count, cerr := s.cacheRepo.CountByKB(ctx, kbID); cerr != nil {
		logger.Warnf(ctx, "wiki backlinks cache CountByKB failed (kb=%s): %v", kbID, cerr)
	} else {
		// Override RowCount with the authoritative COUNT(*) so
		// ListByKB's bounded total matches the unbounded count.
		// For now they're the same query, but future pagination
		// tweaks could diverge — keep both for back-compat.
		resp.RowCount = count
	}
	if bytes, perr := s.cacheRepo.SumPayloadSizeByKB(ctx, kbID); perr != nil {
		logger.Warnf(ctx, "wiki backlinks cache SumPayloadSizeByKB failed (kb=%s): %v", kbID, perr)
	} else {
		resp.PayloadSizeBytes = bytes
	}
	snap := wikiCacheObsRead()
	if snap.Hits+snap.Misses > 0 {
		resp.HitRatio = float64(snap.Hits) / float64(snap.Hits+snap.Misses)
	}
	return resp, nil
}

// ListBacklinkGraph wraps computeBacklinkGraph with a cache-first read and
// writeback. Cache lookups are best-effort: a missing row, a malformed
// payload, or a transient repo error all fall through to a fresh compute.
// Compute failures still bubble up unchanged — the cache never shadows
// an honest error.
//
// Build #21.
// Build #23 — wraps the cache Get with hit/miss/error counters and the
// writeback Upsert with a cache_writes counter. The counters live in
// wiki_backlinks_cache_observability.go so this body stays focused on
// the read/write logic; the three counter incs are deliberately cheap
// (atomic.Add + a Prom Inc) and run on every cache path.
func (s *wikiPageService) ListBacklinkGraph(
	ctx context.Context, req types.WikiBacklinkGraphRequest,
) (*types.WikiBacklinkGraph, error) {
	if s.cacheRepo != nil && req.KbID != "" && req.Slug != "" {
		row, err := s.cacheRepo.Get(ctx, req.KbID, req.Slug)
		switch {
		case err != nil:
			// Get failed — repo error, not a miss. Bump the error
			// counter and fall through to recompute. We do NOT log
			// here because the failure may be transient; Prom will
			// surface the rate and operators decide.
			wikiCacheObsIncError(req.KbID)
		case row == nil:
			// Honest cache miss — no row for (kb_id, slug). Bump the
			// miss counter and fall through to recompute.
			wikiCacheObsIncMiss(req.KbID)
		default:
			if graph, ok := decodeCacheRow(row); ok {
				// Cache hit — bump the hit counter.
				wikiCacheObsIncHit(req.KbID)
				// Build #24 D3: post-filter each section through
				// aclService.ResolveBulk so a row that references a
				// now-private / restricted slug is dropped before the
				// response leaves the service. Nil-safe: when the
				// service was constructed without an aclService the
				// filter is skipped (legacy Build #21/22/23 callers
				// keep their original contract).
				if s.aclService != nil {
					filtered, filterErr := s.filterBacklinkGraphByAcl(ctx, req, graph)
					if filterErr != nil {
						// Fail-closed: drop the cached result rather
						// than return a graph whose ACL decisions we
						// could not verify. This avoids leaking
						// restricted slugs if the ACL service is
						// temporarily down. Fall through to recompute
						// so the next request still has a chance to
						// hit a cache row that survived filtering.
						logger.Warnf(ctx,
							"wiki backlinks D3 filter failed (kb=%s slug=%s): %v — falling back to recompute",
							req.KbID, req.Slug, filterErr)
						wikiCacheObsIncError(req.KbID)
						// continue out of the switch's default arm
						break
					}
					return filtered, nil
				}
				return graph, nil
			}
			// Row exists but payload decode failed — treat as a
			// cache error: bump the error counter (not miss —
			// misses are "no row", this is "row but unusable") and
			// fall through to recompute. The bad row stays in the
			// table until an invalidation replaces it; we don't
			// delete it inline because that would race with the
			// writeback below on a slow recompute.
			wikiCacheObsIncError(req.KbID)
		}
	}
	graph, err := s.computeBacklinkGraph(ctx, req)
	if err != nil {
		return nil, err
	}
	if s.cacheRepo != nil && req.KbID != "" && req.Slug != "" {
		if row, ok := encodeCacheRow(req.KbID, req.Slug, graph); ok {
			// Best-effort writeback — the read path never fails on a
			// failed write because the next miss will simply recompute.
			// Build #23: bump cache_writes on every successful encode,
			// regardless of Upsert success — encode succeeded so the
			// would-be-write happened, even if the write itself later
			// failed (which the existing warn log already covers).
			wikiCacheObsIncWrite()
			if werr := s.cacheRepo.Upsert(ctx, row); werr != nil {
				logger.Warnf(ctx,
					"wiki backlinks cache writeback failed (kb=%s slug=%s): %v",
					req.KbID, req.Slug, werr)
			}
		}
	}
	return graph, nil
}

// filterBacklinkGraphByAcl post-filters the four sections of a cached
// backlink graph through aclService.ResolveBulk. Returns a new graph
// with restricted slugs dropped; the Stats.Counts field is left
// untouched (it now reflects "cached row counts, before per-user
// filtering") and the UI renders it as best-effort.
//
// Build #24 D3 — defense layer complementing the ACL→cache wipe
// hook (D4). Even when the wipe path misses a row (cache TTL race,
// large-KB reverse-lookup dropped slugs not present in the index),
// the read-time filter stops restricted slugs from leaking to
// callers without read permission.
func (s *wikiPageService) filterBacklinkGraphByAcl(
	ctx context.Context, req types.WikiBacklinkGraphRequest, graph *types.WikiBacklinkGraph,
) (*types.WikiBacklinkGraph, error) {
	caller := req.UserID
	if caller == "" {
		// No caller context — fall back to KB-level public read;
		// only inherit-mode pages are visible. We still call
		// ResolveBulk so the per-slug decision matches what the
		// page-level read path would have produced.
		caller = ""
	}

	items := make([]AclResolveItem, 0, len(graph.Direct)+len(graph.Indirect)+len(graph.Related)+len(graph.Broken))
	for _, row := range graph.Direct {
		items = append(items, AclResolveItem{KBID: req.KbID, Slug: row.Slug})
	}
	for _, row := range graph.Indirect {
		items = append(items, AclResolveItem{KBID: req.KbID, Slug: row.Slug})
	}
	for _, row := range graph.Related {
		items = append(items, AclResolveItem{KBID: req.KbID, Slug: row.Slug})
	}
	// Broken slugs do NOT exist as live pages — there is nothing to
	// ACL-check. They survive filtering unchanged.
	if len(items) == 0 {
		return graph, nil
	}

	decisions, err := s.aclService.ResolveBulk(ctx, items, caller)
	if err != nil {
		return nil, err
	}

	allowed := func(slug string) bool {
		d, ok := decisions[req.KbID+":"+slug]
		if !ok {
			// Missing decision — be conservative and drop. The ACL
			// service guarantees a map entry for every input item
			// (it maps errors to deny_allow_list internally); a
			// missing key signals a contract drift and we should
			// fail closed.
			return false
		}
		return d == types.WikiPageAclAllow
	}

	filtered := &types.WikiBacklinkGraph{Stats: graph.Stats, Broken: graph.Broken}
	if filtered.Direct == nil {
		filtered.Direct = []*types.WikiPageBacklink{}
	}
	if filtered.Indirect == nil {
		filtered.Indirect = []*types.WikiBacklinkIndirect{}
	}
	if filtered.Related == nil {
		filtered.Related = []*types.WikiPageBacklinkRelated{}
	}
	for _, row := range graph.Direct {
		if allowed(row.Slug) {
			filtered.Direct = append(filtered.Direct, row)
		}
	}
	for _, row := range graph.Indirect {
		if allowed(row.Slug) {
			filtered.Indirect = append(filtered.Indirect, row)
		}
	}
	for _, row := range graph.Related {
		if allowed(row.Slug) {
			filtered.Related = append(filtered.Related, row)
		}
	}
	return filtered, nil
}

// computeBacklinkGraph bundles four views of the backlink picture around
// a single page into one payload so the panel renders the full graph
// in a single round-trip. The four sections are:
//
//   - Direct   (1-hop):  pages whose `in_links` contains `slug`
//   - Indirect (2-hop):  pages that link to one of the direct set,
//     minus self and the direct set itself; each
//     row carries `via` = the direct slug it came from
//   - Related  (Jaccard): pages whose `out_links` set overlaps the
//     current page's `out_links` above the configured
//     threshold; rows carry `jaccard` ∈ [0, 1]
//   - Broken:           slugs in the current page's `out_links` that
//     do not resolve to any live page in the KB
//
// Parameters (clamp semantics; see handler):
//   - MaxIndirect       default 50, clamp [0, 200]
//   - MaxRelated        default 10, clamp [0, 50]
//   - JaccardThreshold  default 0.3, clamp [0, 1]
//
// All four sections are computed from already-loaded `*WikiPageLite`
// rows fetched through `ListBySlugs` — at most 3 IN queries (direct
// sources, indirect sources, broken-target candidates). No schema
// change, no new index, no full-KB lint traversal.
//
// Build #20.
func (s *wikiPageService) computeBacklinkGraph(
	ctx context.Context, req types.WikiBacklinkGraphRequest,
) (*types.WikiBacklinkGraph, error) {
	kbID := req.KbID
	slug := req.Slug

	// Normalise request defaults / clamps (defensive — the handler
	// already clamps, but tests can call this directly).
	maxIndirect := req.MaxIndirect
	if maxIndirect <= 0 {
		maxIndirect = 50
	}
	if maxIndirect > 200 {
		maxIndirect = 200
	}
	maxRelated := req.MaxRelated
	if maxRelated <= 0 {
		maxRelated = 10
	}
	if maxRelated > 50 {
		maxRelated = 50
	}
	threshold := req.JaccardThreshold
	if threshold <= 0 {
		threshold = 0.3
	}
	if threshold > 1 {
		threshold = 1
	}

	// Resolve the target page itself — single source of truth for
	// its in_links / out_links.
	target, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return &types.WikiBacklinkGraph{
			Direct:   []*types.WikiPageBacklink{},
			Indirect: []*types.WikiBacklinkIndirect{},
			Related:  []*types.WikiPageBacklinkRelated{},
			Broken:   []*types.WikiBacklinkBroken{},
			Stats:    types.WikiBacklinkGraphStats{},
		}, nil
	}

	// --- Direct (1-hop) ---
	directSlugs := make([]string, 0, len(target.InLinks))
	for _, s := range target.InLinks {
		if s == "" || s == slug {
			continue
		}
		directSlugs = append(directSlugs, s)
	}
	var directLites map[string]*types.WikiPageLite
	if len(directSlugs) > 0 {
		directLites, err = s.repo.ListBySlugs(ctx, kbID, directSlugs)
		if err != nil {
			return nil, err
		}
	} else {
		directLites = map[string]*types.WikiPageLite{}
	}
	direct := make([]*types.WikiPageBacklink, 0, len(directLites))
	for _, srcSlug := range directSlugs {
		lite, ok := directLites[srcSlug]
		if !ok || lite == nil {
			continue
		}
		direct = append(direct, &types.WikiPageBacklink{
			Slug:      lite.Slug,
			Title:     lite.Title,
			PageType:  lite.PageType,
			Status:    lite.Status,
			UpdatedAt: lite.UpdatedAt,
		})
	}
	sort.SliceStable(direct, func(i, j int) bool {
		if direct[i].UpdatedAt.Equal(direct[j].UpdatedAt) {
			return direct[i].Slug < direct[j].Slug
		}
		return direct[i].UpdatedAt.After(direct[j].UpdatedAt)
	})

	// --- Indirect (2-hop) ---
	// Walk each direct page's in_links, dedupe against self + direct,
	// resolve via a single batched ListBySlugs call, sort by updated_at
	// desc and truncate.
	indirectBySlug := make(map[string]*types.WikiBacklinkIndirect)
	indirectCandidates := make([]string, 0)
	for _, srcSlug := range directSlugs {
		srcLite, ok := directLites[srcSlug]
		if !ok || srcLite == nil {
			continue
		}
		srcPage, err := s.repo.GetBySlug(ctx, kbID, srcSlug)
		if err != nil || srcPage == nil {
			continue
		}
		for _, in := range srcPage.InLinks {
			if in == "" || in == slug {
				continue
			}
			// Exclude slugs already in direct (1-hop should not
			// also appear as 2-hop).
			if _, isDirect := directLites[in]; isDirect {
				continue
			}
			// Dedup: keep first `via` (most recent direct).
			if _, seen := indirectBySlug[in]; seen {
				continue
			}
			indirectBySlug[in] = &types.WikiBacklinkIndirect{
				WikiPageBacklink: nil, // populated after ListBySlugs
				Via:              srcSlug,
			}
			indirectCandidates = append(indirectCandidates, in)
		}
	}
	var indirectLites map[string]*types.WikiPageLite
	if len(indirectCandidates) > 0 {
		indirectLites, err = s.repo.ListBySlugs(ctx, kbID, indirectCandidates)
		if err != nil {
			return nil, err
		}
	} else {
		indirectLites = map[string]*types.WikiPageLite{}
	}
	indirect := make([]*types.WikiBacklinkIndirect, 0, len(indirectCandidates))
	for _, cand := range indirectCandidates {
		lite, ok := indirectLites[cand]
		if !ok || lite == nil {
			// Orphan 2-hop — drop.
			continue
		}
		row, ok := indirectBySlug[cand]
		if !ok {
			continue
		}
		row.WikiPageBacklink = &types.WikiPageBacklink{
			Slug:      lite.Slug,
			Title:     lite.Title,
			PageType:  lite.PageType,
			Status:    lite.Status,
			UpdatedAt: lite.UpdatedAt,
		}
		indirect = append(indirect, row)
	}
	sort.SliceStable(indirect, func(i, j int) bool {
		li, lj := indirect[i].UpdatedAt, indirect[j].UpdatedAt
		if li.Equal(lj) {
			return indirect[i].Slug < indirect[j].Slug
		}
		return li.After(lj)
	})
	if len(indirect) > maxIndirect {
		indirect = indirect[:maxIndirect]
	}

	// --- Related (Jaccard on out_links) ---
	outLinks := make([]string, 0, len(target.OutLinks))
	for _, o := range target.OutLinks {
		if o == "" || o == slug {
			continue
		}
		outLinks = append(outLinks, o)
	}
	currentSet := make(map[string]struct{}, len(outLinks))
	for _, o := range outLinks {
		currentSet[o] = struct{}{}
	}
	var outLites map[string]*types.WikiPageLite
	if len(outLinks) > 0 {
		outLites, err = s.repo.ListBySlugs(ctx, kbID, outLinks)
		if err != nil {
			return nil, err
		}
	} else {
		outLites = map[string]*types.WikiPageLite{}
	}

	// Existing target slugs (for the broken set + for excluding
	// the candidate's own out_links that point back at the current
	// page — that's overlap, not separate relevance).
	existingSlugs := make(map[string]struct{}, len(outLites))
	for _, lite := range outLites {
		if lite == nil {
			continue
		}
		existingSlugs[lite.Slug] = struct{}{}
	}

	// For related, we need each candidate's out_links set. Fetch
	// the candidates' full pages via the lite projection — they
	// already carry out_links (WikiPageLite.OutLinks). For Jaccard
	// we only need the out_links slice; the lite projection
	// includes it (see internal/types/wiki_page.go:WikiPageLite).
	type scored struct {
		row *types.WikiPageBacklinkRelated
		ts  time.Time
	}
	scoredRows := make([]scored, 0, len(outLinks))
	for _, cand := range outLinks {
		lite, ok := outLites[cand]
		if !ok || lite == nil {
			continue
		}
		candSet := make(map[string]struct{}, len(lite.OutLinks))
		for _, o := range lite.OutLinks {
			if o == "" || o == slug {
				continue
			}
			candSet[o] = struct{}{}
		}
		// Jaccard = |A ∩ B| / |A ∪ B|. Skip if either side empty.
		if len(currentSet) == 0 || len(candSet) == 0 {
			continue
		}
		inter := 0
		for k := range currentSet {
			if _, ok := candSet[k]; ok {
				inter++
			}
		}
		union := len(currentSet) + len(candSet) - inter
		if union == 0 {
			continue
		}
		score := float64(inter) / float64(union)
		if score < threshold {
			continue
		}
		scoredRows = append(scoredRows, scored{
			row: &types.WikiPageBacklinkRelated{
				WikiPageBacklink: &types.WikiPageBacklink{
					Slug:      lite.Slug,
					Title:     lite.Title,
					PageType:  lite.PageType,
					Status:    lite.Status,
					UpdatedAt: lite.UpdatedAt,
				},
				Jaccard: score,
			},
			ts: lite.UpdatedAt,
		})
	}
	sort.SliceStable(scoredRows, func(i, j int) bool {
		if scoredRows[i].row.Jaccard != scoredRows[j].row.Jaccard {
			return scoredRows[i].row.Jaccard > scoredRows[j].row.Jaccard
		}
		if scoredRows[i].ts.Equal(scoredRows[j].ts) {
			return scoredRows[i].row.Slug < scoredRows[j].row.Slug
		}
		return scoredRows[i].ts.After(scoredRows[j].ts)
	})
	related := make([]*types.WikiPageBacklinkRelated, 0, len(scoredRows))
	for _, r := range scoredRows {
		related = append(related, r.row)
	}
	if len(related) > maxRelated {
		related = related[:maxRelated]
	}

	// --- Broken (orphan slugs in current page's out_links) ---
	broken := make([]*types.WikiBacklinkBroken, 0)
	for _, o := range outLinks {
		if _, ok := existingSlugs[o]; !ok {
			broken = append(broken, &types.WikiBacklinkBroken{TargetSlug: o})
		}
	}
	sort.SliceStable(broken, func(i, j int) bool {
		return broken[i].TargetSlug < broken[j].TargetSlug
	})

	return &types.WikiBacklinkGraph{
		Direct:   direct,
		Indirect: indirect,
		Related:  related,
		Broken:   broken,
		Stats: types.WikiBacklinkGraphStats{
			DirectCount:   len(direct),
			IndirectCount: len(indirect),
			RelatedCount:  len(related),
			BrokenCount:   len(broken),
			OutLinkCount:  len(outLinks),
		},
	}, nil
}

// ListSummariesByKnowledgeIDs is the lazy fetcher for the retract /
// reparse branches of reduceSlugUpdates. Returns the content of each
// surviving summary page keyed by its source knowledge id.
func (s *wikiPageService) ListSummariesByKnowledgeIDs(ctx context.Context, kbID string, kids []string) (map[string]string, error) {
	return s.repo.ListSummariesByKnowledgeIDs(ctx, kbID, kids)
}

// ExistsSlugs reports which of the given slugs are live (non-archived,
// non-deleted) in the KB. Used by cleanDeadLinks to validate out-link
// targets before stripping them.
func (s *wikiPageService) ExistsSlugs(ctx context.Context, kbID string, slugs []string) (map[string]bool, error) {
	return s.repo.ExistsSlugs(ctx, kbID, slugs)
}

// ListAllSlugs returns every non-archived slug in the KB. Used by lint
// to compute the live-slug set without paying for ListAll's full row
// materialization.
func (s *wikiPageService) ListAllSlugs(ctx context.Context, kbID string) ([]string, error) {
	return s.repo.ListAllSlugs(ctx, kbID)
}

// ListPagesCursor is the lint-side cursor pagination over wiki_pages.
func (s *wikiPageService) ListPagesCursor(ctx context.Context, kbID string, cursor string, limit int) ([]*types.WikiPage, string, error) {
	return s.repo.ListPagesCursor(ctx, kbID, cursor, limit)
}

// ListByTypeRecent caps the page count for first-time index intro
// generation so the LLM prompt stays bounded on large KBs.
func (s *wikiPageService) ListByTypeRecent(ctx context.Context, kbID string, pageType string, limit int) ([]types.WikiIndexEntry, error) {
	return s.repo.ListByTypeRecent(ctx, kbID, pageType, limit)
}

// FindSimilarPages performs a pg_trgm similarity search; used by the
// dedup pre-filter to surface candidate merge targets.
func (s *wikiPageService) FindSimilarPages(ctx context.Context, kbID string, query string, pageTypes []string, limit int) ([]*types.WikiPageLite, error) {
	return s.repo.FindSimilarPages(ctx, kbID, query, pageTypes, limit)
}

// FindPagesByNormalizedTitle looks up exact same-type title identities for
// wiki ingest, independent of the trigram top-K used for semantic dedup.
func (s *wikiPageService) FindPagesByNormalizedTitle(ctx context.Context, kbID, pageType, identity string) ([]*types.WikiPageLite, error) {
	return s.repo.FindPagesByNormalizedTitle(ctx, kbID, pageType, identity)
}

// FindPagesByNormalizedTitles looks up several normalized title identities
// in one query so wiki ingest does not seq-scan once per extracted item.
func (s *wikiPageService) FindPagesByNormalizedTitles(ctx context.Context, kbID, pageType string, identities []string) ([]*types.WikiPageLite, error) {
	return s.repo.FindPagesByNormalizedTitles(ctx, kbID, pageType, identities)
}

// ListDistinctCategoryPaths returns the existing wiki folder paths. Used by
// wiki ingest's taxonomy planner to ground folder reuse.
func (s *wikiPageService) ListDistinctCategoryPaths(ctx context.Context, kbID string, maxPaths int) ([][]string, error) {
	return s.repo.ListDistinctCategoryPaths(ctx, kbID, maxPaths)
}

// CountByType is a service-layer pass-through over the repo. Used by
// the index intro path to frame the LLM prompt's "showing N of M" hint.
func (s *wikiPageService) CountByType(ctx context.Context, kbID string) (map[string]int64, error) {
	return s.repo.CountByType(ctx, kbID)
}

// SearchPages performs full-text search over wiki pages
func (s *wikiPageService) SearchPages(ctx context.Context, kbID string, query string, limit int) ([]*types.WikiPage, error) {
	return s.repo.Search(ctx, kbID, query, limit)
}

// --- Internal helpers ---

// parseOutLinks extracts [[wiki-link]] slugs from markdown content
func (s *wikiPageService) parseOutLinks(content string) types.StringArray {
	matches := wikiLinkRegex.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var links types.StringArray

	for _, match := range matches {
		if len(match) > 1 {
			slug := strings.TrimSpace(match[1])
			// Handle [[slug|display name]] format — slug is the first part
			if parts := strings.SplitN(slug, "|", 2); len(parts) == 2 {
				slug = strings.TrimSpace(parts[0])
			}
			slug = normalizeSlug(slug)
			if slug != "" && !seen[slug] {
				seen[slug] = true
				links = append(links, slug)
			}
		}
	}
	return links
}

// normalizeSlug normalizes a wiki link slug
func normalizeSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

// slugNamespace returns the prefix of a slug up to (but excluding) the first
// '/', e.g. "summary/abc" -> "summary". Slugs without a '/' map to "".
func slugNamespace(slug string) string {
	if i := strings.IndexByte(slug, '/'); i >= 0 {
		return slug[:i]
	}
	return ""
}

// rewriteDeadWikiLinks walks every [[slug]] / [[slug|display]] occurrence in
// content and lets `resolve` decide, per link, whether to rewrite the slug.
// `resolve` receives the normalized slug and its display text and returns the
// replacement slug plus true to rewrite, or ("", false) to leave the link
// untouched. Display text is preserved verbatim. This is a pure helper — the
// resolution policy lives entirely in the callback.
func rewriteDeadWikiLinks(content string, resolve func(normSlug, display string) (string, bool)) (string, bool) {
	changed := false
	out := wikiLinkRegex.ReplaceAllStringFunc(content, func(match string) string {
		inner := match[2 : len(match)-2]
		rawSlug := inner
		display := ""
		if parts := strings.SplitN(inner, "|", 2); len(parts) == 2 {
			rawSlug = parts[0]
			display = strings.TrimSpace(parts[1])
		}
		norm := normalizeSlug(rawSlug)
		if norm == "" {
			return match
		}
		newSlug, ok := resolve(norm, display)
		if !ok || newSlug == "" || newSlug == norm {
			return match
		}
		changed = true
		if display != "" {
			return "[[" + newSlug + "|" + display + "]]"
		}
		return "[[" + newSlug + "]]"
	})
	return out, changed
}

// RepairContentLinks rewrites [[slug]] / [[slug|display]] references in
// `content` whose target does not exist in the KB but is almost certainly a
// mangled form of a real page. The canonical case: an LLM re-typed a summary
// page's UUID-based slug and inserted/dropped a hex digit
// (summary/…06fb5d5b5b5e → summary/…06fb14d5b14b14e), producing a link that
// 404s and can never be recovered by exact lookup.
//
// Unlike stripDeadWikiLinks (the ingest cleanup pass), this method is
// REWRITE-ONLY: a dead link is corrected only when a confident live candidate
// exists; otherwise it is left completely untouched. It NEVER strips a link to
// plain text, so it is safe on any write path — including writes whose targets
// legitimately do not exist yet (those simply stay as-is until they do).
//
// The candidate pool for each dead link is scoped to live slugs that share the
// same namespace prefix (a dead `summary/<uuid>` only resolves against live
// `summary/*` slugs). Scoping keeps the bigram-similarity lever safe: distinct
// high-entropy UUIDs in one namespace don't collide, while a one-character
// mangle stays comfortably above threshold against its true source.
//
// Returns the possibly-updated content and whether any rewrite happened.
// Errors are only returned for hard repo failures; callers may treat repair as
// best-effort and ignore them.
func (s *wikiPageService) RepairContentLinks(
	ctx context.Context, kbID, selfSlug, content string,
) (string, bool, error) {
	if strings.TrimSpace(content) == "" {
		return content, false, nil
	}
	outLinks := s.parseOutLinks(content)
	if len(outLinks) == 0 {
		return content, false, nil
	}

	existMap, err := s.repo.ExistsSlugs(ctx, kbID, outLinks)
	if err != nil {
		return content, false, err
	}
	deadPrefixes := make(map[string]struct{})
	for _, l := range outLinks {
		if l == selfSlug || existMap[l] {
			continue
		}
		deadPrefixes[slugNamespace(l)] = struct{}{}
	}
	if len(deadPrefixes) == 0 {
		return content, false, nil
	}

	allSlugs, err := s.repo.ListAllSlugs(ctx, kbID)
	if err != nil {
		return content, false, err
	}
	liveByPrefix := make(map[string]map[string]struct{})
	var candidateSlugs []string
	for _, sl := range allSlugs {
		ns := slugNamespace(sl)
		if _, want := deadPrefixes[ns]; !want {
			continue
		}
		set, ok := liveByPrefix[ns]
		if !ok {
			set = make(map[string]struct{})
			liveByPrefix[ns] = set
		}
		set[sl] = struct{}{}
		candidateSlugs = append(candidateSlugs, sl)
	}
	if len(candidateSlugs) == 0 {
		return content, false, nil
	}

	// Build a title -> slug reverse lookup for candidate pages so the
	// display-text lever (the safest, most precise one) can fire. Bounded to
	// the relevant namespaces so this stays cheap even on large KBs.
	titleToSlug := make(map[string]string)
	if lites, lerr := s.repo.ListBySlugs(ctx, kbID, candidateSlugs); lerr == nil {
		for _, lp := range lites {
			if lp != nil && lp.Title != "" {
				titleToSlug[lp.Title] = lp.Slug
			}
		}
	}

	resolveCache := make(map[string]string)
	newContent, changed := rewriteDeadWikiLinks(content, func(norm, display string) (string, bool) {
		if norm == selfSlug || existMap[norm] {
			return "", false
		}
		key := norm + "\x00" + display
		if cached, ok := resolveCache[key]; ok {
			return cached, cached != ""
		}
		resolved, ok := resolveDeadSlug(norm, display, liveByPrefix[slugNamespace(norm)], titleToSlug)
		if !ok || resolved == norm {
			resolveCache[key] = ""
			return "", false
		}
		resolveCache[key] = resolved
		return resolved, true
	})
	return newContent, changed, nil
}

// updateInLinks adds the source slug to the in_links of target pages
func (s *wikiPageService) updateInLinks(ctx context.Context, kbID string, sourceSlug string, targets types.StringArray) {
	for _, targetSlug := range targets {
		targetPage, err := s.repo.GetBySlug(ctx, kbID, targetSlug)
		if err != nil {
			continue // target page may not exist yet
		}
		if !containsString(targetPage.InLinks, sourceSlug) {
			targetPage.InLinks = append(targetPage.InLinks, sourceSlug)
			targetPage.UpdatedAt = time.Now()
			if err := s.repo.UpdateMeta(ctx, targetPage); err != nil {
				logger.Warnf(ctx, "wiki: failed to update in_links for %s: %v", targetSlug, err)
			}
		}
	}
}

// removeInLinks removes the source slug from the in_links of target pages
func (s *wikiPageService) removeInLinks(ctx context.Context, kbID string, sourceSlug string, targets types.StringArray) {
	for _, targetSlug := range targets {
		targetPage, err := s.repo.GetBySlug(ctx, kbID, targetSlug)
		if err != nil {
			continue
		}
		newInLinks := removeString(targetPage.InLinks, sourceSlug)
		if len(newInLinks) != len(targetPage.InLinks) {
			targetPage.InLinks = newInLinks
			targetPage.UpdatedAt = time.Now()
			if err := s.repo.UpdateMeta(ctx, targetPage); err != nil {
				logger.Warnf(ctx, "wiki: failed to update in_links for %s: %v", targetSlug, err)
			}
		}
	}
}

// deleteChunkForPage removes the synced chunk for a wiki page. Chunk sync is
// optional wiring, so a service built without a chunk repository just skips
// it rather than taking the delete down with it.
func (s *wikiPageService) deleteChunkForPage(ctx context.Context, page *types.WikiPage) {
	if s.chunkRepo == nil {
		return
	}
	chunkID := "wp-" + page.ID
	if err := s.chunkRepo.DeleteChunk(ctx, page.TenantID, chunkID); err != nil {
		logger.Warnf(ctx, "wiki: failed to delete chunk for page %s: %v", page.Slug, err)
	}
}

// createDefaultPage creates the default index page.
func (s *wikiPageService) createDefaultPage(ctx context.Context, kbID string, slug string, title string, pageType string, content string) (*types.WikiPage, error) {
	// Get KB to get tenant ID
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("get knowledge base: %w", err)
	}

	page := &types.WikiPage{
		ID:              uuid.New().String(),
		TenantID:        kb.TenantID,
		KnowledgeBaseID: kbID,
		Slug:            slug,
		Title:           title,
		PageType:        pageType,
		Status:          types.WikiPageStatusPublished,
		Content:         content,
		Summary:         title,
		Version:         1,
	}
	normalizeWikiHierarchy(page)

	if err := s.repo.Create(ctx, page); err != nil {
		return nil, fmt.Errorf("create default %s page: %w", slug, err)
	}
	return page, nil
}

func normalizeWikiHierarchy(page *types.WikiPage) {
	if page == nil {
		return
	}
	page.ParentSlug = strings.TrimSpace(page.ParentSlug)

	cleanPath := types.StringArray(types.CleanWikiCategoryPath(page.CategoryPath))
	page.CategoryPath = cleanPath
	page.Depth = len(cleanPath)

	display := strings.TrimSpace(page.Title)
	if display == "" {
		display = strings.TrimSpace(page.Slug)
	}
	page.WikiPath = buildWikiPath(page.PageType, cleanPath, display)
}

func normalizeWikiIndexEntryHierarchy(entry *types.WikiIndexEntry, pageType string) {
	if entry == nil {
		return
	}

	cleanPath := types.StringArray(types.CleanWikiCategoryPath(entry.CategoryPath))
	entry.CategoryPath = cleanPath
	entry.Depth = len(cleanPath)

	display := strings.TrimSpace(entry.Title)
	if display == "" {
		display = strings.TrimSpace(entry.Slug)
	}
	entry.WikiPath = buildWikiPath(pageType, cleanPath, display)
}

// buildWikiPath assembles the normalized, sortable "page_type/cat.../title"
// breadcrumb used for directory ordering. Empty segments are skipped.
func buildWikiPath(pageType string, categoryPath []string, display string) string {
	parts := make([]string, 0, len(categoryPath)+2)
	if pt := strings.TrimSpace(pageType); pt != "" {
		parts = append(parts, pt)
	}
	parts = append(parts, categoryPath...)
	if display != "" {
		parts = append(parts, display)
	}
	return strings.Join(parts, "/")
}

// removeString removes a string from a slice
func removeString(slice []string, s string) types.StringArray {
	result := make(types.StringArray, 0, len(slice))
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// CreateIssue logs a new issue for a wiki page
func (s *wikiPageService) CreateIssue(ctx context.Context, issue *types.WikiPageIssue) (*types.WikiPageIssue, error) {
	if issue.ID == "" {
		issue.ID = uuid.New().String()
	}
	if err := s.repo.CreateIssue(ctx, issue); err != nil {
		return nil, fmt.Errorf("create wiki page issue: %w", err)
	}
	return issue, nil
}

// ListIssues retrieves issues for a knowledge base
func (s *wikiPageService) ListIssues(ctx context.Context, kbID string, slug string, status string) ([]*types.WikiPageIssue, error) {
	return s.repo.ListIssues(ctx, kbID, slug, status)
}

// UpdateIssueStatus updates an issue's status
func (s *wikiPageService) UpdateIssueStatus(ctx context.Context, issueID string, status string) error {
	return s.repo.UpdateIssueStatus(ctx, issueID, status)
}

// --- Folder tree (wiki_folders) ---

// wikiFolderSegments splits a materialized folder path ("AI/RAG") into cleaned
// segments. Empty/blank path yields nil (the wiki root).
func wikiFolderSegments(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return types.CleanWikiCategoryPath(strings.Split(path, "/"))
}

// applyFolderToPage refreshes a page's derived category_path cache from its
// authoritative FolderID. Root ("") clears the path. A folder id that does not
// resolve is treated as a hard error so we never silently misplace a page.
func (s *wikiPageService) applyFolderToPage(ctx context.Context, page *types.WikiPage) error {
	if page == nil {
		return nil
	}
	if strings.TrimSpace(page.FolderID) == "" {
		page.FolderID = ""
		page.CategoryPath = nil
		return nil
	}
	folder, err := s.repo.GetFolderByID(ctx, page.KnowledgeBaseID, page.FolderID)
	if err != nil {
		if errors.Is(err, repository.ErrWikiFolderNotFound) {
			return fmt.Errorf("wiki page references unknown folder %q", page.FolderID)
		}
		return fmt.Errorf("resolve page folder: %w", err)
	}
	page.CategoryPath = types.StringArray(wikiFolderSegments(folder.Path))
	return nil
}

// GetFolder retrieves a single folder by id.
func (s *wikiPageService) GetFolder(ctx context.Context, kbID string, id string) (*types.WikiFolder, error) {
	return s.repo.GetFolderByID(ctx, kbID, id)
}

// ListChildFolders returns the direct children of parentID for a tree view
// scoped to pageTypes. PageCount is recursive (the folder's whole subtree) so
// a parent reflects everything filed beneath it. A folder is shown when its
// subtree holds a page matching pageTypes. Wholly-empty folders (no pages of
// any type underneath) are only listed when multiple types are requested —
// the merged knowledge view — so single-type tabs like summary do not surface
// empty containers.
func (s *wikiPageService) ListChildFolders(
	ctx context.Context, kbID string, parentID string, pageTypes []string,
) ([]types.WikiFolderNode, error) {
	all, err := s.repo.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, err
	}
	scopedDirect, err := s.repo.CountPagesByFolder(ctx, kbID, pageTypes)
	if err != nil {
		return nil, err
	}
	allDirect := scopedDirect
	if len(pageTypes) > 0 {
		allDirect, err = s.repo.CountPagesByFolder(ctx, kbID, nil)
		if err != nil {
			return nil, err
		}
	}
	recScoped := recursiveFolderCounts(all, scopedDirect)
	recAll := recursiveFolderCounts(all, allDirect)
	showEmptyFolders := len(pageTypes) > 1
	// A folder belongs in this view if it (recursively) contains a page of the
	// requested types, or — only in the merged knowledge view — if it is a
	// completely empty container with no pages of any type underneath.
	relevant := func(id string) bool {
		if recScoped[id] > 0 {
			return true
		}
		if showEmptyFolders {
			return recAll[id] == 0
		}
		return false
	}

	out := make([]types.WikiFolderNode, 0)
	for _, f := range all {
		if f.ParentID != parentID || !relevant(f.ID) {
			continue
		}
		hasChildren := false
		for _, g := range all {
			if g.ParentID == f.ID && relevant(g.ID) {
				hasChildren = true
				break
			}
		}
		out = append(out, types.WikiFolderNode{
			WikiFolder:  *f,
			PageCount:   recScoped[f.ID],
			HasChildren: hasChildren,
		})
	}
	return out, nil
}

// recursiveFolderCounts maps each folder id to the sum of `direct` page counts
// over the folder and all of its descendants, using the materialized path so a
// single pass over the (navigation-sized) folder set suffices.
func recursiveFolderCounts(all []*types.WikiFolder, direct map[string]int64) map[string]int64 {
	res := make(map[string]int64, len(all))
	for _, f := range all {
		sum := direct[f.ID]
		prefix := f.Path + "/"
		for _, g := range all {
			if g.ID != f.ID && strings.HasPrefix(g.Path, prefix) {
				sum += direct[g.ID]
			}
		}
		res[f.ID] = sum
	}
	return res
}

// validateFolderName trims and rejects blank names or names carrying directory
// separators (a folder name is a single tree level).
func validateFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("folder name is required")
	}
	if strings.ContainsAny(name, "/｜|／") {
		return "", fmt.Errorf("folder name %q must not contain a path separator", name)
	}
	return name, nil
}

// CreateFolder creates a new empty folder under parentID.
func (s *wikiPageService) CreateFolder(
	ctx context.Context, kbID string, tenantID uint64, parentID string, name string,
) (*types.WikiFolder, error) {
	name, err := validateFolderName(name)
	if err != nil {
		return nil, err
	}

	parentPath := ""
	depth := 1
	if parentID != types.WikiFolderRootID {
		parent, err := s.repo.GetFolderByID(ctx, kbID, parentID)
		if err != nil {
			return nil, err
		}
		parentPath = parent.Path
		depth = parent.Depth + 1
	}

	if _, err := s.repo.GetChildFolderByName(ctx, kbID, parentID, name); err == nil {
		return nil, repository.ErrWikiFolderConflict
	} else if !errors.Is(err, repository.ErrWikiFolderNotFound) {
		return nil, err
	}

	path := name
	if parentPath != "" {
		path = parentPath + "/" + name
	}
	now := time.Now()
	folder := &types.WikiFolder{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		ParentID:        parentID,
		Name:            name,
		Path:            path,
		Depth:           depth,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("create wiki folder: %w", err)
	}
	return folder, nil
}

// FindOrCreateFolderPath resolves a category path to a leaf folder id, creating
// any missing intermediate folders along the way. Concurrency-safe against the
// unique (kb, parent, name) constraint via a re-fetch on create conflict.
func (s *wikiPageService) FindOrCreateFolderPath(
	ctx context.Context, kbID string, tenantID uint64, path []string,
) (string, []string, error) {
	clean := types.CleanWikiCategoryPath(path)
	if len(clean) == 0 {
		return types.WikiFolderRootID, nil, nil
	}
	parentID := types.WikiFolderRootID
	parentPath := ""
	for depth, name := range clean {
		child, err := s.repo.GetChildFolderByName(ctx, kbID, parentID, name)
		if err != nil {
			if !errors.Is(err, repository.ErrWikiFolderNotFound) {
				return "", nil, err
			}
			fp := name
			if parentPath != "" {
				fp = parentPath + "/" + name
			}
			now := time.Now()
			child = &types.WikiFolder{
				ID:              uuid.New().String(),
				TenantID:        tenantID,
				KnowledgeBaseID: kbID,
				ParentID:        parentID,
				Name:            name,
				Path:            fp,
				Depth:           depth + 1,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if cerr := s.repo.CreateFolder(ctx, child); cerr != nil {
				// Lost a create race (or unique violation): the sibling must
				// now exist — re-fetch it rather than failing the whole plan.
				child, err = s.repo.GetChildFolderByName(ctx, kbID, parentID, name)
				if err != nil {
					return "", nil, fmt.Errorf("create wiki folder %q: %w", fp, cerr)
				}
			}
		}
		parentID = child.ID
		parentPath = child.Path
	}
	return parentID, clean, nil
}

// MovePage relocates a page into folderID ("" = root) and refreshes its cached
// category path. Bookkeeping-only write (no version bump).
func (s *wikiPageService) MovePage(
	ctx context.Context, kbID string, slug string, folderID string,
) (*types.WikiPage, error) {
	page, err := s.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	page.FolderID = strings.TrimSpace(folderID)
	if err := s.applyFolderToPage(ctx, page); err != nil {
		return nil, err
	}
	page.UpdatedAt = time.Now()
	normalizeWikiHierarchy(page)
	if err := s.repo.UpdateMeta(ctx, page); err != nil {
		return nil, fmt.Errorf("move wiki page: %w", err)
	}

	// Build #21 — MovePage doesn't change slug or out_links (folder
	// move only), but the cached `folder_path` is derived from FolderID
	// and may show up in WikiPageLite titles / status filters that the
	// panel uses for display. Wipe [slug] ∪ out_links defensively; the
	// over-invalidate is harmless because the next read recomputes.
	if s.cacheRepo != nil && s.cacheInvalidator != nil {
		if slugs, _ := s.resolveBacklinkInvalidation(ctx,
			types.BacklinkCacheInvalidateMovePage,
			kbID, slug); err == nil {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          kbID,
				Op:            types.BacklinkCacheInvalidateMovePage,
				AffectedSlugs: slugs,
			})
		}
	}

	return page, nil
}

// normalizeBatchSlugs trims whitespace, drops empties, and dedupes slugs
// while preserving first-seen order so the returned slice's order matches
// what the caller submitted. Shared by all three batch endpoints (D6).
//
// Build #12.
func normalizeBatchSlugs(slugs []string) []string {
	if len(slugs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(slugs))
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// BatchMovePages relocates up to MaxWikiBatchSize pages into folderID in
// one bookkeeping-only pass. Per-row failures are recorded in the result
// instead of aborting the batch (D1 partial-success). Cross-KB slugs fail
// the whole request with `kb_mismatch` (D2 — request-level 400), not
// surfaced as `not_found`. Any other error surfaces the wrapped service
// error verbatim.
//
// Transactional scope note (D5): each per-row MovePage call is already
// atomic on its own (GORM wraps single-row Update in an implicit tx),
// so an individual row is never half-applied. Wrapping the whole loop
// in a single `s.db.WithContext(ctx).Transaction(...)` would convert
// the batch from partial-success into all-or-nothing, which contradicts
// D1 above. For very large batches that need cross-row atomicity (e.g.
// "move 1000 pages or none"), the right primitive is an async batch
// job with idempotent retries — Build #13+ territory.
//
// Build #12.
func (s *wikiPageService) BatchMovePages(
	ctx context.Context, kbID string, slugs []string, folderID string,
) (*types.WikiBatchResult, error) {
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	result := &types.WikiBatchResult{Succeeded: []string{}, Failed: []types.WikiPageBatchFailure{}}
	for _, slug := range clean {
		page, err := s.MovePage(ctx, kbID, slug, folderID)
		if err != nil {
			result.Failed = append(result.Failed, types.WikiPageBatchFailure{
				Slug: slug, Code: classifyBatchError(err), Error: err.Error(),
			})
			continue
		}
		_ = page
		result.Succeeded = append(result.Succeeded, slug)
	}

	// Build #21 — each row's MovePage already invalidated its own slug
	// + out_links, but a batch-level wipe covers any rows whose
	// out_links were rewritten by a concurrent in-flight UpdatePage
	// that we can't see from here. Best-effort.
	if s.cacheRepo != nil && len(clean) > 0 {
		slugs := make([]string, 0, len(clean))
		for _, slug := range clean {
			list, _ := s.resolveBacklinkInvalidation(ctx, types.BacklinkCacheInvalidateBatchMove, kbID, slug)
			slugs = append(slugs, list...)
		}
		if len(slugs) > 0 {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          kbID,
				Op:            types.BacklinkCacheInvalidateBatchMove,
				AffectedSlugs: slugs,
			})
		}
	}

	return result, nil
}

// BatchDeletePages soft-deletes up to MaxWikiBatchSize pages in one
// transactional pass. Each row reuses the same removeInLinks cascade and
// chunk deletion that DeletePage uses — a successful delete on row N does
// not depend on row N-1, so partial success is the natural shape (D1).
// Cross-KB slugs fail the whole request with `kb_mismatch` (D2).
//
// Build #12.
func (s *wikiPageService) BatchDeletePages(
	ctx context.Context, kbID string, slugs []string,
) (*types.WikiBatchResult, error) {
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	result := &types.WikiBatchResult{Succeeded: []string{}, Failed: []types.WikiPageBatchFailure{}}
	for _, slug := range clean {
		if err := s.DeletePage(ctx, kbID, slug); err != nil {
			result.Failed = append(result.Failed, types.WikiPageBatchFailure{
				Slug: slug, Code: classifyBatchError(err), Error: err.Error(),
			})
			continue
		}
		result.Succeeded = append(result.Succeeded, slug)
	}

	// Build #21 — defense-in-depth: each row's DeletePage already
	// invalidated [self] ∪ in_links, but the batch-level wipe handles
	// concurrent rewrites by UpdatePage that landed between row N's
	// refetch and row N+1's start.
	if s.cacheRepo != nil && len(clean) > 0 {
		all := make([]string, 0, len(clean))
		for _, slug := range clean {
			list, _ := s.resolveBacklinkInvalidation(ctx, types.BacklinkCacheInvalidateBatchDelete, kbID, slug)
			all = append(all, list...)
		}
		if len(all) > 0 {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          kbID,
				Op:            types.BacklinkCacheInvalidateBatchDelete,
				AffectedSlugs: all,
			})
		}
	}

	return result, nil
}

// BatchUpdatePageStatus rewrites `status` for up to MaxWikiBatchSize pages
// via the bookkeeping-only UpdateMeta path (D5 — no version bump). Status
// is validated once at the top of the call so a single typo fails the
// whole batch at the input boundary rather than per-row. Cross-KB slugs
// fail the whole request with `kb_mismatch` (D2).
//
// Build #12.
func (s *wikiPageService) BatchUpdatePageStatus(
	ctx context.Context, kbID string, slugs []string, status string,
) (*types.WikiBatchResult, error) {
	if !types.IsValidWikiPageStatus(status) {
		return nil, fmt.Errorf("invalid status %q: must be draft, published or archived", status)
	}
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	result := &types.WikiBatchResult{Succeeded: []string{}, Failed: []types.WikiPageBatchFailure{}}
	for _, slug := range clean {
		page, err := s.repo.GetBySlug(ctx, kbID, slug)
		if err != nil {
			result.Failed = append(result.Failed, types.WikiPageBatchFailure{
				Slug: slug, Code: classifyBatchError(err), Error: err.Error(),
			})
			continue
		}
		if page.Status == status {
			// No-op skip — the page is already at the target status.
			// Still reported as "succeeded" because the caller's
			// intent is satisfied without an error.
			result.Succeeded = append(result.Succeeded, slug)
			continue
		}
		page.Status = status
		page.UpdatedAt = time.Now()
		if err := s.repo.UpdateMeta(ctx, page); err != nil {
			result.Failed = append(result.Failed, types.WikiPageBatchFailure{
				Slug: slug, Code: classifyBatchError(err), Error: err.Error(),
			})
			continue
		}
		result.Succeeded = append(result.Succeeded, slug)

		// Build #21 — status is not a backlink signal, but the cached
		// payload might carry the page's old status into the panel
		// header chips. Per-row wipe keeps the cache coherent; we also
		// do a batch-level coalesced wipe below.
		if s.cacheRepo != nil && s.cacheInvalidator != nil {
			if list, _ := s.resolveBacklinkInvalidation(ctx,
				types.BacklinkCacheInvalidateBatchStatus, kbID, slug); err == nil && len(list) > 0 {
				s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
					KbID:          kbID,
					Op:            types.BacklinkCacheInvalidateBatchStatus,
					AffectedSlugs: list,
				})
			}
		}
	}

	// Build #21 — defense-in-depth: one coalesced wipe per KB so we
	// don't issue N round-trips for an N-row batch. Per-row wipes
	// above are the canonical source of truth; this is a belt-and-
	// suspenders against any row where the per-row wipe silently
	// failed.
	if s.cacheRepo != nil && len(result.Succeeded) > 0 {
		all := make([]string, 0, len(result.Succeeded))
		for _, slug := range result.Succeeded {
			list, _ := s.resolveBacklinkInvalidation(ctx, types.BacklinkCacheInvalidateBatchStatus, kbID, slug)
			all = append(all, list...)
		}
		if len(all) > 0 {
			s.InvalidateBacklinksCache(ctx, types.BacklinkCacheInvalidateRequest{
				KbID:          kbID,
				Op:            types.BacklinkCacheInvalidateBatchStatus,
				AffectedSlugs: all,
			})
		}
	}

	return result, nil
}

// BatchMovePagesRoute is the auto-routing entry point used by the
// Build #13 handler. It chooses between the synchronous BatchMovePages
// path and the WikiBatchJobService queue based on the slug count.
//
// Build #13.
func (s *wikiPageService) BatchMovePagesRoute(
	ctx context.Context, kbID string, slugs []string, folderID string, createdBy string,
) (*types.WikiBatchRouteResult, error) {
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	if s.batchSvc == nil || len(clean) < types.WikiBatchAsyncThreshold {
		result, err := s.BatchMovePages(ctx, kbID, clean, folderID)
		if err != nil {
			return nil, err
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	undo, err := CaptureUndoState(ctx, s, kbID, clean)
	if err != nil {
		return nil, err
	}
	params, err := json.Marshal(types.WikiBatchJobParams{Slugs: clean, FolderID: folderID})
	if err != nil {
		return nil, err
	}
	job := &types.WikiBatchJob{
		TenantID:        types.TenantIDFromContextOrZero(ctx),
		KnowledgeBaseID: kbID,
		Type:            types.WikiBatchJobTypeMove,
		Params:          params,
		UndoState:       undo,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
	}
	jobID, err := s.batchSvc.EnqueueJob(ctx, job)
	if err != nil {
		// Queue overflow — degrade to sync so the user request still
		// succeeds. Surface the original error in the log so the
		// operator sees the backpressure.
		logger.Warnf(ctx, "wiki batch move queue overflow, falling back to sync: %v", err)
		result, sErr := s.BatchMovePages(ctx, kbID, clean, folderID)
		if sErr != nil {
			return nil, sErr
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	job.ID = jobID
	return &types.WikiBatchRouteResult{Kind: "job", Job: job}, nil
}

// BatchDeletePagesRoute — auto-routing counterpart of BatchDeletePages.
//
// Build #13.
func (s *wikiPageService) BatchDeletePagesRoute(
	ctx context.Context, kbID string, slugs []string, createdBy string,
) (*types.WikiBatchRouteResult, error) {
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	if s.batchSvc == nil || len(clean) < types.WikiBatchAsyncThreshold {
		result, err := s.BatchDeletePages(ctx, kbID, clean)
		if err != nil {
			return nil, err
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	undo, err := CaptureUndoState(ctx, s, kbID, clean)
	if err != nil {
		return nil, err
	}
	params, err := json.Marshal(types.WikiBatchJobParams{Slugs: clean})
	if err != nil {
		return nil, err
	}
	job := &types.WikiBatchJob{
		TenantID:        types.TenantIDFromContextOrZero(ctx),
		KnowledgeBaseID: kbID,
		Type:            types.WikiBatchJobTypeDelete,
		Params:          params,
		UndoState:       undo,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
	}
	jobID, err := s.batchSvc.EnqueueJob(ctx, job)
	if err != nil {
		logger.Warnf(ctx, "wiki batch delete queue overflow, falling back to sync: %v", err)
		result, sErr := s.BatchDeletePages(ctx, kbID, clean)
		if sErr != nil {
			return nil, sErr
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	job.ID = jobID
	return &types.WikiBatchRouteResult{Kind: "job", Job: job}, nil
}

// BatchUpdatePageStatusRoute — auto-routing counterpart of
// BatchUpdatePageStatus. Status is not undoable so we don't capture
// undo_state (per spec A6 / D4).
//
// Build #13.
func (s *wikiPageService) BatchUpdatePageStatusRoute(
	ctx context.Context, kbID string, slugs []string, status string, createdBy string,
) (*types.WikiBatchRouteResult, error) {
	if !types.IsValidWikiPageStatus(status) {
		return nil, fmt.Errorf("invalid status %q: must be draft, published or archived", status)
	}
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	if s.batchSvc == nil || len(clean) < types.WikiBatchAsyncThreshold {
		result, err := s.BatchUpdatePageStatus(ctx, kbID, clean, status)
		if err != nil {
			return nil, err
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	params, err := json.Marshal(types.WikiBatchJobParams{Slugs: clean, Status: status})
	if err != nil {
		return nil, err
	}
	job := &types.WikiBatchJob{
		TenantID:        types.TenantIDFromContextOrZero(ctx),
		KnowledgeBaseID: kbID,
		Type:            types.WikiBatchJobTypeStatus,
		Params:          params,
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
	}
	jobID, err := s.batchSvc.EnqueueJob(ctx, job)
	if err != nil {
		logger.Warnf(ctx, "wiki batch status queue overflow, falling back to sync: %v", err)
		result, sErr := s.BatchUpdatePageStatus(ctx, kbID, clean, status)
		if sErr != nil {
			return nil, sErr
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	job.ID = jobID
	return &types.WikiBatchRouteResult{Kind: "job", Job: job}, nil
}

// assertBatchKBOwnership pre-validates that every slug in `slugs` either
// is missing entirely or lives inside `kbID`. The first slug found in
// any other KB aborts the whole batch with a WikiBatchKBMismatchError
// (D2 — request-level 400). Missing slugs are deliberately allowed
// through: those surface as per-row `not_found` in the partial-success
// result, not as a request-level rejection.
//
// Build #12.
func (s *wikiPageService) assertBatchKBOwnership(
	ctx context.Context, kbID string, slugs []string,
) error {
	for _, slug := range slugs {
		page, err := s.repo.GetBySlugAcrossKB(ctx, slug)
		if err != nil {
			if errors.Is(err, repository.ErrWikiPageNotFound) {
				continue
			}
			return err
		}
		if page.KnowledgeBaseID != kbID {
			return &types.WikiBatchKBMismatchError{
				Slug:     slug,
				ActualKB: page.KnowledgeBaseID,
			}
		}
	}
	return nil
}

// previewBatchResponse runs perSlug for each clean slug and assembles a
// WikiBatchPreviewResponse. Returns kb_mismatch if any slug is owned by
// a different KB (matching the real Batch* path's D2 contract). All
// per-slug work is read-only — by construction no DB writes happen.
//
// Build #16.
func (s *wikiPageService) previewBatchResponse(
	ctx context.Context, kbID string, slugs []string,
	perSlug func(ctx context.Context, slug string) error,
) (*types.WikiBatchPreviewResponse, error) {
	clean := normalizeBatchSlugs(slugs)
	if err := s.assertBatchKBOwnership(ctx, kbID, clean); err != nil {
		return nil, err
	}
	resp := &types.WikiBatchPreviewResponse{
		Success: []string{},
		Failed:  []types.WikiPageBatchFailure{},
		Summary: types.WikiBatchPreviewSummary{Total: len(clean)},
	}
	for _, slug := range clean {
		if err := perSlug(ctx, slug); err != nil {
			resp.Failed = append(resp.Failed, types.WikiPageBatchFailure{
				Slug:  slug,
				Code:  classifyBatchError(err),
				Error: err.Error(),
			})
			resp.Summary.WillFail++
			continue
		}
		resp.Success = append(resp.Success, slug)
		resp.Summary.WillSucceed++
	}
	return resp, nil
}

// PreviewBatchMove dry-runs BatchMovePages: for each slug it loads the
// page (returns not_found if missing) and resolves the target folder
// via applyFolderToPage (returns folder_not_found on a bad folder_id).
//
// We deliberately do NOT call MovePage here because:
//   - MovePage writes (UpdateMeta + category_path), and a Tx-rollback
//     wrapper would still leave the chunk-sync side effects outside
//     the Tx (the service has no GORM handle to thread one through);
//   - We want a strictly read-only preview, not "execute and undo".
//
// Build #16.
func (s *wikiPageService) PreviewBatchMove(
	ctx context.Context, kbID string, slugs []string, folderID string,
) (*types.WikiBatchPreviewResponse, error) {
	trimmedFolderID := strings.TrimSpace(folderID)
	return s.previewBatchResponse(ctx, kbID, slugs, func(ctx context.Context, slug string) error {
		page, err := s.repo.GetBySlug(ctx, kbID, slug)
		if err != nil {
			return err
		}
		// applyFolderToPage mutates the probe in-place (sets CategoryPath
		// or clears it for an empty folder id) but is otherwise a pure
		// read against s.repo.GetFolderByID. We pass a copy so the
		// original page state is never touched.
		probe := *page
		probe.FolderID = trimmedFolderID
		return s.applyFolderToPage(ctx, &probe)
	})
}

// PreviewBatchDelete dry-runs BatchDeletePages: each slug is checked for
// existence via GetBySlug. That's the only validation the real
// DeletePage performs before the soft-delete write — out-link cascades
// + revision cleanup run after the row vanishes and have no previewable
// analogue (they only matter once the row is gone).
//
// Build #16.
func (s *wikiPageService) PreviewBatchDelete(
	ctx context.Context, kbID string, slugs []string,
) (*types.WikiBatchPreviewResponse, error) {
	return s.previewBatchResponse(ctx, kbID, slugs, func(ctx context.Context, slug string) error {
		_, err := s.repo.GetBySlug(ctx, kbID, slug)
		return err
	})
}

// PreviewBatchStatus dry-runs BatchUpdatePageStatus: validates the
// status token (returns "invalid" for anything outside the closed set)
// and confirms each slug exists. Already-at-target pages count as
// success without an extra validation step — same as the real
// BatchUpdatePageStatus.
//
// Build #16.
func (s *wikiPageService) PreviewBatchStatus(
	ctx context.Context, kbID string, slugs []string, status string,
) (*types.WikiBatchPreviewResponse, error) {
	if !types.IsValidWikiPageStatus(status) {
		return nil, fmt.Errorf("invalid status %q: must be draft, published or archived", status)
	}
	return s.previewBatchResponse(ctx, kbID, slugs, func(ctx context.Context, slug string) error {
		_, err := s.repo.GetBySlug(ctx, kbID, slug)
		return err
	})
}

// classifyBatchError maps a per-row service error to a stable, machine-
// readable token so the frontend can render i18n strings per category
// without re-parsing the human-readable message. Anything not on the map
// becomes `internal`.
//
// Build #12.
func classifyBatchError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, repository.ErrWikiPageNotFound):
		return "not_found"
	case errors.Is(err, repository.ErrWikiFolderNotFound):
		return "folder_not_found"
	case errors.Is(err, repository.ErrWikiFolderConflict):
		return "folder_conflict"
	case errors.Is(err, repository.ErrWikiFolderNotEmpty):
		return "folder_not_empty"
	case errors.Is(err, types.ErrWikiBatchKBMismatch):
		return "kb_mismatch"
	}
	return "internal"
}

// RenameOrMoveFolder renames and/or reparents a folder, then recomputes the
// materialized path/depth of the entire subtree and the cached category path of
// every page underneath. Guards against cycles (moving a folder into itself or
// one of its descendants) and sibling name collisions.
func (s *wikiPageService) RenameOrMoveFolder(
	ctx context.Context, kbID string, id string, newName string, newParentID string, moveParent bool,
) (*types.WikiFolder, error) {
	folder, err := s.repo.GetFolderByID(ctx, kbID, id)
	if err != nil {
		return nil, err
	}

	name := folder.Name
	if strings.TrimSpace(newName) != "" {
		if name, err = validateFolderName(newName); err != nil {
			return nil, err
		}
	}

	targetParent := folder.ParentID
	if moveParent {
		targetParent = newParentID
	}

	parentPath := ""
	depthBase := 0
	if targetParent != types.WikiFolderRootID {
		if targetParent == folder.ID {
			return nil, errors.New("cannot move a folder into itself")
		}
		parent, err := s.repo.GetFolderByID(ctx, kbID, targetParent)
		if err != nil {
			return nil, err
		}
		if parent.Path == folder.Path || strings.HasPrefix(parent.Path, folder.Path+"/") {
			return nil, errors.New("cannot move a folder into its own descendant")
		}
		parentPath = parent.Path
		depthBase = parent.Depth
	}

	if existing, err := s.repo.GetChildFolderByName(ctx, kbID, targetParent, name); err == nil {
		if existing.ID != folder.ID {
			return nil, repository.ErrWikiFolderConflict
		}
	} else if !errors.Is(err, repository.ErrWikiFolderNotFound) {
		return nil, err
	}

	oldPath := folder.Path
	newPath := name
	if parentPath != "" {
		newPath = parentPath + "/" + name
	}
	if newPath == oldPath && targetParent == folder.ParentID {
		return folder, nil // no-op
	}

	all, err := s.repo.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	affected := make([]string, 0)
	var updated *types.WikiFolder
	for _, f := range all {
		switch {
		case f.ID == folder.ID:
			f.ParentID = targetParent
			f.Name = name
			f.Path = newPath
			f.Depth = depthBase + 1
		case strings.HasPrefix(f.Path, oldPath+"/"):
			f.Path = newPath + f.Path[len(oldPath):]
			f.Depth = len(wikiFolderSegments(f.Path))
		default:
			continue
		}
		f.UpdatedAt = now
		if err := s.repo.UpdateFolder(ctx, f); err != nil {
			return nil, err
		}
		affected = append(affected, f.ID)
		if f.ID == folder.ID {
			updated = f
		}
	}

	if err := s.recomputePagesForFolders(ctx, kbID, affected); err != nil {
		return nil, err
	}
	if updated == nil {
		updated = folder
	}
	return updated, nil
}

// recomputePagesForFolders refreshes the cached category_path/wiki_path/depth of
// every page filed under any of the given folder ids (used after a folder
// subtree is moved/renamed). Bookkeeping-only writes (no version bump).
func (s *wikiPageService) recomputePagesForFolders(ctx context.Context, kbID string, folderIDs []string) error {
	if len(folderIDs) == 0 {
		return nil
	}
	pages, err := s.repo.ListPagesByFolderIDs(ctx, kbID, folderIDs)
	if err != nil {
		return err
	}
	for _, page := range pages {
		if err := s.applyFolderToPage(ctx, page); err != nil {
			return err
		}
		page.UpdatedAt = time.Now()
		normalizeWikiHierarchy(page)
		if err := s.repo.UpdateMeta(ctx, page); err != nil {
			logger.Warnf(ctx, "wiki: recompute folder path for page %s failed: %v", page.Slug, err)
		}
	}
	return nil
}

// DeleteFolder removes a folder that has no pages and no child folders. The UI
// must relocate contents first; this keeps deletion non-destructive.
func (s *wikiPageService) DeleteFolder(ctx context.Context, kbID string, id string) error {
	if _, err := s.repo.GetFolderByID(ctx, kbID, id); err != nil {
		return err
	}
	children, err := s.repo.ListChildFolders(ctx, kbID, id)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return repository.ErrWikiFolderNotEmpty
	}
	pages, err := s.repo.ListPagesByFolderIDs(ctx, kbID, []string{id})
	if err != nil {
		return err
	}
	if len(pages) > 0 {
		return repository.ErrWikiFolderNotEmpty
	}
	return s.repo.DeleteFolder(ctx, kbID, id)
}

// PruneEmptyFolderChains removes folders that became empty after retracting a
// document, followed by any ancestors made empty by those removals. It only
// considers the supplied folder chains, so intentionally-empty folders
// elsewhere in the wiki are preserved. Callers must wait for the KB's ingest
// queue to drain before invoking this method: taxonomy planning creates a
// folder before reduce writes the page that will reference it.
func (s *wikiPageService) PruneEmptyFolderChains(
	ctx context.Context, kbID string, folderIDs []string,
) ([]string, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}
	all, err := s.repo.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*types.WikiFolder, len(all))
	for _, folder := range all {
		if folder != nil {
			byID[folder.ID] = folder
		}
	}

	candidates := make(map[string]*types.WikiFolder)
	for _, id := range folderIDs {
		seen := make(map[string]struct{})
		for id != types.WikiFolderRootID {
			if _, cycle := seen[id]; cycle {
				break
			}
			seen[id] = struct{}{}
			folder := byID[id]
			if folder == nil {
				break
			}
			candidates[id] = folder
			id = folder.ParentID
		}
	}

	ordered := make([]*types.WikiFolder, 0, len(candidates))
	for _, folder := range candidates {
		ordered = append(ordered, folder)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Depth == ordered[j].Depth {
			return ordered[i].Path > ordered[j].Path
		}
		return ordered[i].Depth > ordered[j].Depth
	})

	deleted := make([]string, 0, len(ordered))
	for _, folder := range ordered {
		children, err := s.repo.ListChildFolders(ctx, kbID, folder.ID)
		if err != nil {
			return deleted, err
		}
		if len(children) > 0 {
			continue
		}
		pages, err := s.repo.ListPagesByFolderIDs(ctx, kbID, []string{folder.ID})
		if err != nil {
			return deleted, err
		}
		if len(pages) > 0 {
			continue
		}
		if err := s.repo.DeleteFolder(ctx, kbID, folder.ID); err != nil {
			if errors.Is(err, repository.ErrWikiFolderNotFound) ||
				errors.Is(err, repository.ErrWikiFolderNotEmpty) {
				continue
			}
			return deleted, err
		}
		deleted = append(deleted, folder.ID)
	}
	return deleted, nil
}

// InjectCrossLinks scans affected pages and injects [[wiki-links]] for mentions
// of other wiki page titles in the content. Pure text replacement, no LLM call.
// Shares the linkifyContent helper with the ingest pipeline so both paths honor
// the same code-block / existing-link / word-boundary rules.
func (s *wikiPageService) InjectCrossLinks(ctx context.Context, kbID string, affectedSlugs []string) {
	allPages, err := s.ListAllPages(ctx, kbID)
	if err != nil || len(allPages) < 2 {
		return
	}

	refs := collectLinkRefs(allPages)
	if len(refs) == 0 {
		return
	}

	affectedSet := make(map[string]bool, len(affectedSlugs))
	for _, slug := range affectedSlugs {
		affectedSet[slug] = true
	}

	var updated int
	for _, p := range allPages {
		if !affectedSet[p.Slug] {
			continue
		}
		if p.PageType == types.WikiPageTypeIndex {
			continue
		}

		newContent, changed := linkifyContent(p.Content, refs, p.Slug)
		if !changed {
			continue
		}
		p.Content = newContent
		if err := s.UpdateAutoLinkedContent(ctx, p); err != nil {
			logger.Warnf(ctx, "wiki: cross-link injection failed for %s: %v", p.Slug, err)
			continue
		}
		updated++
	}

	if updated > 0 {
		logger.Infof(ctx, "wiki: injected cross-links in %d pages", updated)
	}
}

// RebuildIndexPage was historically called by agent write/rename tools to
// refresh the index page's directory listing after a page mutation.
//
// The directory is no longer persisted in wiki_pages.content — it is
// assembled on demand by GetIndexView from the lightweight ListByTypeLight
// projection, so individual page writes don't need to redo O(N) string
// concatenation and rewrite a multi-MB TEXT column anymore. Keeping the
// method name lets existing agent tool call sites (wiki_write_page,
// wiki_rename_page) compile unchanged; the body is now intentionally a
// no-op.
//
// The intro that still lives on the index row is managed separately by
// the ingest pipeline (see wikiIngestService.rebuildIndexPage) on batch
// completion, which is where we actually have the LLM + change description
// context needed to rewrite it.
func (s *wikiPageService) RebuildIndexPage(ctx context.Context, kbID string) error {
	_ = ctx
	_ = kbID
	return nil
}
