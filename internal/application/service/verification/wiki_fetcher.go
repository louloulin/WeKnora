package verification

import (
	"context"
	"errors"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiPageFetcher adapts the wiki_page repository to the verification
// PageFetcher interface. We intentionally project fields rather than
// passing the full repo so the verification package has zero coupling
// to GORM and can be tested with a simple fake.
type WikiPageFetcher struct {
	repo WikiPageRepoForVerification
}

// WikiPageRepoForVerification is the slice of the wiki_page repo the
// scanner needs. Declared locally so the verification package doesn't
// depend on the wider wiki_page repo interface.
type WikiPageRepoForVerification interface {
	GetBySlug(ctx context.Context, kbID, slug string) (*types.WikiPage, error)
	ListAll(ctx context.Context, kbID string) ([]*types.WikiPage, error)
	ListBySlugs(ctx context.Context, kbID string, slugs []string) (map[string]*types.WikiPageLite, error)
}

// NewWikiPageFetcher wraps a wiki_page repo.
func NewWikiPageFetcher(repo WikiPageRepoForVerification) *WikiPageFetcher {
	return &WikiPageFetcher{repo: repo}
}

// GetPage implements PageFetcher.GetPage by projecting a wiki_page row
// into the summary the scanner needs.
func (f *WikiPageFetcher) GetPage(ctx context.Context, kbID, slug string) (*PageSummary, error) {
	if f == nil || f.repo == nil {
		return nil, errors.New("verification: nil fetcher")
	}
	page, err := f.repo.GetBySlug(ctx, kbID, slug)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return nil, nil
	}
	return toSummary(page), nil
}

// ListSlugs implements PageFetcher.ListSlugs.
func (f *WikiPageFetcher) ListSlugs(ctx context.Context, kbID string) ([]string, error) {
	pages, err := f.repo.ListAll(ctx, kbID)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(pages))
	for _, p := range pages {
		if p == nil {
			continue
		}
		slugs = append(slugs, p.Slug)
	}
	return slugs, nil
}

// ListBySlugs implements PageFetcher.ListBySlugs.
func (f *WikiPageFetcher) ListBySlugs(ctx context.Context, kbID string, slugs []string) (map[string]*PageSummary, error) {
	rows, err := f.repo.ListBySlugs(ctx, kbID, slugs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*PageSummary, len(rows))
	for slug, row := range rows {
		if row == nil {
			continue
		}
		// The lite projection doesn't carry the markdown body, so the
		// scanner's content-based checks degrade gracefully to
		// "unknown" rather than producing false negatives. Build #30
		// will swap the lite read for a full read here.
		out[slug] = &PageSummary{
			Slug:      row.Slug,
			Title:     row.Title,
			Status:    row.Status,
			UpdatedAt: row.UpdatedAt,
		}
	}
	return out, nil
}

func toSummary(p *types.WikiPage) *PageSummary {
	if p == nil {
		return nil
	}
	tenantStr := strconv.FormatUint(p.TenantID, 10)
	return &PageSummary{
		Slug:      p.Slug,
		Title:     p.Title,
		KBID:      p.KnowledgeBaseID,
		TenantID:  tenantStr,
		PageID:    p.ID,
		OutLinks:  []string(p.OutLinks),
		InLinks:   []string(p.InLinks),
		Status:    p.Status,
		UpdatedAt: p.UpdatedAt,
		Content:   p.Content,
	}
}
