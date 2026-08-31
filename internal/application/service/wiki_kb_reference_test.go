//go:build wikikbtest
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeWikiKBReferenceRepo is a tiny in-memory implementation of
// WikiKBReferenceRepository used only by the unit tests below.
// Production code uses the GORM-backed implementation; the fake
// keeps the test free of database dependencies so it can run as
// part of the fast `go test ./internal/...` loop.
type fakeWikiKBReferenceRepo struct {
	rows  map[string]*types.WikiKBReference
	clock time.Time
}

func newWikiKBReferenceFakeRepo() *fakeWikiKBReferenceRepo {
	return &fakeWikiKBReferenceRepo{
		rows:  map[string]*types.WikiKBReference{},
		clock: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
	}
}

func pairKey(tenantID, wikiPageID, knowledgeID string) string {
	return tenantID + "|" + wikiPageID + "|" + knowledgeID
}

func (f *fakeWikiKBReferenceRepo) Upsert(_ context.Context, ref *types.WikiKBReference) error {
	now := f.clock
	ref.UpdatedAt = now
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = now
	}
	key := pairKey(ref.TenantID, ref.WikiPageID, ref.KnowledgeID)
	if existing, ok := f.rows[key]; ok {
		existing.ReferenceLabel = ref.ReferenceLabel
		existing.UpdatedAt = now
		f.rows[key] = existing
		return nil
	}
	f.rows[key] = ref
	return nil
}

func (f *fakeWikiKBReferenceRepo) GetByPair(_ context.Context, tenantID, wikiPageID, knowledgeID string) (*types.WikiKBReference, error) {
	if row, ok := f.rows[pairKey(tenantID, wikiPageID, knowledgeID)]; ok {
		return row, nil
	}
	return nil, interfaces.ErrWikiKBReferenceNotFound
}

