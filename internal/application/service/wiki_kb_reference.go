package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// KnowledgeReferenceService is the doc + KB integration glue. It binds
// the Wiki page lifecycle (draft / publish / soft-delete) to the KB
// document lifecycle (parse / embed / soft-delete) without coupling
// either side directly to the other.
//
// Public methods are organised around four intent verbs that map cleanly
// onto the REST surface:
//
//	AddReference      — POST   /api/v1/wiki/pages/:id/references/kb
//	RemoveReference   — DELETE /api/v1/wiki/pages/:id/references/kb/:kbId
//	ListForWikiPage   — GET    /api/v1/wiki/pages/:id/references/kb
//	ListForKnowledge  — GET    /api/v1/knowledge/:id/references/wiki
//	ResolveReference  — GET    /api/v1/wiki/pages/:id/references/kb/:kbId
//
// Concurrency: AddReference is idempotent at the (page, kb) pair level,
// so two simultaneous back-fills from the same wiki content produce one
// row, not a duplicate.
type KnowledgeReferenceService struct {
	repo      interfaces.WikiKBReferenceRepository
	wikiPage  interfaces.WikiPageRepository
	knowledge interfaces.KnowledgeRepository
	kbBase    interfaces.KnowledgeBaseRepository
}

// NewKnowledgeReferenceService wires the service into the container. The
// wikiPage / knowledge / kbBase dependencies are needed for resolution
// — without them we cannot decorate the reference row with titles.
func NewKnowledgeReferenceService(
	repo interfaces.WikiKBReferenceRepository,
	wikiPage interfaces.WikiPageRepository,
	knowledge interfaces.KnowledgeRepository,
	kbBase interfaces.KnowledgeBaseRepository,
) *KnowledgeReferenceService {
	return &KnowledgeReferenceService{
		repo:      repo,
		wikiPage:  wikiPage,
		knowledge: knowledge,
		kbBase:    kbBase,
	}
}

// KnowledgeReferenceNotFoundError is the service-layer sentinel the
// handler maps to HTTP 404. We re-export the repository error so the
// handler only has to switch on one set of sentinels.
var ErrKnowledgeReferenceNotFound = errors.New("knowledge reference not found")

