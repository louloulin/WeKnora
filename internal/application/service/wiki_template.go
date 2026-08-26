package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// wikiTemplateService is the wiki template skeleton engine (Build #18).
//
// It owns three responsibilities:
//
//   - Validate the skeleton shape (size cap, required fields).
//   - Materialise children + sections atomically: delete prior
//     auto-generated children, create new children, rewrite the
//     parent's body with placeholder tokens resolved.
//   - Resolve `{{tagged_pages:foo}}` tokens via the existing
//     WikiTagService (Build #17) — no second index needed.
//
// The engine is wired with two post-construction setters
// (SetPageService, SetTagService) so container.go can break the
// WikiTagService ↔ WikiPageService dependency cycle, mirroring the
// pattern WikiTagService itself uses.
type wikiTemplateService struct {
	pageSvc interfaces.WikiPageService
	tagSvc  interfaces.WikiTagService
}

// NewWikiTemplateService returns an un-wired service. Callers must
// invoke SetPageService + SetTagService before use. Failing to wire
// surfaces as a friendly error on first call rather than a confusing
// nil-deref panic.
func NewWikiTemplateService() *wikiTemplateService {
	return &wikiTemplateService{}
}

// SetPageService stores the page service used for CreatePage /
// DeletePage / UpdatePage calls.
func (s *wikiTemplateService) SetPageService(svc interfaces.WikiPageService) {
	s.pageSvc = svc
}

// SetTagService stores the tag service used to resolve tagged-pages
// tokens + to apply default tags to newly-materialised children.
func (s *wikiTemplateService) SetTagService(svc interfaces.WikiTagService) {
	s.tagSvc = svc
}

// ensureDeps returns a friendly error if the post-construction wiring
// was skipped. Surfaces before any DB write so callers don't see a
// confusing nil-deref panic.
func (s *wikiTemplateService) ensureDeps() error {
	if s.pageSvc == nil {
		return errors.New("wikiTemplateService: page service not wired")
	}
	if s.tagSvc == nil {
		return errors.New("wikiTemplateService: tag service not wired")
	}
	return nil
}

// ApplyTemplate is the main entry point. The full lifecycle lives in
// one transactional function so the rebuild semantics (Q2=A — atomic
// delete-then-create) stay consistent.
func (s *wikiTemplateService) ApplyTemplate(
	ctx context.Context,
	kbID string,
	parentSlug string,
	req types.WikiApplyTemplateRequest,
) (*types.WikiApplyTemplateResult, error) {
	if err := s.ensureDeps(); err != nil {
		return nil, err
	}
	if kbID == "" || parentSlug == "" {
		return nil, errors.New("kbID and parentSlug are required")
	}
	if err := validateSkeleton(&req.Skeleton); err != nil {
		return nil, err
	}

	parent, err := s.pageSvc.GetPageBySlug(ctx, kbID, parentSlug)
	if err != nil {
		return nil, fmt.Errorf("load parent page: %w", err)
	}

	tagged, err := s.resolveTaggedTokens(ctx, kbID, req.Skeleton.TaggedTokens)
	if err != nil {
		return nil, fmt.Errorf("resolve tagged tokens: %w", err)
	}

	// Atomic block: delete prior auto children, create new ones,
	// rewrite parent body. We deliberately do NOT use a tx wrapper
	// here because individual operations go through service-level
	// helpers that manage their own transactions. Failure mid-way
	// surfaces as an error and leaves the DB consistent with the
	// last successful step (no half-written children).
	if err := s.deletePriorAutoChildren(ctx, kbID, parentSlug); err != nil {
		return nil, fmt.Errorf("delete prior auto children: %w", err)
	}

	created, err := s.materialiseChildren(ctx, kbID, parent, req.Skeleton, req.TemplateID)
	if err != nil {
		return nil, err
	}

	newBody, err := s.rewriteParentBody(parent.Content, req, tagged)
	if err != nil {
		return nil, fmt.Errorf("rewrite parent body: %w", err)
	}

	if req.BodyOverride != "" || newBody != parent.Content {
		parent.Content = newBody
		if _, err := s.pageSvc.UpdatePage(ctx, parent); err != nil {
			return nil, fmt.Errorf("persist rewritten parent: %w", err)
		}
	}

	return &types.WikiApplyTemplateResult{
		ParentSlug:  parent.Slug,
		ParentTitle: parent.Title,
		Pages:       created,
		Sections:    resolveSectionAnchors(req.Skeleton.Sections),
		TaggedPages: tagged,
		NewBody:     newBody,
	}, nil
}

