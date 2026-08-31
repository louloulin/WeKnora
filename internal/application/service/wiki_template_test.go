package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// stubWikiTemplatePageSvc is a minimal in-memory implementation of
// interfaces.WikiPageService used by the template-engine harness. The
// struct embeds the production interface as a nil field so it
// satisfies the full type without the test having to declare all
// 50+ methods — only the ones ApplyTemplate / PreviewSkeleton
// actually call are overridden below; any non-overridden call would
// nil-deref and surface as a clear panic. That's intentional: the
// test should fail loudly if a future change makes ApplyTemplate
// start using a new method.
type stubWikiTemplatePageSvc struct {
	interfaces.WikiPageService
	pages        map[string]*types.WikiPage
	createdSlugs []string
	deletedSlugs []string
}

func newStubTemplatePageSvc() *stubWikiTemplatePageSvc {
	return &stubWikiTemplatePageSvc{
		pages: map[string]*types.WikiPage{},
	}
}

func (s *stubWikiTemplatePageSvc) seed(kbID, slug string, p *types.WikiPage) {
	if p == nil {
		p = &types.WikiPage{}
	}
	p.KnowledgeBaseID = kbID
	p.Slug = slug
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Title == "" {
		p.Title = slug
	}
	if p.Content == "" {
		p.Content = "initial body"
	}
	if p.Version == 0 {
		p.Version = 1
	}
	if len(p.SourceRefs) == 0 {
		p.SourceRefs = types.StringArray{}
	}
	s.pages[kbID+"|"+slug] = p
}

func (s *stubWikiTemplatePageSvc) GetPageBySlug(_ context.Context, kbID, slug string) (*types.WikiPage, error) {
	p, ok := s.pages[kbID+"|"+slug]
	if !ok {
		return nil, errors.New("not found")
	}
	// Return a copy so callers don't accidentally mutate our map.
	copy := *p
	if copy.SourceRefs == nil {
		copy.SourceRefs = types.StringArray{}
	}
	return &copy, nil
}

func (s *stubWikiTemplatePageSvc) ListAllPages(_ context.Context, kbID string) ([]*types.WikiPage, error) {
	out := []*types.WikiPage{}
	for k, p := range s.pages {
		if len(kbID) > 0 && (len(k) < len(kbID) || k[:len(kbID)] != kbID) {
			continue
		}
		copy := *p
		if copy.SourceRefs == nil {
			copy.SourceRefs = types.StringArray{}
		}
		out = append(out, &copy)
	}
	return out, nil
}

func (s *stubWikiTemplatePageSvc) DeletePage(_ context.Context, kbID, slug string) error {
	delete(s.pages, kbID+"|"+slug)
	s.deletedSlugs = append(s.deletedSlugs, slug)
	return nil
}

func (s *stubWikiTemplatePageSvc) CreatePage(_ context.Context, p *types.WikiPage) (*types.WikiPage, error) {
	if p == nil || p.Slug == "" {
		return nil, errors.New("missing slug")
	}
	if _, exists := s.pages[p.KnowledgeBaseID+"|"+p.Slug]; exists {
		return nil, errors.New("slug exists")
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	s.pages[p.KnowledgeBaseID+"|"+p.Slug] = p
	s.createdSlugs = append(s.createdSlugs, p.Slug)
	return p, nil
}

func (s *stubWikiTemplatePageSvc) UpdatePage(_ context.Context, p *types.WikiPage) (*types.WikiPage, error) {
	if p == nil || p.Slug == "" {
		return nil, errors.New("missing slug")
	}
	s.pages[p.KnowledgeBaseID+"|"+p.Slug] = p
	return p, nil
}

func (s *stubWikiTemplatePageSvc) ExistsSlugs(_ context.Context, kbID string, slugs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		_, exists := s.pages[kbID+"|"+slug]
		out[slug] = exists
	}
	return out, nil
}

// stubWikiTemplateTagSvc is a minimal in-memory implementation of
// interfaces.WikiTagService used by the template-engine harness.
// Same embed-the-interface trick as the page stub.
type stubWikiTemplateTagSvc struct {
	interfaces.WikiTagService
	tags     map[string]types.WikiTag
	pageTags map[string]map[string]struct{} // slug -> tagID set
	addCalls int
}

func newStubTemplateTagSvc() *stubWikiTemplateTagSvc {
	return &stubWikiTemplateTagSvc{
		tags:     map[string]types.WikiTag{},
		pageTags: map[string]map[string]struct{}{},
	}
}

func (s *stubWikiTemplateTagSvc) seedTag(id, name, color string) {
	s.tags[id] = types.WikiTag{ID: id, Name: name, Color: color}
}

func (s *stubWikiTemplateTagSvc) seedPageTag(slug, tagID string) {
	if s.pageTags[slug] == nil {
		s.pageTags[slug] = map[string]struct{}{}
	}
	s.pageTags[slug][tagID] = struct{}{}
}