func (f *fakeWikiKBReferenceRepo) ListByWikiPage(_ context.Context, tenantID, wikiPageID string, _, _ int) ([]*types.WikiKBReference, error) {
	var out []*types.WikiKBReference
	for _, row := range f.rows {
		if row.TenantID == tenantID && row.WikiPageID == wikiPageID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeWikiKBReferenceRepo) ListByKnowledge(_ context.Context, tenantID, knowledgeID string, _, _ int) ([]*types.WikiKBReference, error) {
	var out []*types.WikiKBReference
	for _, row := range f.rows {
		if row.TenantID == tenantID && row.KnowledgeID == knowledgeID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeWikiKBReferenceRepo) SoftDelete(_ context.Context, tenantID, wikiPageID, knowledgeID string) error {
	key := pairKey(tenantID, wikiPageID, knowledgeID)
	if _, ok := f.rows[key]; !ok {
		return interfaces.ErrWikiKBReferenceNotFound
	}
	delete(f.rows, key)
	return nil
}

// fakeWikiPageRepo returns a canned page; deleted toggles the
// soft-delete sentinel so we can exercise the resolution status enum.
type fakeWikiPageRepo struct {
	title   string
	deleted bool
}

func (f *fakeWikiPageRepo) GetByID(_ context.Context, _ string) (*types.WikiPage, error) {
	page := &types.WikiPage{Title: f.title}
	if f.deleted {
		page.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	}
	return page, nil
}

type fakeKnowledgeRepo struct {
	title    string
	fileName string
	deleted  bool
}

func (f *fakeKnowledgeRepo) GetKnowledgeByIDOnly(_ context.Context, _ string) (*types.Knowledge, error) {
	kb := &types.Knowledge{Title: f.title, FileName: f.fileName}
	if f.deleted {
		kb.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	}
	return kb, nil
}

// TestKnowledgeReferenceService_AddReference_Upserts covers the happy
// path of the doc+KB bridge: a single (page, kb) pair is added twice
// and the second call only updates the label / timestamp.
func TestKnowledgeReferenceService_AddReference_Upserts(t *testing.T) {
	repo := newWikiKBReferenceFakeRepo()
	svc := service.NewKnowledgeReferenceService(repo, &fakeWikiPageRepo{}, &fakeKnowledgeRepo{}, nil)

	first, err := svc.AddReference(context.Background(),
		"tenant-1", "page-1", "kb-1", "Foo release notes", "user-1")
	if err != nil {
		t.Fatalf("AddReference returned error: %v", err)
	}
	if first.ReferenceLabel != "Foo release notes" {
		t.Fatalf("expected reference label 'Foo release notes', got %q", first.ReferenceLabel)
	}

	second, err := svc.AddReference(context.Background(),
		"tenant-1", "page-1", "kb-1", "Bar release notes", "user-1")
	if err != nil {
		t.Fatalf("second AddReference returned error: %v", err)
	}
	if second.ReferenceLabel != "Bar release notes" {
		t.Fatalf("expected reference label to be refreshed, got %q", second.ReferenceLabel)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("expected exactly one row after upsert, got %d", len(repo.rows))
	}
}

// TestKnowledgeReferenceService_RemoveReference_Idempotent asserts
// that removing a non-existent pair does not raise — that path is
// used by the wiki page delete cascade where the row may already
// be gone.
func TestKnowledgeReferenceService_RemoveReference_Idempotent(t *testing.T) {
	repo := newWikiKBReferenceFakeRepo()
	svc := service.NewKnowledgeReferenceService(repo, &fakeWikiPageRepo{}, &fakeKnowledgeRepo{}, nil)

	if err := svc.RemoveReference(context.Background(), "tenant-1", "missing", "kb-1"); err != nil {
		t.Fatalf("expected idempotent remove to succeed, got %v", err)
	}
}

// TestKnowledgeReferenceService_Resolve_StatusEnum walks the four
// ReferenceStatus states (active, wiki_deleted, kb_deleted,
// both_deleted) so we know the status enum composition rule holds.
func TestKnowledgeReferenceService_Resolve_StatusEnum(t *testing.T) {
	cases := []struct {
		name     string
		wikiDel  bool
		kbDel    bool
		expected types.ReferenceStatus
	}{
		{"active", false, false, types.ReferenceStatusActive},
		{"wiki_deleted", true, false, types.ReferenceStatusWikiDeleted},
		{"kb_deleted", false, true, types.ReferenceStatusKBDeleted},
		{"both_deleted", true, true, types.ReferenceStatusBothDeleted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newWikiKBReferenceFakeRepo()
			page := &fakeWikiPageRepo{title: "Wiki Page", deleted: tc.wikiDel}
			kb := &fakeKnowledgeRepo{title: "KB Doc", fileName: "kb-doc.md", deleted: tc.kbDel}
			svc := service.NewKnowledgeReferenceService(repo, page, kb, nil)

			repo.rows["tenant-1|page-1|kb-1"] = &types.WikiKBReference{
				TenantID:    "tenant-1",
				WikiPageID:  "page-1",
				KnowledgeID: "kb-1",
			}

			resolved, err := svc.ResolveReference(context.Background(),
				"tenant-1", "page-1", "kb-1")
			if err != nil {
				t.Fatalf("ResolveReference: %v", err)
			}
			if resolved.Status != tc.expected {
				t.Fatalf("expected status %q, got %q", tc.expected, resolved.Status)
			}
			if resolved.WikiTitle != "Wiki Page" {
				t.Fatalf("expected wiki title to be decorated, got %q", resolved.WikiTitle)
			}
			if resolved.KBTitle != "KB Doc" {
				t.Fatalf("expected KB title to be decorated, got %q", resolved.KBTitle)
			}
		})
	}
}

// TestKnowledgeReferenceService_Resolve_NotFound verifies the
// service-layer sentinel is wired to ErrKnowledgeReferenceNotFound
// (and not the repository error) so the handler can map a single
// switch.
func TestKnowledgeReferenceService_Resolve_NotFound(t *testing.T) {
	repo := newWikiKBReferenceFakeRepo()
	svc := service.NewKnowledgeReferenceService(repo, &fakeWikiPageRepo{}, &fakeKnowledgeRepo{}, nil)

	_, err := svc.ResolveReference(context.Background(), "tenant-1", "missing", "kb-1")
	if !errors.Is(err, service.ErrKnowledgeReferenceNotFound) {
		t.Fatalf("expected ErrKnowledgeReferenceNotFound, got %v", err)
	}
}

// TestKnowledgeReferenceService_AddReference_ValidatesInputs guards
// against empty IDs making it to the repository layer where they
// would crash the unique index.
func TestKnowledgeReferenceService_AddReference_ValidatesInputs(t *testing.T) {
	svc := service.NewKnowledgeReferenceService(newWikiKBReferenceFakeRepo(),
		&fakeWikiPageRepo{}, &fakeKnowledgeRepo{}, nil)

	_, err := svc.AddReference(context.Background(), "", "page", "kb", "label", "user")
	if err == nil {
		t.Fatalf("expected empty tenantID to be rejected")
	}
}