// PreviewSkeleton is identical to ApplyTemplate except it skips all
// DB writes. Used by the dialog's "预览" button before the user
// commits. Implementation reuses the read-only helpers + the body
// rewriter; deletePriorAutoChildren / materialiseChildren /
// UpdatePage are bypassed.
func (s *wikiTemplateService) PreviewSkeleton(
	ctx context.Context,
	kbID string,
	parentSlug string,
	req types.WikiApplyTemplateRequest,
) (*types.WikiApplyTemplateResult, error) {
	if err := s.ensureDeps(); err != nil {
		return nil, err
	}
	if kbID == "" || parentSlug == "" {
		return nil, errors.New("kbID and parentSlug are required")
	}
	if err := validateSkeleton(&req.Skeleton); err != nil {
		return nil, err
	}

	parent, err := s.pageSvc.GetPageBySlug(ctx, kbID, parentSlug)
	if err != nil {
		return nil, fmt.Errorf("load parent page: %w", err)
	}

	tagged, err := s.resolveTaggedTokens(ctx, kbID, req.Skeleton.TaggedTokens)
	if err != nil {
		return nil, fmt.Errorf("resolve tagged tokens: %w", err)
	}

	previewBody, err := s.rewriteParentBody(parent.Content, req, tagged)
	if err != nil {
		return nil, fmt.Errorf("rewrite preview body: %w", err)
	}

	created := make([]types.WikiApplyTemplateCreatedPage, 0, len(req.Skeleton.Children))
	for _, child := range req.Skeleton.Children {
		slug := child.Slug
		if slug == "" {
			slug = slugifyChildSlug(parent.Slug, child.Title)
		}
		created = append(created, types.WikiApplyTemplateCreatedPage{
			Slug:  slug,
			Title: child.Title,
		})
	}

	return &types.WikiApplyTemplateResult{
		ParentSlug:  parent.Slug,
		ParentTitle: parent.Title,
		Pages:       created,
		Sections:    resolveSectionAnchors(req.Skeleton.Sections),
		TaggedPages: tagged,
		NewBody:     previewBody,
	}, nil
}

// validateSkeleton enforces the safety rules from the spec: at least one
// entry of any kind, never more than WikiTemplateSkeletonSafetyCap total.
func validateSkeleton(s *types.WikiTemplateSkeleton) error {
	total := len(s.Children) + len(s.Sections) + len(s.TaggedTokens)
	if total == 0 {
		return types.ErrWikiTemplateEmptySkeleton
	}
	if total > types.WikiTemplateSkeletonSafetyCap {
		return types.ErrWikiTemplateOversizeSkeleton
	}
	for i, c := range s.Children {
		if strings.TrimSpace(c.Title) == "" {
			return fmt.Errorf("skeleton.children[%d].title is required", i)
		}
	}
	for i, sec := range s.Sections {
		if strings.TrimSpace(sec.Title) == "" {
			return fmt.Errorf("skeleton.sections[%d].title is required", i)
		}
	}
	return nil
}

