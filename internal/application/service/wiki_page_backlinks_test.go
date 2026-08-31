package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

// stubBacklinksRepo is a minimal in-memory WikiPageRepository used by
// the `ListPageBacklinks` unit tests below. It exercises only the two
// methods `wikiPageService.ListPageBacklinks` actually touches — the
// rest of the repo is irrelevant to backlinks and panics if called so
// we notice unintended coupling early.
//
// Build #11.
type stubBacklinksRepo struct {
	getCalls int // test-side counter
	calls    []string

	target   *types.WikiPage        // page whose `in_links` we query
	lites    map[string]*types.WikiPageLite // slug -> lite projection
	getErr   error
	listErr  error
}

func (s *stubBacklinksRepo) GetBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.target == nil || s.target.Slug != slug {
		return nil, repository.ErrWikiPageNotFound
	}
	cp := *s.target
	return &cp, nil
}

func (s *stubBacklinksRepo) ListBySlugs(ctx context.Context, kbID string, slugs []string) (map[string]*types.WikiPageLite, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := map[string]*types.WikiPageLite{}
	for _, sl := range slugs {
		if lite, ok := s.lites[sl]; ok {
			out[sl] = lite
		}
	}
	return out, nil
}

func newBacklinksService(repo *stubBacklinksRepo) *wikiPageService {
	return &wikiPageService{repo: repo}
}

func mustLite(slug, title, ptype, status string, updatedAt time.Time) *types.WikiPageLite {
	return &types.WikiPageLite{
		Slug:      slug,
		Title:     title,
		PageType:  ptype,
		Status:    status,
		UpdatedAt: updatedAt,
	}
}

func TestListPageBacklinks_EmptyInLinksReturnsEmpty(t *testing.T) {
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:     "summary/intro",
			InLinks:  types.StringArray{},
		},
		lites: map[string]*types.WikiPageLite{},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListPageBacklinks(context.Background(), "kb1", "summary/intro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(got))
	}
}

func TestListPageBacklinks_MultipleLiveSourcesOrderedByUpdatedDesc(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	_ = t0
	t1 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"entity/acme", "entity/globex", "concept/market"},
		},
		lites: map[string]*types.WikiPageLite{
			"entity/acme":    mustLite("entity/acme", "Acme", "entity", "live", t1),
			"entity/globex":  mustLite("entity/globex", "Globex", "entity", "live", t2),
			"concept/market": mustLite("concept/market", "Market", "concept", "live", t1),
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListPageBacklinks(context.Background(), "kb1", "summary/intro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	// t1 tie → alphabetical slug order: concept/market < entity/acme.
	if got[0].Slug != "concept/market" || got[1].Slug != "entity/acme" || got[2].Slug != "entity/globex" {
		t.Fatalf("ordering wrong: %s, %s, %s", got[0].Slug, got[1].Slug, got[2].Slug)
	}
}

func TestListPageBacklinks_OrphanSlugFiltered(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"entity/acme", "entity/deleted"},
		},
		lites: map[string]*types.WikiPageLite{
			"entity/acme": mustLite("entity/acme", "Acme", "entity", "live", t0),
			// "entity/deleted" intentionally absent → orphan
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListPageBacklinks(context.Background(), "kb1", "summary/intro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry (orphan filtered), got %d", len(got))
	}
	if got[0].Slug != "entity/acme" {
		t.Fatalf("expected entity/acme, got %s", got[0].Slug)
	}
}

func TestListPageBacklinks_SelfSlugInInLinksExcluded(t *testing.T) {
	t0 := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	repo := &stubBacklinksRepo{
		target: &types.WikiPage{
			Slug:    "summary/intro",
			InLinks: types.StringArray{"summary/intro", "entity/acme"},
		},
		lites: map[string]*types.WikiPageLite{
			"entity/acme": mustLite("entity/acme", "Acme", "entity", "live", t0),
		},
	}
	svc := newBacklinksService(repo)
	got, err := svc.ListPageBacklinks(context.Background(), "kb1", "summary/intro")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry (self excluded), got %d", len(got))
	}
	if got[0].Slug != "entity/acme" {
		t.Fatalf("expected entity/acme, got %s", got[0].Slug)
	}
}


