package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiTagService owns the user-facing wiki tag system. Two layers:
//
//  1. Tag-definition CRUD (List / Get / Create / Update / Delete) —
//     runs entirely against the WikiTagRepository; no worker pool.
//  2. Per-page and per-batch associations (GetPageTags / SetPageTags /
//     BatchTag) — SetPageTags is synchronous; BatchTag mirrors the
//     existing Batch{Move,Delete,Status}Route pattern (sync under the
//     threshold, queued at/above it).
//
// batchSvc is wired post-construction via SetBatchJobService to break
// the WikiBatchJobService ↔ WikiTagService cycle (the batch service
// already needs the tag service for per-slug execution).
//
// Build #17.
type wikiTagService struct {
	repo     interfaces.WikiTagRepository
	batchSvc interfaces.WikiBatchJobService
}

// NewWikiTagService constructs the service. The batch job service is
// wired later via SetBatchJobService — see wireWikiBatchJobService.
//
// Build #17.
func NewWikiTagService(repo interfaces.WikiTagRepository) interfaces.WikiTagService {
	return &wikiTagService{repo: repo}
}

// SetBatchJobService attaches the async batch job service post-
// construction. nil is a valid value: callers that pre-date the async
// path (legacy harness tests) keep working as sync-only, mirroring
// how WikiPageService handles the same edge.
//
// Build #17.
func (s *wikiTagService) SetBatchJobService(svc interfaces.WikiBatchJobService) {
	s.batchSvc = svc
}

// List returns every tag in the KB with its current page_count. The
// repository performs a single LEFT JOIN + GROUP BY so the call costs
// one round-trip regardless of tag count.
func (s *wikiTagService) List(ctx context.Context, kbID string) ([]types.WikiTagWithCount, error) {
	if kbID == "" {
		return nil, fmt.Errorf("kb id required")
	}
	return s.repo.ListWithCount(ctx, kbID)
}

// Get returns a single tag scoped to the KB. Returns ErrWikiTagNotFound
// when the tag does not exist or lives in a different KB (the handler
// maps both to 404 to avoid leaking cross-KB existence).
func (s *wikiTagService) Get(ctx context.Context, kbID, tagID string) (*types.WikiTag, error) {
	if kbID == "" || tagID == "" {
		return nil, types.ErrWikiTagNotFound
	}
	tag, err := s.repo.GetByID(ctx, kbID, tagID)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, types.ErrWikiTagNotFound
	}
	return tag, nil
}

// Create inserts a new tag. Pre-validates name + color so the repo never
// sees a row the DB would reject for shape reasons.
func (s *wikiTagService) Create(ctx context.Context, kbID, name, color string) (*types.WikiTag, error) {
	cleaned, err := validateWikiTagName(name)
	if err != nil {
		return nil, err
	}
	if color == "" {
		color = "blue"
	}
	if !types.IsValidWikiTagColor(color) {
		return nil, types.ErrWikiTagInvalidColor
	}
	if kbID == "" {
		return nil, fmt.Errorf("kb id required")
	}
	tag := &types.WikiTag{
		ID:              uuid.NewString(),
		TenantID:        types.TenantIDFromContextOrZero(ctx),
		KnowledgeBaseID: kbID,
		Name:            cleaned,
		Color:           color,
		SortOrder:       0,
	}
	if err := s.repo.Create(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

// Update applies a partial patch. nil fields are left untouched. Name
// and color are re-validated; an empty patch returns the current row.
func (s *wikiTagService) Update(ctx context.Context, kbID, tagID string, patch types.WikiTagUpdateRequest) (*types.WikiTag, error) {
	if kbID == "" || tagID == "" {
		return nil, types.ErrWikiTagNotFound
	}
	if patch.Name != nil {
		cleaned, err := validateWikiTagName(*patch.Name)
		if err != nil {
			return nil, err
		}
		patch.Name = &cleaned
	}
	if patch.Color != nil && !types.IsValidWikiTagColor(*patch.Color) {
		return nil, types.ErrWikiTagInvalidColor
	}
	updated, err := s.repo.Update(ctx, kbID, tagID, patch)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, types.ErrWikiTagNotFound
	}
	return updated, nil
}

// Delete removes the tag definition. wiki_page_tags rows cascade at the
// DB level; the call still returns ErrWikiTagNotFound for the missing
// case so the handler can answer 404.
func (s *wikiTagService) Delete(ctx context.Context, kbID, tagID string) error {
	if kbID == "" || tagID == "" {
		return types.ErrWikiTagNotFound
	}
	if err := s.repo.Delete(ctx, kbID, tagID); err != nil {
		if errors.Is(err, types.ErrWikiTagNotFound) {
			return types.ErrWikiTagNotFound
		}
		return err
	}
	return nil
}

// GetPageTags returns the tags currently attached to a single page.
// Empty slice (not error) when the page has no tags.
func (s *wikiTagService) GetPageTags(ctx context.Context, kbID, slug string) ([]types.WikiTag, error) {
	if kbID == "" || slug == "" {
		return []types.WikiTag{}, nil
	}
	tags, err := s.repo.GetPageTags(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		return []types.WikiTag{}, nil
	}
	return tags, nil
}

// SetPageTags atomically replaces the join rows for one page. Validates
// the cap up front (WikiTagMaxPerPage) so a transaction is never opened
// for an obviously-invalid request.
func (s *wikiTagService) SetPageTags(ctx context.Context, kbID, slug string, tagIDs []string) ([]types.WikiTag, error) {
	if kbID == "" || slug == "" {
		return nil, types.ErrWikiTagNotFound
	}
	if len(tagIDs) > types.WikiTagMaxPerPage {
		return nil, types.ErrWikiTagLimitExceeded
	}
	// Dedup before passing the list down — SetPageTags treats the input
	// as the canonical tag set; duplicates don't change semantics but
	// would inflate the KB-bound verification count.
	deduped := dedupStrings(tagIDs)
	resolved, err := s.repo.SetPageTags(ctx, kbID, slug, deduped)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return []types.WikiTag{}, nil
	}
	return resolved, nil
}