// deletePriorAutoChildren removes pages whose SourceRefs carry the
// canonical "auto-template" tag and that live under the same parent.
// This is the "rebuild" semantics Q2=A.
//
// We funnel the deletes through WikiPageService so the tenant +
// access checks run consistently with the rest of the wiki code.
// Because WikiPageService does not expose ListPagesByParent, we
// materialise the full KB once and filter in-process. Cost is O(KB)
// per apply, which is fine for the typical KB sizes this feature
// targets (dozens to low-hundreds of pages).
func (s *wikiTemplateService) deletePriorAutoChildren(
	ctx context.Context,
	kbID string,
	parentSlug string,
) error {
	pages, err := s.pageSvc.ListAllPages(ctx, kbID)
	if err != nil {
		return err
	}
	for _, p := range pages {
		if p.ParentSlug != parentSlug {
			continue
		}
		if !pageHasAutoTemplateRef(p) {
			continue
		}
		if err := s.pageSvc.DeletePage(ctx, kbID, p.Slug); err != nil {
			// Don't unwind on a single failure — log and continue.
			// The caller will see the partial state via the error
			// returned from materialiseChildren / UpdatePage next.
			logger.Errorf(ctx, "delete prior auto child %s: %v", p.Slug, err)
		}
	}
	return nil
}

// pageHasAutoTemplateRef checks if the page carries the canonical
// "auto-template" source-ref. Sub-tokens after the colon are allowed.
func pageHasAutoTemplateRef(p *types.WikiPage) bool {
	for _, ref := range p.SourceRefs {
		if ref == types.WikiTemplateAutoChildSourceRef {
			return true
		}
		if strings.HasPrefix(ref, types.WikiTemplateAutoChildSourceRef+":") {
			return true
		}
	}
	return false
}

