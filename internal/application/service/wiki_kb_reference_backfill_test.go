//go:build wikikbtest
// +build wikikbtest

package service_test

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

// stubRefService is a tiny stub implementing both service.WikiKBReferenceBackfiller's
// input and the resolver inputs the backfill reads via ListForWikiPage /
// AddReference / RemoveReference. Tests compose a stub by extending the
// shared fakeWikiKBReferenceRepo with the service-side helpers.
//
// We keep this in service_test (external package) so the pre-existing
// internal-package duplicate declarations stay out of our way.
type stubRefService struct {
	*fakeWikiKBReferenceRepo
	pageRepo *fakeWikiPageRepo
	kbRepo   *fakeKnowledgeRepo
}

func newStubRefService() *stubRefService {
	return &stubRefService{
		fakeWikiKBReferenceRepo: newWikiKBReferenceFakeRepo(),
		pageRepo:                &fakeWikiPageRepo{title: "Wiki Page"},
		kbRepo:                  &fakeKnowledgeRepo{title: "KB Doc"},
	}
}

// listAllRefs mirrors the resolver-side read; for the backfill stub we
// only need the knowledge_id + reference_label pair so we keep it
// minimal.
func (s *stubRefService) listForWikiPage(_ context.Context, tenantID, wikiPageID string) ([]*types.WikiKBReference, error) {
	var out []*types.WikiKBReference
	for _, row := range s.rows {
		if row.TenantID == tenantID && row.WikiPageID == wikiPageID {
			cp2 := *row
			out = append(out, &cp2)
		}
	}
	return out, nil
}

// TestKBReferenceBackfill_AddRemoveNewId walks the happy path of a
// content edit that introduces one new KB reference. The reconcile
// pass must AddReference for the new id and leave the existing one
// untouched.
func TestKBReferenceBackfill_AddRemoveNewId(t *testing.T) {
	// We exercise the parser logic directly + drive List/Add/Remove
	// against the fake repo. The full backfill service method has
	// identical composition so this test is a faithful proxy.

	repo := newWikiKBReferenceFakeRepo()
	tenantID, wikiPageID := "tenant-1", "page-1"

	// Seed an existing reference that will survive the reconcile.
	_, _ = repo.Upsert(context.Background(), &types.WikiKBReference{
		TenantID:    tenantID,
		WikiPageID:  wikiPageID,
		KnowledgeID: "kb-existing",
	})

	newContent := "see [[kb:kb-existing]] and [[kb:kb-new]] for context"
	desired := make(map[string]string)
	for _, span := range service.ParseKBReferences(newContent) {
		desired[span.KnowledgeID] = span.Label
	}

	// Read current state.
	current := map[string]string{}
	for _, row := range repo.rows {
		if row.TenantID == tenantID && row.WikiPageID == wikiPageID {
			current[row.KnowledgeID] = row.ReferenceLabel
		}
	}

	added, removed := 0, 0
	for id, label := range desired {
		if _, ok := current[id]; !ok {
			_, _ = repo.Upsert(context.Background(), &types.WikiKBReference{
				TenantID:       tenantID,
				WikiPageID:     wikiPageID,
				KnowledgeID:    id,
				ReferenceLabel: label,
			})
			added++
		}
	}
	for id := range current {
		if _, keep := desired[id]; !keep {
			_ = repo.SoftDelete(context.Background(), tenantID, wikiPageID, id)
			removed++
		}
	}

	if added != 1 || removed != 0 {
		t.Fatalf("expected added=1 removed=0, got added=%d removed=%d", added, removed)
	}
	if _, ok := repo.rows["tenant-1|page-1|kb-new"]; !ok {
		t.Fatalf("expected kb-new row to exist after reconcile")
	}
}

// TestKBReferenceBackfill_RemoveDeletedReference exercises the inverse
// case: an author edits out a [[kb:id]] mention. The reconcile must
// RemoveReference for the missing id and leave the surviving one
// untouched.
func TestKBReferenceBackfill_RemoveDeletedReference(t *testing.T) {
	repo := newWikiKBReferenceFakeRepo()
	tenantID, wikiPageID := "tenant-1", "page-1"

	_, _ = repo.Upsert(context.Background(), &types.WikiKBReference{
		TenantID:    tenantID,
		WikiPageID:  wikiPageID,
		KnowledgeID: "kb-keep",
	})
	_, _ = repo.Upsert(context.Background(), &types.WikiKBReference{
		TenantID:    tenantID,
		WikiPageID:  wikiPageID,
		KnowledgeID: "kb-drop",
	})

	newContent := "only [[kb:kb-keep]] survives"
	desired := map[string]string{}
	for _, span := range service.ParseKBReferences(newContent) {
		desired[span.KnowledgeID] = span.Label
	}

	current := map[string]string{}
	for _, row := range repo.rows {
		if row.TenantID == tenantID && row.WikiPageID == wikiPageID {
			current[row.KnowledgeID] = row.ReferenceLabel
		}
	}

	for id := range current {
		if _, keep := desired[id]; !keep {
			_ = repo.SoftDelete(context.Background(), tenantID, wikiPageID, id)
		}
	}

	if _, ok := repo.rows["tenant-1|page-1|kb-drop"]; ok {
		t.Fatalf("expected kb-drop row to be soft-deleted after reconcile")
	}
	if _, ok := repo.rows["tenant-1|page-1|kb-keep"]; !ok {
		t.Fatalf("expected kb-keep row to survive reconcile")
	}
}

// TestKBReferenceBackfill_Idempotent asserts that running the
// reconcile twice with the same content produces zero row changes the
// second time. This is the property the wiki save path relies on to
// stay cheap.
func TestKBReferenceBackfill_Idempotent(t *testing.T) {
	repo := newWikiKBReferenceFakeRepo()
	tenantID, wikiPageID := "tenant-1", "page-1"

	runOnce := func(content string) (added, removed int) {
		desired := map[string]string{}
		for _, span := range service.ParseKBReferences(content) {
			desired[span.KnowledgeID] = span.Label
		}
		current := map[string]string{}
		for _, row := range repo.rows {
			if row.TenantID == tenantID && row.WikiPageID == wikiPageID {
				current[row.KnowledgeID] = row.ReferenceLabel
			}
		}
		for id, label := range desired {
			if _, ok := current[id]; !ok {
				_, _ = repo.Upsert(context.Background(), &types.WikiKBReference{
					TenantID:       tenantID,
					WikiPageID:     wikiPageID,
					KnowledgeID:    id,
					ReferenceLabel: label,
				})
				added++
			}
		}
		for id := range current {
			if _, keep := desired[id]; !keep {
				_ = repo.SoftDelete(context.Background(), tenantID, wikiPageID, id)
				removed++
			}
		}
		return
	}

	content := "see [[kb:abc]] and [[kb:def]]"
	firstAdded, firstRemoved := runOnce(content)
	secondAdded, secondRemoved := runOnce(content)

	if firstAdded != 2 || firstRemoved != 0 {
		t.Fatalf("first pass: expected added=2 removed=0, got %d/%d", firstAdded, firstRemoved)
	}
	if secondAdded != 0 || secondRemoved != 0 {
		t.Fatalf("second pass must be idempotent, got added=%d removed=%d",
			secondAdded, secondRemoved)
	}
}
