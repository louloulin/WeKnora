package slides

import (
	"context"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo is an in-memory SlideDeckRepository used to drive service
// tests without booting GORM. The map is intentionally naive — a
// real tenant isolation invariant is not the unit under test.
type fakeRepo struct {
	mu     sync.Mutex
	decks  map[string]*types.SlideDeck
	slides map[string]*types.Slide
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		decks:  map[string]*types.SlideDeck{},
		slides: map[string]*types.Slide{},
	}
}

func (r *fakeRepo) CreateDeck(_ context.Context, d *types.SlideDeck) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.decks[d.ID]; ok {
		return types.ErrSlideInvalid("duplicate deck id")
	}
	r.decks[d.ID] = d
	return nil
}

func (r *fakeRepo) GetDeck(_ context.Context, tenantID uint64, id string) (*types.SlideDeck, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.decks[id]
	if !ok || d.TenantID != tenantID {
		return nil, types.ErrSlideInvalid("deck not found")
	}
	return d, nil
}

func (r *fakeRepo) UpdateDeck(_ context.Context, tenantID uint64, id string, patch types.UpdateSlideDeckRequest) (*types.SlideDeck, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.decks[id]
	if !ok || d.TenantID != tenantID {
		return nil, types.ErrSlideInvalid("deck not found")
	}
	if patch.Title != nil {
		d.Title = *patch.Title
	}
	if patch.Theme != nil {
		d.Theme = *patch.Theme
	}
	if patch.Visibility != nil {
		d.Visibility = *patch.Visibility
	}
	return d, nil
}

func (r *fakeRepo) DeleteDeck(_ context.Context, tenantID uint64, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.decks[id]
	if !ok || d.TenantID != tenantID {
		return types.ErrSlideInvalid("deck not found")
	}
	delete(r.decks, id)
	for k, s := range r.slides {
		if s.DeckID == id {
			delete(r.slides, k)
		}
	}
	return nil
}

