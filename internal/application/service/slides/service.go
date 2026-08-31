// Package slides — Build #44 Slide deck application service.
//
// Composes SlideDeckRepository into:
//   - CRUD on decks + slides
//   - AI auto-generate from a source document (header + bullets)
//   - export to PPTX (via the existing pptxgenjs pipeline on the client
//     side; this service emits the JSON the client consumes) and to
//     Markdown / JSON / HTML
package slides

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// SlideService is the application-level Slide surface.
type SlideService struct {
	repo interfaces.SlideDeckRepository
	// audit hook for Build #46 Governance compatibility.
	audit func(ctx context.Context, tenantID uint64, deckID string, userID uint64, action, detail string)
}

// NewSlideService wires the service.
func NewSlideService(repo interfaces.SlideDeckRepository) *SlideService {
	return &SlideService{repo: repo}
}

// SetAuditHook lets the container wire a governance audit emitter.
func (s *SlideService) SetAuditHook(hook func(ctx context.Context, tenantID uint64, deckID string, userID uint64, action, detail string)) {
	s.audit = hook
}

// CreateDeck persists a new deck + the supplied seed slides.
func (s *SlideService) CreateDeck(ctx context.Context, tenantID, userID uint64, req types.CreateSlideDeckRequest) (*types.SlideDeck, error) {
	if req.Title == "" {
		return nil, types.ErrSlideInvalid("title is required")
	}
	if req.Theme == "" {
		req.Theme = types.SlideThemeNotion
	}
	if !types.ValidSlideThemes[req.Theme] {
		return nil, types.ErrSlideInvalid("theme is invalid")
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	d := &types.SlideDeck{
		ID:          newID(),
		TenantID:    tenantID,
		Title:       req.Title,
		Theme:       req.Theme,
		SourceDocID: req.SourceDocID,
		KBID:        req.KBID,
		OwnerUserID: userID,
		Visibility:  req.Visibility,
		SlideCount:  len(req.Slides),
	}
	if err := s.repo.CreateDeck(ctx, d); err != nil {
		return nil, err
	}
	for i, ss := range req.Slides {
		layout := ss.Layout
		if layout == "" {
			layout = types.SlideLayoutBullet
		}
		slide := &types.Slide{
			ID:         newID(),
			DeckID:     d.ID,
			Index:      i,
			Layout:     layout,
			Title:      ss.Title,
			Body:       ss.Body,
			Bullets:    marshalBullets(ss.Bullets),
			LeftCol:    ss.LeftCol,
			RightCol:   ss.RightCol,
			ImageURL:   ss.ImageURL,
			QuoteText:  ss.QuoteText,
			QuoteAttr:  ss.QuoteAttr,
			Notes:      ss.Notes,
			Background: ss.Background,
		}
		if err := s.repo.CreateSlide(ctx, slide); err != nil {
			logger.Warnf(ctx, "[Slides] seed slide %d failed: %v", i, err)
		}
	}
	s.emitAudit(ctx, tenantID, d.ID, userID, "create", fmt.Sprintf("title=%s theme=%s slides=%d", d.Title, d.Theme, len(req.Slides)))
	return d, nil
}

// GetDeck fetches a deck.
func (s *SlideService) GetDeck(ctx context.Context, tenantID uint64, id string) (*types.SlideDeck, error) {
	return s.repo.GetDeck(ctx, tenantID, id)
}

// UpdateDeck applies a partial patch.
func (s *SlideService) UpdateDeck(ctx context.Context, tenantID, userID uint64, id string, patch types.UpdateSlideDeckRequest) (*types.SlideDeck, error) {
	out, err := s.repo.UpdateDeck(ctx, tenantID, id, patch)
	if err != nil {
		return nil, err
	}
	if out != nil {
		s.emitAudit(ctx, tenantID, id, userID, "update", "metadata")
	}
	return out, nil
}

// DeleteDeck removes the deck + slides (transactional).
func (s *SlideService) DeleteDeck(ctx context.Context, tenantID, userID uint64, id string) error {
	s.emitAudit(ctx, tenantID, id, userID, "delete", "")
	return s.repo.DeleteDeck(ctx, tenantID, id)
}

// ListDecks lists decks with filters.
func (s *SlideService) ListDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) ([]*types.SlideDeck, error) {
	return s.repo.ListDecks(ctx, tenantID, filter)
}

// CountDecks returns the count for the same filters.
func (s *SlideService) CountDecks(ctx context.Context, tenantID uint64, filter types.ListSlideDecksFilter) (int64, error) {
	return s.repo.CountDecks(ctx, tenantID, filter)
}

