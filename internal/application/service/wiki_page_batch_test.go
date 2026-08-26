package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

// stubBatchRepo is the minimal in-memory WikiPageRepository implementation
// the batch service tests need. It exercises only the methods the three
// Batch* service methods actually touch (GetBySlug, GetFolderByID,
// UpdateMeta, Delete, DeleteRevisionsByPage). It panics on any other call
// so a future change that adds coupling fails loudly in the harness
// instead of silently dropping data.
//
// Build #12.
type stubBatchRepo struct {
	pages    map[string]*types.WikiPage // slug -> page (KB-scoped via slug uniqueness)
	folders  map[string]*types.WikiFolder // folderID -> folder
	getErr   error
	updateErr error
}

func newStubBatchRepo() *stubBatchRepo {
	return &stubBatchRepo{
		pages:   map[string]*types.WikiPage{},
		folders: map[string]*types.WikiFolder{},
	}
}

func (s *stubBatchRepo) addPage(slug string, status string, outLinks []string, folderID string) *types.WikiPage {
	return s.addPageInKB("kb1", slug, status, outLinks, folderID)
}

func (s *stubBatchRepo) addPageInKB(
	kbID string, slug string, status string, outLinks []string, folderID string,
) *types.WikiPage {
	p := &types.WikiPage{
		ID:              "id-" + kbID + "-" + slug,
		KnowledgeBaseID: kbID,
		Slug:            slug,
		Title:           slug,
		PageType:        types.WikiPageTypeConcept,
		Status:          status,
		OutLinks:        types.StringArray(outLinks),
		FolderID:        folderID,
		UpdatedAt:       time.Now(),
	}
	s.pages[slug] = p
	return p
}

func (s *stubBatchRepo) GetBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	p, ok := s.pages[slug]
	if !ok {
		return nil, repository.ErrWikiPageNotFound
	}
	if p.KnowledgeBaseID != kbID {
		// Matches real repo semantics: a KB-scoped query filters out
		// cross-KB rows so callers see `not_found`.
		return nil, repository.ErrWikiPageNotFound
	}
	cp := *p
	return &cp, nil
}

func (s *stubBatchRepo) GetBySlugAcrossKB(ctx context.Context, slug string) (*types.WikiPage, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	p, ok := s.pages[slug]
	if !ok {
		return nil, repository.ErrWikiPageNotFound
	}
	cp := *p
	return &cp, nil
}

func (s *stubBatchRepo) GetFolderByID(ctx context.Context, kbID string, id string) (*types.WikiFolder, error) {
	f, ok := s.folders[id]
	if !ok {
		return nil, repository.ErrWikiFolderNotFound
	}
	cp := *f
	return &cp, nil
}

func (s *stubBatchRepo) UpdateMeta(ctx context.Context, page *types.WikiPage) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if p, ok := s.pages[page.Slug]; ok {
		p.Status = page.Status
		p.FolderID = page.FolderID
		p.CategoryPath = page.CategoryPath
		p.Depth = page.Depth
		p.UpdatedAt = page.UpdatedAt
	}
	return nil
}

func (s *stubBatchRepo) Delete(ctx context.Context, kbID string, slug string) error {
	delete(s.pages, slug)
	return nil
}

func (s *stubBatchRepo) DeleteRevisionsByPage(ctx context.Context, pageID string) error {
	return nil
}

// Anything else is unexpected. Listing the methods explicitly would balloon
// the stub; panicking keeps the harness honest without that surface area.

// normalize-batch-slugs helper

func TestNormalizeBatchSlugs_DropsEmptyAndDedupes(t *testing.T) {
	got := normalizeBatchSlugs([]string{" a ", "", "b", "a", "c", "  "})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("position %d: got %q want %q", i, got[i], w)
		}
	}
}

// classifyBatchError

func TestClassifyBatchError_KnownSentinels(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{repository.ErrWikiPageNotFound, "not_found"},
		{repository.ErrWikiFolderNotFound, "folder_not_found"},
		{repository.ErrWikiFolderConflict, "folder_conflict"},
		{repository.ErrWikiFolderNotEmpty, "folder_not_empty"},
		{errors.New("random DB blip"), "internal"},
		{nil, ""},
	}
	for _, c := range cases {
		got := classifyBatchError(c.err)
		if got != c.want {
			t.Errorf("classifyBatchError(%v): got %q want %q", c.err, got, c.want)
		}
	}
}

// Build #12 service tests