// AddReference upserts the (wiki_page_id, knowledge_id) pair. The
// referenceLabel is the human-readable text the author typed between
// the [[kb:id]] delimiters; we keep it for forensic audit even if the
// underlying KB document title later changes.
//
// Authorization checks are deliberately not done here — the handler
// already verifies wiki-page write access. The service trusts that the
// caller has the right to bind references on this page.
func (s *KnowledgeReferenceService) AddReference(
	ctx context.Context,
	tenantID, wikiPageID, knowledgeID, referenceLabel, actorUserID string,
) (*types.WikiKBReference, error) {
	if tenantID == "" || wikiPageID == "" || knowledgeID == "" {
		return nil, errors.New("tenantID, wikiPageID and knowledgeID are required")
	}
	// Defensive normalisation: trim whitespace and cap the label so a
	// runaway author cannot blow past the VARCHAR(256) storage budget.
	label := strings.TrimSpace(referenceLabel)
	if len(label) > 256 {
		label = label[:256]
	}
	ref := &types.WikiKBReference{
		TenantID:       tenantID,
		WikiPageID:     wikiPageID,
		KnowledgeID:    knowledgeID,
		ReferenceLabel: label,
		CreatedBy:      actorUserID,
	}
	if err := s.repo.Upsert(ctx, ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// RemoveReference soft-deletes the row. Idempotent: if the pair was
// already gone, we treat it as success and return nil.
func (s *KnowledgeReferenceService) RemoveReference(
	ctx context.Context, tenantID, wikiPageID, knowledgeID string,
) error {
	err := s.repo.SoftDelete(ctx, tenantID, wikiPageID, knowledgeID)
	if err != nil && !errors.Is(err, interfaces.ErrWikiKBReferenceNotFound) {
		return err
	}
	return nil
}

// ListForWikiPage returns every live reference on the page, decorated
// with the KB title and a small content snippet so the UI can render
// the card without a follow-up fetch.
func (s *KnowledgeReferenceService) ListForWikiPage(
	ctx context.Context, tenantID, wikiPageID string, limit, offset int,
) ([]*types.ResolvedWikiKBReference, error) {
	rows, err := s.repo.ListByWikiPage(ctx, tenantID, wikiPageID, limit, offset)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(ctx, tenantID, rows)
}

// ListForKnowledge is the inverse view: "which wiki pages mention this
// KB document?". This is what the KB document viewer shows in its
// "Mentioned in Wiki Pages" sidebar.
func (s *KnowledgeReferenceService) ListForKnowledge(
	ctx context.Context, tenantID, knowledgeID string, limit, offset int,
) ([]*types.ResolvedWikiKBReference, error) {
	rows, err := s.repo.ListByKnowledge(ctx, tenantID, knowledgeID, limit, offset)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(ctx, tenantID, rows)
}

// ResolveReference returns a single resolved reference by the (page,
// kb) pair. The handler uses this for the GET-by-id endpoint and for
// rendering an inline [[kb:id]] mention in a wiki page.
func (s *KnowledgeReferenceService) ResolveReference(
	ctx context.Context, tenantID, wikiPageID, knowledgeID string,
) (*types.ResolvedWikiKBReference, error) {
	row, err := s.repo.GetByPair(ctx, tenantID, wikiPageID, knowledgeID)
	if err != nil {
		if errors.Is(err, interfaces.ErrWikiKBReferenceNotFound) {
			return nil, ErrKnowledgeReferenceNotFound
		}
		return nil, err
	}
	resolved, err := s.resolveAll(ctx, tenantID, []*types.WikiKBReference{row})
	if err != nil {
		return nil, err
	}
	if len(resolved) == 0 {
		return nil, ErrKnowledgeReferenceNotFound
	}
	return resolved[0], nil
}

// resolveAll decorates each reference row with its wiki / KB titles and
// the resolution status. Tombstoned endpoints are still returned so the
// UI can render a "deleted" badge without losing the link audit.
//
// The function is best-effort: a missing wiki page or KB document does
// not abort the whole list; it just sets the corresponding status flag.
func (s *KnowledgeReferenceService) resolveAll(
	ctx context.Context, tenantID string, rows []*types.WikiKBReference,
) ([]*types.ResolvedWikiKBReference, error) {
	out := make([]*types.ResolvedWikiKBReference, 0, len(rows))
	for _, row := range rows {
		r := &types.ResolvedWikiKBReference{
			WikiKBReference: *row,
			Status:          types.ReferenceStatusActive,
		}
		if s.wikiPage != nil {
			if page, err := s.wikiPage.GetByID(ctx, row.WikiPageID); err == nil {
				if page != nil {
					if page.DeletedAt.Valid {
						r.Status = bumpWikiDeleted(r.Status)
					}
					r.WikiTitle = page.Title
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		if s.knowledge != nil {
			if kb, err := s.knowledge.GetKnowledgeByIDOnly(ctx, row.KnowledgeID); err == nil {
				if kb != nil {
					if kb.DeletedAt.Valid {
						r.Status = bumpKBDeleted(r.Status)
					}
					r.KBTitle = kb.Title
					r.KBFileName = kb.FileName
					r.KBSnippet = firstNonEmpty(kb.Description, kb.FileName)
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// bumpWikiDeleted / bumpKBDeleted compose the status enum so that
// "wiki deleted AND kb deleted" → ReferenceStatusBothDeleted.
func bumpWikiDeleted(s types.ReferenceStatus) types.ReferenceStatus {
	switch s {
	case types.ReferenceStatusActive:
		return types.ReferenceStatusWikiDeleted
	case types.ReferenceStatusKBDeleted:
		return types.ReferenceStatusBothDeleted
	}
	return s
}

func bumpKBDeleted(s types.ReferenceStatus) types.ReferenceStatus {
	switch s {
	case types.ReferenceStatusActive:
		return types.ReferenceStatusKBDeleted
	case types.ReferenceStatusWikiDeleted:
		return types.ReferenceStatusBothDeleted
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