// ============================================================================
// Auto-generated WikiPageRepository no-op stubs.
// ============================================================================
func (r *stubBacklinksRepo) Create(_ context.Context, _ *types.WikiPage) error { return nil }
func (r *stubBacklinksRepo) Update(_ context.Context, _ *types.WikiPage) error { return nil }
func (r *stubBacklinksRepo) UpdateMeta(_ context.Context, _ *types.WikiPage) error { return nil }
func (r *stubBacklinksRepo) UpdateAutoLinkedContent(_ context.Context, _ *types.WikiPage) error { return nil }
func (r *stubBacklinksRepo) GetByID(_ context.Context, _ string) (*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) GetBySlugAcrossKB(_ context.Context, _ string) (*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) ListBacklinksAcrossKBs(_ context.Context, _ uint64, _ string, _ string, _ int) ([]*types.WikiPageLite, error) { return nil, nil }
func (r *stubBacklinksRepo) List(_ context.Context, _ *types.WikiPageListRequest) ([]*types.WikiPage, int64, error) { return nil, 0, nil }
func (r *stubBacklinksRepo) ListByType(_ context.Context, _ string, _ string) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) ListByTypeLight(_ context.Context, _ string, _ string, _ int, _ int) ([]types.WikiIndexEntry, int64, error) { return nil, 0, nil }
func (r *stubBacklinksRepo) ListBySourceRef(_ context.Context, _ string, _ string) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) ListSlugsBySourceRef(_ context.Context, _ string, _ string) ([]string, error) { return nil, nil }
func (r *stubBacklinksRepo) ListSummariesByKnowledgeIDs(_ context.Context, _ string, _ []string) (map[string]string, error) { return nil, nil }
func (r *stubBacklinksRepo) ExistsSlugs(_ context.Context, _ string, _ []string) (map[string]bool, error) { return nil, nil }
func (r *stubBacklinksRepo) ListAllSlugs(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (r *stubBacklinksRepo) ListPagesCursor(_ context.Context, _ string, _ string, _ int) ([]*types.WikiPage, string, error) { return nil, "", nil }
func (r *stubBacklinksRepo) ListByTypeRecent(_ context.Context, _ string, _ string, _ int) ([]types.WikiIndexEntry, error) { return nil, nil }
func (r *stubBacklinksRepo) FindSimilarPages(_ context.Context, _ string, _ string, _ []string, _ int) ([]*types.WikiPageLite, error) { return nil, nil }
func (r *stubBacklinksRepo) FindPagesMissingTSZh(_ context.Context, _ int) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) UpdateContentTSZh(_ context.Context, _ string, _ string) error { return nil }
func (r *stubBacklinksRepo) FindPagesByNormalizedTitle(_ context.Context, _ string, _ string, _ string) ([]*types.WikiPageLite, error) { return nil, nil }
func (r *stubBacklinksRepo) FindPagesByNormalizedTitles(_ context.Context, _ string, _ string, _ []string) ([]*types.WikiPageLite, error) { return nil, nil }
func (r *stubBacklinksRepo) ListDistinctCategoryPaths(_ context.Context, _ string, _ int) ([][]string, error) { return nil, nil }
func (r *stubBacklinksRepo) CreateFolder(_ context.Context, _ *types.WikiFolder) error { return nil }
func (r *stubBacklinksRepo) GetFolderByID(_ context.Context, _ string, _ string) (*types.WikiFolder, error) { return nil, nil }
func (r *stubBacklinksRepo) GetChildFolderByName(_ context.Context, _ string, _ string, _ string) (*types.WikiFolder, error) { return nil, nil }
func (r *stubBacklinksRepo) ListChildFolders(_ context.Context, _ string, _ string) ([]*types.WikiFolder, error) { return nil, nil }
func (r *stubBacklinksRepo) ListAllFolders(_ context.Context, _ string) ([]*types.WikiFolder, error) { return nil, nil }
func (r *stubBacklinksRepo) UpdateFolder(_ context.Context, _ *types.WikiFolder) error { return nil }
func (r *stubBacklinksRepo) DeleteFolder(_ context.Context, _ string, _ string) error { return nil }
func (r *stubBacklinksRepo) CountPagesInFolder(_ context.Context, _ string, _ string) (int64, error) { return 0, nil }
func (r *stubBacklinksRepo) CountPagesByFolder(_ context.Context, _ string, _ []string) (map[string]int64, error) { return nil, nil }
func (r *stubBacklinksRepo) ListPagesByFolderIDs(_ context.Context, _ string, _ []string) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) ListAll(_ context.Context, _ string) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) ListRecentForSuggestions(_ context.Context, _ uint64, _ []string, _ int) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) Delete(_ context.Context, _ string, _ string) error { return nil }
func (r *stubBacklinksRepo) DeleteByID(_ context.Context, _ string) error { return nil }
func (r *stubBacklinksRepo) RestoreDeleted(_ context.Context, _ string, _ string, _ string) error { return nil }
func (r *stubBacklinksRepo) Search(_ context.Context, _ string, _ string, _ int) ([]*types.WikiPage, error) { return nil, nil }
func (r *stubBacklinksRepo) CountByType(_ context.Context, _ string) (map[string]int64, error) { return nil, nil }
func (r *stubBacklinksRepo) CountOrphans(_ context.Context, _ string) (int64, error) { return 0, nil }
func (r *stubBacklinksRepo) UpdateWithRevision(_ context.Context, _ *types.WikiPage, _ *types.WikiPageRevision) error { return nil }
func (r *stubBacklinksRepo) ListRevisions(_ context.Context, _ string, _ string, _ int, _ int) ([]*types.WikiPageRevision, int64, error) { return nil, 0, nil }
func (r *stubBacklinksRepo) GetRevision(_ context.Context, _ string, _ string, _ int) (*types.WikiPageRevision, error) { return nil, nil }
func (r *stubBacklinksRepo) PruneRevisions(_ context.Context, _ types.WikiRevisionPruneRequest) error { return nil }
func (r *stubBacklinksRepo) DeleteRevisionsByPage(_ context.Context, _ string) error { return nil }
func (r *stubBacklinksRepo) CreateIssue(_ context.Context, _ *types.WikiPageIssue) error { return nil }
func (r *stubBacklinksRepo) ListIssues(_ context.Context, _ string, _ string, _ string) ([]*types.WikiPageIssue, error) { return nil, nil }
func (r *stubBacklinksRepo) UpdateIssueStatus(_ context.Context, _ string, _ string) error { return nil }

