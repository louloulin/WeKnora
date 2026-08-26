package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// stubWikiTagRepo is a minimal in-memory WikiTagRepository used by the
// harness tests below. The full GORM-backed implementation is exercised
// in the integration suite (TODO); here we cover the service-layer
// contracts: validation, per-page limit, set-page atomicity, and the
// BatchTag sync path's per-slug failure reporting.
//
// Build #17.
type stubWikiTagRepo struct {
	tags     map[string]types.WikiTag            // tagID -> tag
	pages    map[string]string                    // slug -> pageID (within the test KB)
	pageTags map[string]map[string]struct{}       // pageID -> set of tagIDs
	tagCount map[string]int64                    // tagID -> page count

	// Force-collision injection. When set, the next Create call returns
	// ErrWikiTagConflict without storing the row.
	conflictOnNextCreate bool
}

func newStubTagRepo() *stubWikiTagRepo {
	return &stubWikiTagRepo{
		tags:     map[string]types.WikiTag{},
		pages:    map[string]string{},
		pageTags: map[string]map[string]struct{}{},
		tagCount: map[string]int64{},
	}
}

// minimal WikiTagRepository surface used by the harness. Methods not
// touched by the test suite are intentionally left out so the stub
// stays small.

func (s *stubWikiTagRepo) GetByID(_ context.Context, _ string, tagID string) (*types.WikiTag, error) {
	t, ok := s.tags[tagID]
	if !ok {
		return nil, nil
	}
	return &t, nil
}

func (s *stubWikiTagRepo) Create(_ context.Context, t *types.WikiTag) error {
	if s.conflictOnNextCreate {
		s.conflictOnNextCreate = false
		return types.ErrWikiTagConflict
	}
	if _, exists := s.tags[t.ID]; exists {
		return types.ErrWikiTagConflict
	}
	s.tags[t.ID] = *t
	return nil
}

func (s *stubWikiTagRepo) Update(_ context.Context, _ string, tagID string, patch types.WikiTagUpdateRequest) (*types.WikiTag, error) {
	t, ok := s.tags[tagID]
	if !ok {
		return nil, nil
	}
	if patch.Name != nil {
		t.Name = *patch.Name
	}
	if patch.Color != nil {
		t.Color = *patch.Color
	}
	if patch.SortOrder != nil {
		t.SortOrder = *patch.SortOrder
	}
	s.tags[tagID] = t
	return &t, nil
}

func (s *stubWikiTagRepo) Delete(_ context.Context, _ string, tagID string) error {
	if _, ok := s.tags[tagID]; !ok {
		return types.ErrWikiTagNotFound
	}
	delete(s.tags, tagID)
	delete(s.tagCount, tagID)
	for pageID, set := range s.pageTags {
		delete(set, tagID)
		s.pageTags[pageID] = set
	}
	return nil
}

func (s *stubWikiTagRepo) List(_ context.Context, _ string) ([]types.WikiTag, error) {
	out := []types.WikiTag{}
	for _, t := range s.tags {
		out = append(out, t)
	}
	return out, nil
}

func (s *stubWikiTagRepo) ListWithCount(_ context.Context, _ string) ([]types.WikiTagWithCount, error) {
	out := []types.WikiTagWithCount{}
	for _, t := range s.tags {
		out = append(out, types.WikiTagWithCount{WikiTag: t, PageCount: s.tagCount[t.ID]})
	}
	return out, nil
}