func (s *stubWikiTemplateTagSvc) List(_ context.Context, _ string) ([]types.WikiTagWithCount, error) {
	out := make([]types.WikiTagWithCount, 0, len(s.tags))
	for _, t := range s.tags {
		out = append(out, types.WikiTagWithCount{WikiTag: t, PageCount: 0})
	}
	return out, nil
}

func (s *stubWikiTemplateTagSvc) ApplyBatchTagOneSlug(_ context.Context, _ string, slug, tagID, op string) error {
	if op != types.WikiBatchTagOpAdd {
		return errors.New("only add supported in stub")
	}
	if _, ok := s.tags[tagID]; !ok {
		return errors.New("tag not found: " + tagID)
	}
	if s.pageTags[slug] == nil {
		s.pageTags[slug] = map[string]struct{}{}
	}
	s.pageTags[slug][tagID] = struct{}{}
	s.addCalls++
	return nil
}

func (s *stubWikiTemplateTagSvc) GetPageTags(_ context.Context, _ string, slug string) ([]types.WikiTag, error) {
	set := s.pageTags[slug]
	out := []types.WikiTag{}
	for tagID := range set {
		if t, ok := s.tags[tagID]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// buildTemplateService wires the two stubs into a fresh service.
// Going through the production setters (which take the full
// interfaces) exercises the same wiring code path the container uses.
func buildTemplateService(pageSvc *stubWikiTemplatePageSvc, tagSvc *stubWikiTemplateTagSvc) interfaces.WikiTemplateService {
	s := NewWikiTemplateService()
	s.SetPageService(pageSvc)
	s.SetTagService(tagSvc)
	return s
}

// --- Tests -------------------------------------------------------------

func TestWikiTemplate_ApplyTemplate_Children(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent-page"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{
		Title:   "Parent",
		Content: "intro\n\n{{child_pages}}\n",
	})

	req := types.WikiApplyTemplateRequest{
		TemplateID: "tpl-meeting",
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "Agenda", Content: "agenda body"},
				{Title: "Notes", Content: ""},
				{Title: "Action Items", DefaultTags: []string{"todo"}},
			},
		},
	}
	result, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req)
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	if len(result.Pages) != 3 {
		t.Fatalf("want 3 created pages, got %d", len(result.Pages))
	}
	for _, c := range result.Pages {
		if c.Slug == "" {
			t.Errorf("created page missing slug: %+v", c)
		}
	}
	if len(pageSvc.createdSlugs) != 3 {
		t.Errorf("pageSvc.createdSlugs = %d, want 3", len(pageSvc.createdSlugs))
	}
}

func TestWikiTemplate_ApplyTemplate_AtomicDeletePrior(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{Title: "Parent"})

	// Seed two prior auto-template children + one manual child that
	// must survive the rebuild.
	pageSvc.seed(kbID, "parent-old-1", &types.WikiPage{
		Title:      "Old-1",
		ParentSlug: parentSlug,
		SourceRefs: types.StringArray{types.WikiTemplateAutoChildSourceRef},
	})
	pageSvc.seed(kbID, "parent-old-2", &types.WikiPage{
		Title:      "Old-2",
		ParentSlug: parentSlug,
		SourceRefs: types.StringArray{types.WikiTemplateAutoChildSourceRef + ":legacy"},
	})
	pageSvc.seed(kbID, "parent-manual", &types.WikiPage{
		Title:      "Manual",
		ParentSlug: parentSlug,
		SourceRefs: types.StringArray{"user-authored"},
	})

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "Fresh"},
			},
		},
	}
	if _, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	// Old-1 + Old-2 should be deleted; Manual + Fresh should remain.
	if _, ok := pageSvc.pages[kbID+"|parent-old-1"]; ok {
		t.Errorf("old-1 should have been deleted")
	}
	if _, ok := pageSvc.pages[kbID+"|parent-old-2"]; ok {
		t.Errorf("old-2 should have been deleted")
	}
	if _, ok := pageSvc.pages[kbID+"|parent-manual"]; !ok {
		t.Errorf("manual child should NOT have been deleted")
	}
	if _, ok := pageSvc.pages[kbID+"|parent-fresh"]; !ok {
		t.Errorf("fresh child should have been created")
	}
}

func TestWikiTemplate_ApplyTemplate_SlugUniqueness(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{Title: "Parent"})
	// Pre-existing page forces the apply path to use the -2 suffix.
	pageSvc.seed(kbID, "parent-section", &types.WikiPage{Title: "Existing"})

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "Section"},
			},
		},
	}
	result, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req)
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	if got := result.Pages[0].Slug; got != "parent-section-2" {
		t.Errorf("expected unique slug `parent-section-2`, got %q", got)
	}
}

func TestWikiTemplate_ApplyTemplate_EmptySkeleton(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{Title: "Parent"})

	_, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, types.WikiApplyTemplateRequest{})
	if !errors.Is(err, types.ErrWikiTemplateEmptySkeleton) {
		t.Fatalf("expected ErrWikiTemplateEmptySkeleton, got %v", err)
	}
}