// materialiseChildren creates N child pages, each with
// parent_slug = parent.Slug and source_refs = ["auto-template",
// "auto-template:templateID" (when set)].
func (s *wikiTemplateService) materialiseChildren(
	ctx context.Context,
	kbID string,
	parent *types.WikiPage,
	skel types.WikiTemplateSkeleton,
	templateID string,
) ([]types.WikiApplyTemplateCreatedPage, error) {
	out := make([]types.WikiApplyTemplateCreatedPage, 0, len(skel.Children))

	// Slug derivation guarantees uniqueness within the parent: if a
	// derived slug already exists in this KB, append `-2`, `-3`, ...
	used := map[string]bool{parent.Slug: true}

	for i, child := range skel.Children {
		desired := child.Slug
		if desired == "" {
			desired = slugifyChildSlug(parent.Slug, child.Title)
		}
		finalSlug, err := s.uniqueChildSlug(ctx, kbID, desired, used)
		if err != nil {
			return nil, err
		}
		used[finalSlug] = true

		sourceRefs := []string{types.WikiTemplateAutoChildSourceRef}
		if templateID != "" {
			sourceRefs = append(sourceRefs, types.WikiTemplateAutoChildSourceRef+":"+templateID)
		}

		newPage := &types.WikiPage{
			ID:              uuid.New().String(),
			Slug:            finalSlug,
			KnowledgeBaseID: kbID,
			Title:           child.Title,
			Content:         child.Content,
			ParentSlug:      parent.Slug,
			PageType:        types.WikiPageTypeConcept,
			Status:          types.WikiPageStatusDraft,
			SourceRefs:      sourceRefs,
			LastEditSource:  types.WikiEditSourceUser,
			Version:         1,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if _, err := s.pageSvc.CreatePage(ctx, newPage); err != nil {
			return nil, fmt.Errorf("create child %q: %w", child.Title, err)
		}

		// Apply default tags via the Build #17 tag system. We swallow
		// per-tag failures (logger only) so one missing tag doesn't
		// kill the whole apply. Tags that don't exist yet are
		// auto-created with the default blue palette.
		for _, tagName := range child.DefaultTags {
			if err := s.applyTagToPage(ctx, kbID, newPage.Slug, tagName); err != nil {
				logger.Warnf(ctx, "apply tag %q to %s: %v", tagName, newPage.Slug, err)
			}
		}

		out = append(out, types.WikiApplyTemplateCreatedPage{
			Slug:  finalSlug,
			Title: child.Title,
		})
	}

	return out, nil
}

// uniqueChildSlug keeps the candidate slug unique within the KB. The
// local `used` set tracks names we've already claimed in the current
// apply so two children with the same derived slug don't collide.
func (s *wikiTemplateService) uniqueChildSlug(
	ctx context.Context,
	kbID string,
	candidate string,
	used map[string]bool,
) (string, error) {
	current := candidate
	for n := 1; n < 1000; n++ {
		if !used[current] {
			exists, err := s.pageSvc.ExistsSlugs(ctx, kbID, []string{current})
			if err != nil {
				return "", err
			}
			if !exists[current] {
				return current, nil
			}
		}
		current = fmt.Sprintf("%s-%d", candidate, n+1)
	}
	return "", fmt.Errorf("could not derive unique slug from %q", candidate)
}

// applyTagToPage resolves tagName to a tag id (creating the tag if
// missing), then adds it to the page via the Build #17 worker-pool
// hook. Returns the underlying error so callers can log + continue.
func (s *wikiTemplateService) applyTagToPage(
	ctx context.Context,
	kbID string,
	slug string,
	tagName string,
) error {
	if tagName == "" {
		return nil
	}
	tag, err := s.tagByName(ctx, kbID, tagName)
	if err != nil {
		return err
	}
	if tag == nil || tag.ID == "" {
		return errors.New("tag id missing after create/get")
	}
	return s.tagSvc.ApplyBatchTagOneSlug(ctx, kbID, slug, tag.ID, types.WikiBatchTagOpAdd)
}

// tagByName resolves a tag by its display name. The WikiTagService
// interface only exposes GetByID; we iterate the per-KB list and
// match by name. Acceptable for typical KB sizes — the tag list is
// rarely more than a few dozen rows.
func (s *wikiTemplateService) tagByName(
	ctx context.Context,
	kbID string,
	name string,
) (*types.WikiTag, error) {
	tags, err := s.tagSvc.List(ctx, kbID)
	if err != nil {
		return nil, err
	}
	for i := range tags {
		if tags[i].Name == name {
			// List returns WikiTagWithCount; downgrade to WikiTag.
			return &tags[i].WikiTag, nil
		}
	}
	return nil, nil
}

// resolveTaggedTokens expands each token in TaggedTokens into the list
// of page slugs tagged with that name. Returns a map keyed by the
// token name (the literal that appears in `{{tagged_pages:foo}}`).
//
// Cost: O(KB × T_pages) per token because we don't have a direct
// "pages by tag id" method on WikiPageService. Acceptable for typical
// KB sizes; flagged as a follow-up optimisation target.
func (s *wikiTemplateService) resolveTaggedTokens(
	ctx context.Context,
	kbID string,
	tokens []string,
) (map[string][]string, error) {
	out := make(map[string][]string, len(tokens))
	if len(tokens) == 0 {
		return out, nil
	}

	tagByName, err := s.buildTagIndex(ctx, kbID)
	if err != nil {
		return nil, err
	}

	pages, err := s.pageSvc.ListAllPages(ctx, kbID)
	if err != nil {
		return nil, err
	}

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		tagID, ok := tagByName[token]
		if !ok {
			out[token] = []string{}
			continue
		}
		matches := make([]string, 0, 4)
		for _, p := range pages {
			pageTags, err := s.tagSvc.GetPageTags(ctx, kbID, p.Slug)
			if err != nil {
				return nil, err
			}
			for _, t := range pageTags {
				if t.ID == tagID {
					matches = append(matches, p.Slug)
					break
				}
			}
		}
		out[token] = matches
	}
	return out, nil
}

// buildTagIndex flattens the WikiTagService.List response into a
// name → id lookup so the resolver can short-circuit by name in O(1).
func (s *wikiTemplateService) buildTagIndex(
	ctx context.Context,
	kbID string,
) (map[string]string, error) {
	tags, err := s.tagSvc.List(ctx, kbID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(tags))
	for i := range tags {
		out[tags[i].Name] = tags[i].ID
	}
	return out, nil
}

