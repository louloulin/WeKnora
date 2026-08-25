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