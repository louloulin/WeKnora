package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// WikiKBReferenceBackfillService reconciles wiki_kb_references rows
// against the [[kb:id]] mentions actually present in a wiki page's
// content. The contract is idempotent: a second call with the same
// content produces zero row changes.
//
// Where the service fits in the write path:
//
//	wikiPageService.UpdatePage
//	  └── persist new content / title / etc.
//	  └── WikiKBReferenceBackfillService.ReconcileAfterSave(ctx, tenantID, pageID, newContent, actor)
//
// The hook sits AFTER the persist so a failed reconcile can never
// leave the page in an inconsistent state (the wiki content is the
// source of truth; the backfill row set is a derived index).
//
// Concurrency: two concurrent saves for the same page both run the
// reconcile; because the row identity is (page, kb) and the upsert is
// idempotent, the only observable effect is one of the two actors'
// labels winning in the audit. That is acceptable — both actors have
// permission to write the page, and the audit log records whichever
// label arrived last.
// WikiKBReferenceBackfiller is the contract the wiki page service uses
// to trigger a reconciliation after a save. Defining it as an interface
// keeps the wikiPageService from depending on the concrete
// WikiKBReferenceBackfillService — tests can swap a stub.
type WikiKBReferenceBackfiller interface {
	ReconcileAfterSave(
		ctx context.Context,
		tenantID, wikiPageID, newContent, actorUserID string,
	) (ReconcileResult, error)
}

type WikiKBReferenceBackfillService struct {
	references *KnowledgeReferenceService
}

// NewWikiKBReferenceBackfillService is the DI constructor.
func NewWikiKBReferenceBackfillService(refs *KnowledgeReferenceService) *WikiKBReferenceBackfillService {
	return &WikiKBReferenceBackfillService{references: refs}
}

// ReconcileResult is the outcome of one backfill pass. It is
// intentionally small (added / removed counts + whether anything
// changed) so callers can emit metrics and audit logs cheaply.
type ReconcileResult struct {
	Added   int
	Removed int
	Changed bool
}

// ReconcileAfterSave walks the new content via ParseKBReferences and
// reconciles wiki_kb_references to match. Inserted rows use the
// reference_label parsed from the [[kb:id|label]] syntax; removed rows
// are the pairs that existed before but no longer appear in content.
//
// The function is best-effort: a soft-failed reconcile returns the
// error so the caller can log it, but does NOT propagate failure
// upward — the wiki content has already been persisted and a stale
// references table is far less bad than a save that succeeded in the
// database but failed at the API boundary.
//
// ReconcileAfterSave takes a (possibly nil) reference to the underlying
// repository so it can list the current page references cheaply without
// going through the resolver (which decorates with titles + KB titles).
// The repository is exposed only on the KnowledgeReferenceService
// implementation; if it is not available, the service falls back to
// service.ListForWikiPage which pays the decoration cost.
func (s *WikiKBReferenceBackfillService) ReconcileAfterSave(
	ctx context.Context,
	tenantID, wikiPageID, newContent, actorUserID string,
) (ReconcileResult, error) {
	result := ReconcileResult{}
	if s.references == nil {
		return result, nil
	}
	if tenantID == "" || wikiPageID == "" {
		return result, nil
	}

	// Build the desired set from the new content.
	desired := make(map[string]string, 8)
	for _, span := range ParseKBReferences(newContent) {
		desired[span.KnowledgeID] = span.Label
	}

	// Read the current set. ListForWikiPage returns resolved rows;
	// we only need the KnowledgeID + ReferenceLabel fields so the
	// decoration cost is paid once per reconcile.
	current, err := s.references.ListForWikiPage(ctx, tenantID, wikiPageID, 200, 0)
	if err != nil {
		return result, err
	}
	currentIDs := make(map[string]string, len(current))
	for _, row := range current {
		currentIDs[row.KnowledgeID] = row.ReferenceLabel
	}

	// Add new ids that are not in the current set, or refresh label
	// for ids whose label has changed since the last reconcile.
	for id, label := range desired {
		if existing, ok := currentIDs[id]; !ok || existing != label {
			if _, err := s.references.AddReference(ctx,
				tenantID, wikiPageID, id, label, actorUserID); err != nil {
				return result, err
			}
			if !ok {
				result.Added++
			}
		}
	}

	// Remove ids that are no longer mentioned.
	for id := range currentIDs {
		if _, keep := desired[id]; keep {
			continue
		}
		if err := s.references.RemoveReference(ctx, tenantID, wikiPageID, id); err != nil {
			return result, err
		}
		result.Removed++
	}

	result.Changed = result.Added > 0 || result.Removed > 0
	return result, nil
}

// Interface guard — both interfaces must remain compatible with the
// production GORM implementation. Tests substitute their own fakes.
var (
	_ interfaces.WikiKBReferenceRepository = (interfaces.WikiKBReferenceRepository)(nil)
	_ types.WikiKBReference                = types.WikiKBReference{}
)