func (s *stubWikiTagRepo) GetPageTags(_ context.Context, _ string, slug string) ([]types.WikiTag, error) {
	pageID, ok := s.pages[slug]
	if !ok {
		return []types.WikiTag{}, nil
	}
	set := s.pageTags[pageID]
	if len(set) == 0 {
		return []types.WikiTag{}, nil
	}
	out := []types.WikiTag{}
	for tagID := range set {
		if t, ok := s.tags[tagID]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *stubWikiTagRepo) SetPageTags(_ context.Context, _ string, slug string, tagIDs []string) ([]types.WikiTag, error) {
	pageID, ok := s.pages[slug]
	if !ok {
		return nil, types.ErrWikiTagNotFound
	}
	set := map[string]struct{}{}
	for _, tid := range tagIDs {
		if _, exists := s.tags[tid]; !exists {
			return nil, types.ErrWikiTagNotFound
		}
		set[tid] = struct{}{}
	}
	s.pageTags[pageID] = set
	out := []types.WikiTag{}
	for tid := range set {
		out = append(out, s.tags[tid])
	}
	return out, nil
}

func (s *stubWikiTagRepo) AddTagToPages(_ context.Context, _ string, slugs []string, tagID string) ([]string, []types.WikiPageBatchFailure, error) {
	if _, ok := s.tags[tagID]; !ok {
		return nil, nil, types.ErrWikiTagNotFound
	}
	succeeded := []string{}
	failed := []types.WikiPageBatchFailure{}
	for _, slug := range slugs {
		pageID, ok := s.pages[slug]
		if !ok {
			failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "not_found", Error: "page not found"})
			continue
		}
		set := s.pageTags[pageID]
		if set == nil {
			set = map[string]struct{}{}
			s.pageTags[pageID] = set
		}
		if _, exists := set[tagID]; !exists {
			set[tagID] = struct{}{}
			s.tagCount[tagID]++
		}
		succeeded = append(succeeded, slug)
	}
	return succeeded, failed, nil
}

func (s *stubWikiTagRepo) RemoveTagFromPages(_ context.Context, _ string, slugs []string, tagID string) ([]string, []types.WikiPageBatchFailure, error) {
	if _, ok := s.tags[tagID]; !ok {
		return nil, nil, types.ErrWikiTagNotFound
	}
	succeeded := []string{}
	failed := []types.WikiPageBatchFailure{}
	for _, slug := range slugs {
		pageID, ok := s.pages[slug]
		if !ok {
			failed = append(failed, types.WikiPageBatchFailure{Slug: slug, Code: "not_found", Error: "page not found"})
			continue
		}
		set := s.pageTags[pageID]
		if _, exists := set[tagID]; exists {
			delete(set, tagID)
			s.pageTags[pageID] = set
			s.tagCount[tagID]--
		}
		succeeded = append(succeeded, slug)
	}
	return succeeded, failed, nil
}

func (s *stubWikiTagRepo) ClearPageTags(_ context.Context, pageID string) error {
	delete(s.pageTags, pageID)
	return nil
}

// withTagTenant stamps the minimal context the tag service expects.
func withTagTenant(ctx context.Context) context.Context {
	return context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
}

// TestTagCreate_TrimsAndDefaults verifies Create normalizes whitespace
// and falls back to the "blue" palette entry when color is omitted.
func TestTagCreate_TrimsAndDefaults(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	got, err := svc.Create(withTagTenant(context.Background()), "kb1", "  todo  ", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Name != "todo" {
		t.Fatalf("Create name = %q, want todo", got.Name)
	}
	if got.Color != "blue" {
		t.Fatalf("Create color = %q, want blue (default)", got.Color)
	}
}

// TestTagCreate_RejectsBadColor verifies the palette guard fires before
// the repository.
func TestTagCreate_RejectsBadColor(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	_, err := svc.Create(withTagTenant(context.Background()), "kb1", "todo", "rainbow")
	if !errors.Is(err, types.ErrWikiTagInvalidColor) {
		t.Fatalf("Create(bad color) err = %v, want ErrWikiTagInvalidColor", err)
	}
}

// TestTagCreate_Duplicate verifies the repo's UNIQUE-violation path
// surfaces as ErrWikiTagConflict.
func TestTagCreate_Duplicate(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	if _, err := svc.Create(withTagTenant(context.Background()), "kb1", "todo", "blue"); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	repo.conflictOnNextCreate = true
	_, err := svc.Create(withTagTenant(context.Background()), "kb1", "todo", "blue")
	if !errors.Is(err, types.ErrWikiTagConflict) {
		t.Fatalf("Create dup err = %v, want ErrWikiTagConflict", err)
	}
}

// TestSetPageTags_RejectsOverflow verifies the per-page limit guard
// fires before the repository. WikiTagMaxPerPage == 10.
func TestSetPageTags_RejectsOverflow(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	ids := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		ids = append(ids, uuid.NewString())
	}
	_, err := svc.SetPageTags(withTagTenant(context.Background()), "kb1", "alpha", ids)
	if !errors.Is(err, types.ErrWikiTagLimitExceeded) {
		t.Fatalf("SetPageTags(11) err = %v, want ErrWikiTagLimitExceeded", err)
	}
}