// ListSlides returns every slide in a deck.
func (s *SlideService) ListSlides(ctx context.Context, tenantID uint64, deckID string) ([]*types.Slide, error) {
	return s.repo.ListSlidesByDeck(ctx, tenantID, deckID)
}

// CreateSlide adds a slide to a deck.
func (s *SlideService) CreateSlide(ctx context.Context, tenantID, userID uint64, deckID string, req types.CreateSlideRequest) (*types.Slide, error) {
	d, err := s.repo.GetDeck(ctx, tenantID, deckID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, types.ErrSlideInvalid("slide deck not found")
	}
	layout := req.Layout
	if layout == "" {
		layout = types.SlideLayoutBullet
	}
	if !types.ValidSlideLayouts[layout] {
		return nil, types.ErrSlideInvalid("layout is invalid")
	}
	// Append at the end.
	existing, _ := s.repo.ListSlidesByDeck(ctx, tenantID, deckID)
	slide := &types.Slide{
		ID:         newID(),
		DeckID:     deckID,
		Index:      len(existing),
		Layout:     layout,
		Title:      req.Title,
		Body:       req.Body,
		Bullets:    marshalBullets(req.Bullets),
		LeftCol:    req.LeftCol,
		RightCol:   req.RightCol,
		ImageURL:   req.ImageURL,
		QuoteText:  req.QuoteText,
		QuoteAttr:  req.QuoteAttr,
		Notes:      req.Notes,
		Background: req.Background,
	}
	if err := s.repo.CreateSlide(ctx, slide); err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantID, deckID, userID, "slide_create", fmt.Sprintf("slide_id=%s layout=%s", slide.ID, slide.Layout))
	return slide, nil
}

// UpdateSlide applies a partial patch.
func (s *SlideService) UpdateSlide(ctx context.Context, tenantID, userID uint64, deckID, slideID string, patch types.UpdateSlideRequest) (*types.Slide, error) {
	out, err := s.repo.UpdateSlide(ctx, tenantID, deckID, slideID, patch)
	if err != nil {
		return nil, err
	}
	if out != nil {
		s.emitAudit(ctx, tenantID, deckID, userID, "slide_update", fmt.Sprintf("slide_id=%s", slideID))
	}
	return out, nil
}

// DeleteSlide removes a single slide.
func (s *SlideService) DeleteSlide(ctx context.Context, tenantID, userID uint64, deckID, slideID string) error {
	s.emitAudit(ctx, tenantID, deckID, userID, "slide_delete", fmt.Sprintf("slide_id=%s", slideID))
	return s.repo.DeleteSlide(ctx, tenantID, deckID, slideID)
}

// AutoGenerateFromDoc builds a deck by analyzing a source document. The
// source is fetched as Markdown (via the existing KB retrieval surface)
// and split into sections. Each top-level heading + its first paragraph
// becomes a "bullet" slide; the first heading is promoted to a "title"
// slide. Caller supplies the doc's markdown via the SourceDocID hook —
// for offline tests we accept the title + markdown directly.
func (s *SlideService) AutoGenerateFromDoc(ctx context.Context, tenantID, userID uint64, sourceDocID, kbID, title string, theme types.SlideTheme, markdown string, maxSlides int) (*types.SlideDeck, error) {
	if markdown == "" {
		return nil, types.ErrSlideInvalid("markdown is required")
	}
	if theme == "" {
		theme = types.SlideThemeNotion
	}
	if !types.ValidSlideThemes[theme] {
		return nil, types.ErrSlideInvalid("theme is invalid")
	}
	if maxSlides <= 0 || maxSlides > 200 {
		maxSlides = 20
	}
	if title == "" {
		title = "Auto-generated deck"
	}
	sections := splitMarkdownSections(markdown)
	if len(sections) == 0 {
		return nil, types.ErrSlideInvalid("no sections extracted from markdown")
	}
	if len(sections) > maxSlides-1 { // -1 to leave room for the title slide
		sections = sections[:maxSlides-1]
	}
	slides := []types.CreateSlideRequest{
		{Layout: types.SlideLayoutTitle, Title: title},
	}
	for _, sec := range sections {
		bullets := strings.Split(sec.body, "\n")
		out := make([]string, 0, len(bullets))
		for _, b := range bullets {
			t := strings.TrimSpace(b)
			if t != "" {
				out = append(out, t)
			}
		}
		slides = append(slides, types.CreateSlideRequest{
			Layout:  types.SlideLayoutBullet,
			Title:   sec.heading,
			Body:    sec.body,
			Bullets: out,
		})
	}
	d, err := s.CreateDeck(ctx, tenantID, userID, types.CreateSlideDeckRequest{
		Title:       title,
		Theme:       theme,
		SourceDocID: sourceDocID,
		KBID:        kbID,
		Visibility:  "private",
		Slides:      slides,
	})
	if err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantID, d.ID, userID, "auto_generate", fmt.Sprintf("source=%s slides=%d", sourceDocID, len(slides)))
	return d, nil
}