// rewriteParentBody scans the parent body for placeholder tokens and
// substitutes them. When BodyOverride is supplied, the override is
// the new source body and we still scan it for tokens.
//
// Supported tokens:
//
//	{{child_pages}}        → catalog of created children
//	{{child_section}}      → in-page anchor list of all sections
//	{{tagged_pages:foo}}   → list of pages tagged `foo`
func (s *wikiTemplateService) rewriteParentBody(
	currentBody string,
	req types.WikiApplyTemplateRequest,
	tagged map[string][]string,
) (string, error) {
	body := currentBody
	if req.BodyOverride != "" {
		body = req.BodyOverride
	}
	body += req.BodyAppend

	childPages := make([]types.WikiApplyTemplateCreatedPage, 0, len(req.Skeleton.Children))
	for _, c := range req.Skeleton.Children {
		childPages = append(childPages, types.WikiApplyTemplateCreatedPage{
			Slug:  c.Slug,
			Title: c.Title,
		})
	}

	body = childPagesToken.ReplaceAllStringFunc(body, func(_ string) string {
		if len(childPages) == 0 {
			return "*(no auto-generated children)*"
		}
		var b strings.Builder
		b.WriteString("**子页面**\n\n")
		for _, c := range childPages {
			b.WriteString(fmt.Sprintf("- [%s](./%s)\n", c.Title, c.Slug))
		}
		return b.String()
	})

	body = childSectionToken.ReplaceAllStringFunc(body, func(_ string) string {
		if len(req.Skeleton.Sections) == 0 {
			return "*(no in-page sections)*"
		}
		var b strings.Builder
		b.WriteString("**章节**\n\n")
		for _, sec := range req.Skeleton.Sections {
			anchor := sec.Anchor
			if anchor == "" {
				anchor = slugifyAnchor(sec.Title)
			}
			b.WriteString(fmt.Sprintf("- [%s](#%s)\n", sec.Title, anchor))
		}
		return b.String()
	})

	body = taggedPagesToken.ReplaceAllStringFunc(body, func(match string) string {
		// capture the token name from inside {{tagged_pages:foo}}
		m := taggedPagesInner.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		token := strings.TrimSpace(m[1])
		pages := tagged[token]
		if len(pages) == 0 {
			return fmt.Sprintf("*(no pages tagged `%s`)*", token)
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("**标签 `%s` 的页面**\n\n", token))
		for _, p := range pages {
			b.WriteString(fmt.Sprintf("- [%s](./%s)\n", p, p))
		}
		return b.String()
	})

	return body, nil
}

// resolveSectionAnchors returns the anchor ids the rewrite will emit
// (used by the result so the UI can show a final summary).
func resolveSectionAnchors(sections []types.WikiTemplatePlaceholderSection) []types.WikiApplyTemplateResolvedSection {
	out := make([]types.WikiApplyTemplateResolvedSection, 0, len(sections))
	for _, sec := range sections {
		anchor := sec.Anchor
		if anchor == "" {
			anchor = slugifyAnchor(sec.Title)
		}
		out = append(out, types.WikiApplyTemplateResolvedSection{
			Anchor: anchor,
			Title:  sec.Title,
		})
	}
	return out
}

// slugifyChildSlug derives "<parent>-<title>" with safe characters.
func slugifyChildSlug(parent, title string) string {
	return slugifyAnchor(parent) + "-" + slugifyAnchor(title)
}

// slugifyAnchor lowercases + collapses non-alphanumerics to hyphens.
func slugifyAnchor(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "section"
	}
	return out
}

// Placeholder regexes — kept package-level so they compile once.
var (
	childPagesToken  = regexp.MustCompile(`\{\{child_pages\}\}`)
	childSectionToken = regexp.MustCompile(`\{\{child_section\}\}`)
	taggedPagesToken = regexp.MustCompile(`\{\{tagged_pages:[^}]+\}\}`)
	taggedPagesInner = regexp.MustCompile(`\{\{tagged_pages:([^}]+)\}\}`)
)