func TestWikiTemplate_ApplyTemplate_OversizeSkeleton(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{Title: "Parent"})

	// 201 children exceeds the 200 cap.
	children := make([]types.WikiTemplatePlaceholderChild, 201)
	for i := range children {
		children[i] = types.WikiTemplatePlaceholderChild{Title: "X"}
	}
	_, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{Children: children},
	})
	if !errors.Is(err, types.ErrWikiTemplateOversizeSkeleton) {
		t.Fatalf("expected ErrWikiTemplateOversizeSkeleton, got %v", err)
	}
}

func TestWikiTemplate_ApplyTemplate_RewritesParentBody(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{
		Title:   "Parent",
		Content: "intro\n\n{{child_pages}}\n\n## 章节\n{{child_section}}\n",
	})

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "First"},
				{Title: "Second"},
			},
			Sections: []types.WikiTemplatePlaceholderSection{
				{Anchor: "agenda", Title: "Agenda", Body: "agenda body"},
				{Anchor: "notes", Title: "Notes"},
			},
		},
	}
	if _, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	updated := pageSvc.pages[kbID+"|parent"]
	if !wikiTemplateContains(updated.Content, "First") || !wikiTemplateContains(updated.Content, "Second") {
		t.Errorf("body rewrite should embed child titles; got %q", updated.Content)
	}
	if !wikiTemplateContains(updated.Content, "#agenda") || !wikiTemplateContains(updated.Content, "#notes") {
		t.Errorf("body rewrite should embed section anchors; got %q", updated.Content)
	}
}

func TestWikiTemplate_ApplyTemplate_AppliesDefaultTags(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	// Pre-create the "todo" tag so the apply path can resolve it.
	tagSvc.seedTag("todo-id", "todo", "blue")

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{Title: "Parent"})

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "TaggedChild", DefaultTags: []string{"todo"}},
			},
		},
	}
	if _, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	if _, ok := tagSvc.pageTags["parent-taggedchild"]["todo-id"]; !ok {
		t.Errorf("expected todo-id applied to parent-taggedchild, got %+v", tagSvc.pageTags)
	}
}

func TestWikiTemplate_PreviewSkeleton_NoWrites(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{
		Title:   "Parent",
		Content: "intro\n\n{{child_pages}}\n",
	})

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{
				{Title: "PreviewChild"},
			},
		},
	}
	result, err := svc.PreviewSkeleton(context.Background(), kbID, parentSlug, req)
	if err != nil {
		t.Fatalf("PreviewSkeleton: %v", err)
	}
	if len(result.Pages) != 1 {
		t.Errorf("expected 1 preview page, got %d", len(result.Pages))
	}
	if len(pageSvc.createdSlugs) != 0 {
		t.Errorf("PreviewSkeleton should not create any pages; got %d", len(pageSvc.createdSlugs))
	}
	// Parent body must be unchanged.
	if pageSvc.pages[kbID+"|parent"].Content != "intro\n\n{{child_pages}}\n" {
		t.Errorf("PreviewSkeleton should not mutate parent body")
	}
}

func TestWikiTemplate_ApplyTemplate_ResolvesTaggedPages(t *testing.T) {
	pageSvc := newStubTemplatePageSvc()
	tagSvc := newStubTemplateTagSvc()
	svc := buildTemplateService(pageSvc, tagSvc)

	tagSvc.seedTag("todo-id", "todo", "blue")
	const kbID = "kb-test"
	const parentSlug = "parent"
	pageSvc.seed(kbID, parentSlug, &types.WikiPage{
		Title:   "Parent",
		Content: "{{tagged_pages:todo}}",
	})
	// Pages tagged with `todo` should appear in the rewritten body.
	pageSvc.seed(kbID, "todo-page-1", &types.WikiPage{Title: "TODO 1"})
	tagSvc.seedPageTag("todo-page-1", "todo-id")
	pageSvc.seed(kbID, "todo-page-2", &types.WikiPage{Title: "TODO 2"})
	tagSvc.seedPageTag("todo-page-2", "todo-id")

	req := types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children:     []types.WikiTemplatePlaceholderChild{{Title: "Any"}},
			TaggedTokens: []string{"todo"},
		},
	}
	if _, err := svc.ApplyTemplate(context.Background(), kbID, parentSlug, req); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	body := pageSvc.pages[kbID+"|parent"].Content
	if !wikiTemplateContains(body, "todo-page-1") || !wikiTemplateContains(body, "todo-page-2") {
		t.Errorf("tagged-pages body rewrite should embed tagged slugs; got %q", body)
	}
}

func TestWikiTemplate_ApplyTemplate_RejectsUnwired(t *testing.T) {
	// Confirm the post-construction setters are honoured: a service
	// that was never wired must surface a friendly error.
	svc := NewWikiTemplateService()
	_, err := svc.ApplyTemplate(context.Background(), "kb", "slug", types.WikiApplyTemplateRequest{
		Skeleton: types.WikiTemplateSkeleton{
			Children: []types.WikiTemplatePlaceholderChild{{Title: "X"}},
		},
	})
	if err == nil {
		t.Fatalf("expected error from unwired service, got nil")
	}
}

// wikiTemplateContains is a tiny helper to keep test bodies readable.
func wikiTemplateContains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}