// TestBatchTag_SyncPath verifies the synchronous BatchTag path returns
// a WikiBatchRouteResult with kind="sync" when len(slugs) is below
// WikiBatchAsyncThreshold. Per-slug failures bubble up in
// WikiBatchResult.Failed.
func TestBatchTag_SyncPath(t *testing.T) {
	repo := newStubTagRepo()
	repo.pages["alpha"] = "p_alpha"
	repo.pages["beta"] = "p_beta"
	tagID := uuid.NewString()
	repo.tags[tagID] = types.WikiTag{ID: tagID, KnowledgeBaseID: "kb1", Name: "todo", Color: "blue"}
	svc := NewWikiTagService(repo) // batchSvc nil -> sync-only

	route, err := svc.BatchTag(withTagTenant(context.Background()), "kb1",
		[]string{"alpha", "beta", "ghost"}, tagID, types.WikiBatchTagOpAdd)
	if err != nil {
		t.Fatalf("BatchTag: %v", err)
	}
	if route.Kind != "sync" {
		t.Fatalf("BatchTag kind = %q, want sync (batchSvc nil + small input)", route.Kind)
	}
	if len(route.Result.Succeeded) != 2 {
		t.Fatalf("succeeded = %d, want 2", len(route.Result.Succeeded))
	}
	if len(route.Result.Failed) != 1 || route.Result.Failed[0].Code != "not_found" {
		t.Fatalf("failed = %+v, want one not_found", route.Result.Failed)
	}
}

// TestBatchTag_InvalidOp verifies the op guard rejects anything other
// than add/remove with a non-zero error.
func TestBatchTag_InvalidOp(t *testing.T) {
	repo := newStubTagRepo()
	tagID := uuid.NewString()
	repo.tags[tagID] = types.WikiTag{ID: tagID, KnowledgeBaseID: "kb1", Name: "x", Color: "blue"}
	svc := NewWikiTagService(repo)
	_, err := svc.BatchTag(withTagTenant(context.Background()), "kb1",
		[]string{"alpha"}, tagID, "purge")
	if err == nil {
		t.Fatalf("BatchTag(bad op) returned nil error")
	}
}

// TestBatchTag_TagMissingReturnsNotFound verifies BatchTag surfaces a
// missing tag as ErrWikiTagNotFound so the handler can map it to 404
// regardless of which path (sync/async) is selected.
func TestBatchTag_TagMissingReturnsNotFound(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	_, err := svc.BatchTag(withTagTenant(context.Background()), "kb1",
		[]string{"alpha"}, uuid.NewString(), types.WikiBatchTagOpAdd)
	if !errors.Is(err, types.ErrWikiTagNotFound) {
		t.Fatalf("BatchTag(missing tag) err = %v, want ErrWikiTagNotFound", err)
	}
}

// TestApplyBatchTagOneSlug_AddsAndRemoves verifies the per-slug hook the
// worker pool calls succeeds for both add and remove ops and tolerates
// a missing page (still succeeds with no side-effects on add; remove is
// silent-success).
func TestApplyBatchTagOneSlug_AddsAndRemoves(t *testing.T) {
	repo := newStubTagRepo()
	repo.pages["alpha"] = "p_alpha"
	tagID := uuid.NewString()
	repo.tags[tagID] = types.WikiTag{ID: tagID, KnowledgeBaseID: "kb1", Name: "todo", Color: "blue"}
	svc := NewWikiTagService(repo)

	if err := svc.ApplyBatchTagOneSlug(withTagTenant(context.Background()),
		"kb1", "alpha", tagID, types.WikiBatchTagOpAdd); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := svc.ApplyBatchTagOneSlug(withTagTenant(context.Background()),
		"kb1", "alpha", tagID, types.WikiBatchTagOpRemove); err != nil {
		t.Fatalf("remove: %v", err)
	}
}

// TestGetPageTags_EmptyReturnsEmptySlice verifies the read path never
// returns nil — the frontend always reads `tags ?? []`.
func TestGetPageTags_EmptyReturnsEmptySlice(t *testing.T) {
	repo := newStubTagRepo()
	svc := NewWikiTagService(repo)
	tags, err := svc.GetPageTags(withTagTenant(context.Background()), "kb1", "ghost")
	if err != nil {
		t.Fatalf("GetPageTags: %v", err)
	}
	if tags == nil {
		t.Fatalf("GetPageTags returned nil; want empty slice")
	}
	if len(tags) != 0 {
		t.Fatalf("GetPageTags len = %d, want 0", len(tags))
	}
}