package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// Build #16 — PreviewBatchMove / PreviewBatchDelete / PreviewBatchStatus
// harness coverage. The tests share stubBatchRepo with wiki_page_batch_test.go
// (same package) and exercise only the read-only code path the preview
// implements: GetBySlug, GetFolderByID (via applyFolderToPage), and
// assertBatchKBOwnership.

func TestPreviewBatchMove_AllSucceed_NoFolderChange(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchMove(context.Background(), "kb1", []string{"a", "b"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.Total != 2 || got.Summary.WillSucceed != 2 || got.Summary.WillFail != 0 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
	if len(got.Success) != 2 || len(got.Failed) != 0 {
		t.Fatalf("unexpected body: success=%v failed=%v", got.Success, got.Failed)
	}
}

func TestPreviewBatchMove_FolderMissingClassifiesAsFolderNotFound(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchMove(context.Background(), "kb1", []string{"a", "b"}, "no-such-folder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 0 || got.Summary.WillFail != 2 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
	for _, f := range got.Failed {
		if f.Code != "folder_not_found" {
			t.Errorf("slug=%s code=%q, want folder_not_found", f.Slug, f.Code)
		}
	}
}

func TestPreviewBatchMove_PartialFailure(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchMove(context.Background(), "kb1", []string{"a", "missing", "b"}, "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 2 || got.Summary.WillFail != 1 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
	if got.Failed[0].Slug != "missing" || got.Failed[0].Code != "not_found" {
		t.Fatalf("failed entry wrong: %+v", got.Failed[0])
	}
	// Preview must not mutate.
	if repo.pages["a"].FolderID != "" {
		t.Errorf("page a FolderID was mutated: %q", repo.pages["a"].FolderID)
	}
}

func TestPreviewBatchMove_DedupesAndTrims(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchMove(context.Background(), "kb1", []string{" a ", "a", "", "  "}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.Total != 1 || got.Summary.WillSucceed != 1 {
		t.Fatalf("dedup broken: %+v", got.Summary)
	}
}

func TestPreviewBatchMove_CrossKBRejectsRequestLevel(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPageInKB("kb1", "a", types.WikiPageStatusPublished, nil, "")
	repo.addPageInKB("kb2", "foreign", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	_, err := svc.PreviewBatchMove(context.Background(), "kb1", []string{"a", "foreign"}, "")
	if err == nil {
		t.Fatalf("expected kb_mismatch error, got nil")
	}
	if !types.IsWikiBatchKBMismatch(err) {
		t.Fatalf("err not WikiBatchKBMismatchError: %v", err)
	}
}

func TestPreviewBatchDelete_AllSucceed(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusDraft, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchDelete(context.Background(), "kb1", []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 2 || got.Summary.WillFail != 0 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
	// Preview must not delete.
	if _, ok := repo.pages["a"]; !ok {
		t.Errorf("page a was deleted by preview")
	}
}

func TestPreviewBatchDelete_NotFound(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchDelete(context.Background(), "kb1", []string{"a", "missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 1 || got.Summary.WillFail != 1 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
	if got.Failed[0].Code != "not_found" {
		t.Fatalf("failed code=%q", got.Failed[0].Code)
	}
}

func TestPreviewBatchStatus_AllSucceed(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusDraft, nil, "")
	repo.addPage("b", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchStatus(context.Background(), "kb1", []string{"a", "b"}, "archived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 2 || got.Summary.WillFail != 0 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
}

func TestPreviewBatchStatus_RejectsInvalidStatus(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	_, err := svc.PreviewBatchStatus(context.Background(), "kb1", []string{"a"}, "drafty")
	if err == nil {
		t.Fatalf("expected invalid status error")
	}
}

func TestPreviewBatchStatus_NotFound(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchStatus(context.Background(), "kb1", []string{"a", "missing"}, "draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 1 || got.Summary.WillFail != 1 {
		t.Fatalf("bad summary: %+v", got.Summary)
	}
}

func TestPreviewBatchStatus_AlreadyAtTarget(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPage("a", types.WikiPageStatusPublished, nil, "")
	repo.addPage("b", types.WikiPageStatusDraft, nil, "")
	svc := &wikiPageService{repo: repo}

	got, err := svc.PreviewBatchStatus(context.Background(), "kb1", []string{"a", "b"}, "published")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Summary.WillSucceed != 2 || got.Summary.WillFail != 0 {
		t.Fatalf("already-at-target must count as success: %+v", got.Summary)
	}
}

func TestPreviewBatchDelete_CrossKBRejectsRequestLevel(t *testing.T) {
	repo := newStubBatchRepo()
	repo.addPageInKB("kb1", "a", types.WikiPageStatusPublished, nil, "")
	repo.addPageInKB("kb2", "foreign", types.WikiPageStatusPublished, nil, "")
	svc := &wikiPageService{repo: repo}

	_, err := svc.PreviewBatchDelete(context.Background(), "kb1", []string{"a", "foreign"})
	if err == nil {
		t.Fatalf("expected kb_mismatch error, got nil")
	}
	if !types.IsWikiBatchKBMismatch(err) {
		t.Fatalf("err not WikiBatchKBMismatchError: %v", err)
	}
}