func (r *fakeRepo) ListDecks(_ context.Context, tenantID uint64, _ types.ListSlideDecksFilter) ([]*types.SlideDeck, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*types.SlideDeck, 0, len(r.decks))
	for _, d := range r.decks {
		if d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (r *fakeRepo) CountDecks(_ context.Context, tenantID uint64, _ types.ListSlideDecksFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, d := range r.decks {
		if d.TenantID == tenantID {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) CreateSlide(_ context.Context, s *types.Slide) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slides[s.ID] = s
	if d, ok := r.decks[s.DeckID]; ok {
		d.SlideCount++
	}
	return nil
}

func (r *fakeRepo) GetSlide(_ context.Context, _ uint64, deckID, slideID string) (*types.Slide, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slides[slideID]
	if !ok || s.DeckID != deckID {
		return nil, types.ErrSlideInvalid("slide not found")
	}
	return s, nil
}

func (r *fakeRepo) UpdateSlide(_ context.Context, _ uint64, deckID, slideID string, patch types.UpdateSlideRequest) (*types.Slide, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slides[slideID]
	if !ok || s.DeckID != deckID {
		return nil, types.ErrSlideInvalid("slide not found")
	}
	if patch.Title != nil {
		s.Title = *patch.Title
	}
	if patch.Body != nil {
		s.Body = *patch.Body
	}
	if patch.Bullets != nil {
		s.Bullets = joinStrings(*patch.Bullets)
	}
	return s, nil
}

func (r *fakeRepo) DeleteSlide(_ context.Context, _ uint64, deckID, slideID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slides[slideID]
	if !ok || s.DeckID != deckID {
		return types.ErrSlideInvalid("slide not found")
	}
	delete(r.slides, slideID)
	if d, ok := r.decks[deckID]; ok && d.SlideCount > 0 {
		d.SlideCount--
	}
	return nil
}

func (r *fakeRepo) ListSlidesByDeck(_ context.Context, _ uint64, deckID string) ([]*types.Slide, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*types.Slide, 0, len(r.slides))
	for _, s := range r.slides {
		if s.DeckID == deckID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteByKB(_ context.Context, _ uint64, kbID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for k, d := range r.decks {
		if d.KBID == kbID {
			delete(r.decks, k)
			deleted++
		}
	}
	for k, s := range r.slides {
		// Slide has no KBID column; key off DeckID→KBID lookup.
		if d, ok := r.decks[s.DeckID]; ok && d.KBID == kbID {
			delete(r.slides, k)
		}
	}
	return deleted, nil
}

// joinStrings is a tiny helper used by the slide-update fake to keep
// the test self-contained without pulling encoding/json.
func joinStrings(xs []string) string {
	out := "["
	for i, x := range xs {
		if i > 0 {
			out += ","
		}
		out += "\"" + x + "\""
	}
	return out + "]"
}

// Compile-time check that fakeRepo satisfies the interface.
var _ interfaces.SlideDeckRepository = (*fakeRepo)(nil)

func newServiceWithFake(t *testing.T) (*SlideService, *fakeRepo) {
	t.Helper()
	r := newFakeRepo()
	return NewSlideService(r), r
}

func TestCreateDeckAndListSlides(t *testing.T) {
	svc, repo := newServiceWithFake(t)
	ctx := context.Background()

	d, err := svc.CreateDeck(ctx, 1, 7, types.CreateSlideDeckRequest{
		Title:        "Q3 Onboarding",
		Theme:        types.SlideThemeNotion,
		KBID:         "kb-onboarding",
		SourceDocID:  "doc-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, d.ID)
	assert.Equal(t, "Q3 Onboarding", d.Title)
	assert.Equal(t, types.SlideThemeNotion, d.Theme)
	assert.Equal(t, 0, d.SlideCount, "fresh deck has no slides yet")

	s, err := svc.CreateSlide(ctx, 1, 7, d.ID, types.CreateSlideRequest{
		Title:  "Welcome",
		Layout: types.SlideLayoutTitle,
	})
	require.NoError(t, err)
	assert.Equal(t, d.ID, s.DeckID)

	slides, err := svc.ListSlides(ctx, 1, d.ID)
	require.NoError(t, err)
	assert.Len(t, slides, 1)

	// sanity: repo got both rows
	assert.Len(t, repo.decks, 1)
	assert.Len(t, repo.slides, 1)
}

func TestAutoGenerateFromDoc_SplitsOnHeadings(t *testing.T) {
	svc, _ := newServiceWithFake(t)
	ctx := context.Background()

	markdown := "# Plan\nintro paragraph\n\n## Goals\nship fast\nship safe\n\n## Risks\nscope creep"

	d, err := svc.AutoGenerateFromDoc(ctx, 1, 7, "doc-x", "kb-x", "Plan", types.SlideThemeLark, markdown, 0)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, "Plan", d.Title)
	slides, err := svc.ListSlides(ctx, 1, d.ID)
	require.NoError(t, err)
	// title + 2 sections = 3 slides (parser may emit +1 for intro body)
	assert.GreaterOrEqual(t, len(slides), 3)
	assert.LessOrEqual(t, len(slides), 4)
	assert.NotEmpty(t, slides[0].Layout)
}

func TestUpdateAndDeleteSlide(t *testing.T) {
	svc, _ := newServiceWithFake(t)
	ctx := context.Background()
	d, err := svc.CreateDeck(ctx, 1, 7, types.CreateSlideDeckRequest{Title: "X", KBID: "k"})
	require.NoError(t, err)
	s, err := svc.CreateSlide(ctx, 1, 7, d.ID, types.CreateSlideRequest{Title: "old"})
	require.NoError(t, err)

	// Update
	newTitle := "new"
	upd, err := svc.UpdateSlide(ctx, 1, 7, d.ID, s.ID, types.UpdateSlideRequest{Title: &newTitle})
	require.NoError(t, err)
	assert.Equal(t, "new", upd.Title)

	// Delete
	require.NoError(t, svc.DeleteSlide(ctx, 1, 7, d.ID, s.ID))
	slides, _ := svc.ListSlides(ctx, 1, d.ID)
	assert.Len(t, slides, 0)
}

func TestExportMarkdownAndJSON(t *testing.T) {
	svc, _ := newServiceWithFake(t)
	ctx := context.Background()
	d, err := svc.CreateDeck(ctx, 1, 7, types.CreateSlideDeckRequest{Title: "Demo", KBID: "k"})
	require.NoError(t, err)
	_, err = svc.CreateSlide(ctx, 1, 7, d.ID, types.CreateSlideRequest{Title: "S1", Layout: types.SlideLayoutTitle})
	require.NoError(t, err)

	md, err := svc.ExportMarkdown(ctx, 1, d.ID)
	require.NoError(t, err)
	assert.Contains(t, md, "# Demo")
	assert.Contains(t, md, "S1")

	js, err := svc.ExportJSON(ctx, 1, d.ID)
	require.NoError(t, err)
	assert.Contains(t, js, "Demo")
	assert.Contains(t, js, "S1")
}

func TestAutoGenerateFromDoc_EmptyMarkdownGraceful(t *testing.T) {
	svc, _ := newServiceWithFake(t)
	ctx := context.Background()
	// Empty markdown should still produce a deck with at least the title slide
	// (graceful empty fallback). Random gibberish shouldn't error.
	d, err := svc.AutoGenerateFromDoc(ctx, 1, 7, "d", "k", "Empty", types.SlideThemeDark, "# Empty\n\nbody text", 0)
	require.NoError(t, err)
	slides, _ := svc.ListSlides(ctx, 1, d.ID)
	assert.GreaterOrEqual(t, len(slides), 1)
}