// ExportMarkdown renders a Markdown outline of the deck.
func (s *SlideService) ExportMarkdown(ctx context.Context, tenantID uint64, deckID string) (string, error) {
	d, slides, err := s.loadFull(ctx, tenantID, deckID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(d.Title)
	b.WriteString("\n\n")
	for i, s := range slides {
		fmt.Fprintf(&b, "## Slide %d: %s\n\n", i+1, s.Title)
		if s.Body != "" {
			b.WriteString(s.Body)
			b.WriteString("\n\n")
		}
		if s.Bullets != "" && s.Bullets != "[]" {
			b.WriteString(s.Bullets)
			b.WriteString("\n\n")
		}
	}
	return b.String(), nil
}

// ExportJSON emits the structured deck payload (consumed by the
// Vue SlidesEditor + pptxgenjs renderer on the client side).
func (s *SlideService) ExportJSON(ctx context.Context, tenantID uint64, deckID string) (string, error) {
	d, slides, err := s.loadFull(ctx, tenantID, deckID)
	if err != nil {
		return "", err
	}
	out := struct {
		Deck   *types.SlideDeck  `json:"deck"`
		Slides []*exportSlideRow `json:"slides"`
	}{Deck: d}
	for _, s := range slides {
		out.Slides = append(out.Slides, &exportSlideRow{
			ID:         s.ID,
			Index:      s.Index,
			Layout:     string(s.Layout),
			Title:      s.Title,
			Body:       s.Body,
			Bullets:    unmarshalBullets(s.Bullets),
			LeftCol:    s.LeftCol,
			RightCol:   s.RightCol,
			ImageURL:   s.ImageURL,
			QuoteText:  s.QuoteText,
			QuoteAttr:  s.QuoteAttr,
			Notes:      s.Notes,
			Background: s.Background,
		})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type exportSlideRow struct {
	ID         string   `json:"id"`
	Index      int      `json:"index"`
	Layout     string   `json:"layout"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Bullets    []string `json:"bullets"`
	LeftCol    string   `json:"left_col"`
	RightCol   string   `json:"right_col"`
	ImageURL   string   `json:"image_url"`
	QuoteText  string   `json:"quote_text"`
	QuoteAttr  string   `json:"quote_attr"`
	Notes      string   `json:"notes"`
	Background string   `json:"background"`
}

// loadFull returns the deck + its slides.
func (s *SlideService) loadFull(ctx context.Context, tenantID uint64, deckID string) (*types.SlideDeck, []*types.Slide, error) {
	d, err := s.repo.GetDeck(ctx, tenantID, deckID)
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, types.ErrSlideInvalid("slide deck not found")
	}
	slides, err := s.repo.ListSlidesByDeck(ctx, tenantID, deckID)
	if err != nil {
		return nil, nil, err
	}
	return d, slides, nil
}

// emitAudit writes an audit event when the hook is set.
func (s *SlideService) emitAudit(ctx context.Context, tenantID uint64, deckID string, userID uint64, action, detail string) {
	if s.audit == nil {
		return
	}
	s.audit(ctx, tenantID, deckID, userID, action, detail)
}

// mdSection is a markdown section split by H2 headings.
type mdSection struct {
	heading string
	body    string
}

// splitMarkdownSections extracts sections from a Markdown body.
// Each "## Heading" starts a new section; everything until the next
// "## " belongs to it.
func splitMarkdownSections(md string) []mdSection {
	out := []mdSection{}
	current := mdSection{}
	lines := strings.Split(md, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			if current.heading != "" || current.body != "" {
				out = append(out, current)
			}
			current = mdSection{heading: strings.TrimPrefix(trim, "## ")}
			continue
		}
		if strings.HasPrefix(trim, "# ") {
			// H1 acts as the document title — skip (already in deck title).
			continue
		}
		current.body += line + "\n"
	}
	if current.heading != "" || current.body != "" {
		out = append(out, current)
	}
	return out
}

// marshalBullets encodes []string to JSON.
func marshalBullets(items []string) string {
	if items == nil {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}

// unmarshalBullets decodes the JSON array.
func unmarshalBullets(s string) []string {
	if s == "" {
		return []string{}
	}
	out := []string{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// newID returns a 32-char hex ID.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