func TestBatchMovePages_AllSucceed(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	repo.addPage("c", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchMovePages(context.Background(), "kb1", []string{"a", "b", "c"}, "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 3 || len(got.Failed) != 0 {
		t.Fatalf("got succeeded=%v failed=%v", got.Succeeded, got.Failed)
	}
	for _, slug := range []string{"a", "b", "c"} {
		if repo.pages[slug].FolderID != "root" {
			t.Errorf("page %s FolderID = %q, want root", slug, repo.pages[slug].FolderID)
		}
	}
}

func TestBatchMovePages_PartialFailureIsolated(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchMovePages(context.Background(), "kb1", []string{"a", "missing", "b"}, "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 2 || len(got.Failed) != 1 {
		t.Fatalf("got succeeded=%v failed=%v", got.Succeeded, got.Failed)
	}
	if got.Failed[0].Slug != "missing" || got.Failed[0].Code != "not_found" {
		t.Fatalf("failed entry wrong: %+v", got.Failed[0])
	}
}

func TestBatchMovePages_DedupesAndTrims(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchMovePages(context.Background(), "kb1", []string{" a ", "a", "b", "  "}, "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 2 || len(got.Failed) != 0 {
		t.Fatalf("got succeeded=%v failed=%v", got.Succeeded, got.Failed)
	}
}

func TestBatchDeletePages_AllSucceed(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchDeletePages(context.Background(), "kb1", []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 2 {
		t.Fatalf("got %+v", got)
	}
	if _, ok := repo.pages["a"]; ok {
		t.Errorf("page a should be deleted")
	}
}

func TestBatchDeletePages_PartialFailureIsolated(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchDeletePages(context.Background(), "kb1", []string{"a", "ghost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 1 || len(got.Failed) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestBatchUpdatePageStatus_AllSucceed(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusDraft, nil, "")
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchUpdatePageStatus(context.Background(), "kb1", []string{"a", "b"}, types.WikiPageStatusArchived)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 2 || len(got.Failed) != 0 {
		t.Fatalf("got %+v", got)
	}
	if repo.pages["a"].Status != types.WikiPageStatusArchived {
		t.Errorf("a status = %q", repo.pages["a"].Status)
	}
}

func TestBatchUpdatePageStatus_RejectsInvalidStatus(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	_, err := svc.BatchUpdatePageStatus(context.Background(), "kb1", []string{"a"}, "invalid")
	if err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestBatchUpdatePageStatus_NoOpSkipsUpdate(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusArchived, nil, "")
	originalUpdated := repo.pages["a"].UpdatedAt
	svc := &wikiPageService{repo: repo}
	got, err := svc.BatchUpdatePageStatus(context.Background(), "kb1", []string{"a"}, types.WikiPageStatusArchived)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Succeeded) != 1 {
		t.Fatalf("got %+v", got)
	}
	// No-op should NOT have touched UpdatedAt — the user didn't change
	// anything, so the bookkeeping row stays put.
	if !repo.pages["a"].UpdatedAt.Equal(originalUpdated) {
		t.Errorf("no-op should not bump UpdatedAt: was %v now %v", originalUpdated, repo.pages["a"].UpdatedAt)
	}
}

// Cross-KB detection (A6, D2). A slug that exists in a different KB must
// abort the whole batch with kb_mismatch — never surface as a per-row
// `not_found` (which would mask a stale-client bug).

func TestBatchMovePages_CrossKBRejects(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPageInKB("kb-other", "shared-slug", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	_, err := svc.BatchMovePages(context.Background(), "kb1", []string{"shared-slug"}, "root")
	if err == nil {
		t.Fatalf("expected kb_mismatch error, got nil")
	}
	var mismatch *types.WikiBatchKBMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *types.WikiBatchKBMismatchError, got %T: %v", err, err)
	}
	if mismatch.Slug != "shared-slug" || mismatch.ActualKB != "kb-other" {
		t.Errorf("mismatch detail wrong: %+", mismatch)
	}
	if !types.IsWikiBatchKBMismatch(err) {
		t.Errorf("IsWikiBatchKBMismatch returned false")
	}
}

func TestBatchDeletePages_CrossKBRejects(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPageInKB("kb-other", "shared-slug", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	_, err := svc.BatchDeletePages(context.Background(), "kb1", []string{"shared-slug"})
	if !types.IsWikiBatchKBMismatch(err) {
		t.Fatalf("expected kb_mismatch, got %v", err)
	}
}

func TestBatchUpdatePageStatus_CrossKBRejects(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPageInKB("kb-other", "shared-slug", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	_, err := svc.BatchUpdatePageStatus(context.Background(), "kb1", []string{"shared-slug"}, types.WikiPageStatusArchived)
	if !types.IsWikiBatchKBMismatch(err) {
		t.Fatalf("expected kb_mismatch, got %v", err)
	}
}

// assertBatchKBOwnership helper

func TestAssertBatchKBOwnership_PassesForSameKB(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	if err := svc.assertBatchKBOwnership(context.Background(), "kb1", []string{"a", "b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertBatchKBOwnership_AllowsMissingSlugs(t *testing.T) {
	// Missing slugs are NOT a kb_mismatch — they fall through to per-row
	// not_found in the partial-success result.
	repo := newStubBatchRepo()
	svc := &wikiPageService{repo: repo}
	if err := svc.assertBatchKBOwnership(context.Background(), "kb1", []string{"missing"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertBatchKBOwnership_FlagsFirstCrossKB(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "") // kb1
	repo.addPageInKB("kb-other", "shared", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}
	err := svc.assertBatchKBOwnership(context.Background(), "kb1", []string{"a", "shared"})
	if err == nil {
		t.Fatalf("expected kb_mismatch")
	}
	var mismatch *types.WikiBatchKBMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *WikiBatchKBMismatchError, got %T", err)
	}
	if mismatch.Slug != "shared" || mismatch.ActualKB != "kb-other" {
		t.Errorf("mismatch detail wrong: %+", mismatch)
	}
}