// BatchTag is the auto-routing counterpart of SetPageTags. It mirrors
// Batch{Move,Delete,Status}Route exactly:
//
//   - below the threshold (or with no batch service wired): runs in
//     process and returns WikiBatchRouteResult{Kind:"sync"} with the
//     per-row result
//   - at/above the threshold: enqueues a WikiBatchJob of type='tag'
//     and returns WikiBatchRouteResult{Kind:"job"}
//
// The worker pool's executeOneSlug handles the per-slug execution
// (Build #17 integrates WikiBatchJobTypeTag there).
func (s *wikiTagService) BatchTag(ctx context.Context, kbID string, slugs []string, tagID, op string) (*types.WikiBatchRouteResult, error) {
	if kbID == "" {
		return nil, fmt.Errorf("kb id required")
	}
	if op != types.WikiBatchTagOpAdd && op != types.WikiBatchTagOpRemove {
		return nil, types.ErrWikiTagInvalidName // reuse sentinel; never reaches the wire
	}
	clean := normalizeBatchSlugs(slugs)
	if len(clean) == 0 {
		return nil, fmt.Errorf("at least one slug required")
	}
	// The tag must exist in this KB regardless of which path is taken.
	// Resolving once up front means the worker doesn't have to.
	tag, err := s.repo.GetByID(ctx, kbID, tagID)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, types.ErrWikiTagNotFound
	}
	if s.batchSvc == nil || len(clean) < types.WikiBatchAsyncThreshold {
		result, err := s.applyBatchTagSync(ctx, kbID, clean, tagID, op)
		if err != nil {
			return nil, err
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	params, err := json.Marshal(types.WikiBatchJobParams{
		Slugs:  clean,
		Status: tagID + "|" + op, // overload Status as the tag-op pair
	})
	if err != nil {
		return nil, err
	}
	job := &types.WikiBatchJob{
		TenantID:        types.TenantIDFromContextOrZero(ctx),
		KnowledgeBaseID: kbID,
		Type:            types.WikiBatchJobTypeTag,
		Params:          params,
		CreatedAt:       time.Now(),
	}
	jobID, err := s.batchSvc.EnqueueJob(ctx, job)
	if err != nil {
		logger.Warnf(ctx, "wiki batch tag queue overflow, falling back to sync: %v", err)
		result, sErr := s.applyBatchTagSync(ctx, kbID, clean, tagID, op)
		if sErr != nil {
			return nil, sErr
		}
		return &types.WikiBatchRouteResult{Kind: "sync", Result: result}, nil
	}
	job.ID = jobID
	return &types.WikiBatchRouteResult{Kind: "job", Job: job}, nil
}

// applyBatchTagSync runs add/remove for every slug in the same process.
// The repo handles cross-KB safety + per-slug failure isolation, so
// this stays a thin loop.
func (s *wikiTagService) applyBatchTagSync(ctx context.Context, kbID string, slugs []string, tagID, op string) (*types.WikiBatchResult, error) {
	var succeeded []string
	var failures []types.WikiPageBatchFailure
	switch op {
	case types.WikiBatchTagOpAdd:
		s, f, err := s.repo.AddTagToPages(ctx, kbID, slugs, tagID)
		if err != nil {
			return nil, err
		}
		succeeded = s
		failures = f
	case types.WikiBatchTagOpRemove:
		s, f, err := s.repo.RemoveTagFromPages(ctx, kbID, slugs, tagID)
		if err != nil {
			return nil, err
		}
		succeeded = s
		failures = f
	}
	return &types.WikiBatchResult{Succeeded: succeeded, Failed: failures}, nil
}

// ApplyBatchTagOneSlug is the single-slug hook the worker pool calls
// from executeOneSlug. Lives on the service so the worker stays free
// of tag repository imports. Returns nil on success (idempotent), or
// one of the sentinel errors the worker translates into a
// WikiBatchJobFailureRecord.
func (s *wikiTagService) ApplyBatchTagOneSlug(ctx context.Context, kbID, slug, tagID, op string) error {
	if op != types.WikiBatchTagOpAdd && op != types.WikiBatchTagOpRemove {
		return fmt.Errorf("unsupported tag op %q", op)
	}
	if op == types.WikiBatchTagOpAdd {
		_, failed, err := s.repo.AddTagToPages(ctx, kbID, []string{slug}, tagID)
		if err != nil {
			return err
		}
		if len(failed) > 0 {
			return errors.New(failed[0].Error)
		}
		return nil
	}
	_, failed, err := s.repo.RemoveTagFromPages(ctx, kbID, []string{slug}, tagID)
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		return errors.New(failed[0].Error)
	}
	return nil
}

// validateWikiTagName enforces the name rules at the service layer.
// Empty / whitespace-only / overlong names are rejected; trim is
// applied so trailing whitespace doesn't slip past the UNIQUE check.
func validateWikiTagName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", types.ErrWikiTagInvalidName
	}
	if len([]rune(trimmed)) > types.WikiTagNameMaxLength {
		return "", types.ErrWikiTagInvalidName
	}
	return trimmed, nil
}
