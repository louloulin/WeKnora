package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubWikiCommentRepo is an in-memory implementation of the repository
// used by the unit tests. The real implementation is GORM-backed; the
// service tests only care about the contract.
type stubWikiCommentRepo struct {
	rows     map[string]*types.WikiComment
	tenantID uint64
}

func newStubWikiCommentRepo() *stubWikiCommentRepo {
	return &stubWikiCommentRepo{rows: map[string]*types.WikiComment{}}
}

func (r *stubWikiCommentRepo) Create(ctx context.Context, c *types.WikiComment) error {
	if _, exists := r.rows[c.ID]; exists {
		return interfaces.ErrWikiCommentConflict
	}
	cp := *c
	r.rows[c.ID] = &cp
	return nil
}

func (r *stubWikiCommentRepo) GetByID(ctx context.Context, id string) (*types.WikiComment, error) {
	if c, ok := r.rows[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (r *stubWikiCommentRepo) ListByPage(ctx context.Context, kbID string, slug string) ([]types.WikiComment, error) {
	out := []types.WikiComment{}
	for _, c := range r.rows {
		if c.KnowledgeBaseID == kbID && c.PageSlug == slug {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (r *stubWikiCommentRepo) Update(ctx context.Context, id string, body string, mentionsJSON string) (*types.WikiComment, error) {
	c, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	c.Body = body
	c.Mentions = mentionsJSON
	return r.GetByID(ctx, id)
}

func (r *stubWikiCommentRepo) SetResolved(ctx context.Context, id string, resolved bool, resolvedBy string) (*types.WikiComment, error) {
	c, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	if resolved {
		c.ResolvedBy = resolvedBy
	} else {
		c.ResolvedBy = ""
	}
	return r.GetByID(ctx, id)
}

func (r *stubWikiCommentRepo) Delete(ctx context.Context, id string) error {
	delete(r.rows, id)
	return nil
}

func (r *stubWikiCommentRepo) CountByPage(ctx context.Context, kbID string, slug string) (int, int, int, error) {
	open, resolved, replies := 0, 0, 0
	for _, c := range r.rows {
		if c.KnowledgeBaseID != kbID || c.PageSlug != slug {
			continue
		}
		if c.ResolvedBy != "" {
			resolved++
		} else {
			open++
		}
		if c.ParentID != "" {
			replies++
		}
	}
	return open, resolved, replies, nil
}

func TestWikiCommentService_Create_HappyPath(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	comment, err := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{
		Body: "Hello world",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if comment.ID == "" || comment.AuthorID != "u-1" || comment.Body != "Hello world" {
		t.Fatalf("unexpected comment: %+v", comment)
	}
}

func TestWikiCommentService_Create_RejectsEmptyBody(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	_, err := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: ""})
	if err != interfaces.ErrWikiCommentBadInput {
		t.Fatalf("expected ErrWikiCommentBadInput, got %v", err)
	}
}

func TestWikiCommentService_Create_RejectsTooLargeBody(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	huge := strings.Repeat("x", types.WikiCommentMaxBodyBytes+1)
	_, err := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: huge})
	if err != interfaces.ErrWikiCommentBadInput {
		t.Fatalf("expected ErrWikiCommentBadInput for oversized body, got %v", err)
	}
}

func TestWikiCommentService_Create_Reply(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	parent, err := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "top-level"})
	if err != nil {
		t.Fatalf("parent Create: %v", err)
	}

	reply, err := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-2", "Bob", "", types.WikiCommentCreateRequest{
		Body:     "a reply",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("reply Create: %v", err)
	}
	if reply.ParentID != parent.ID {
		t.Fatalf("reply parent_id = %s, want %s", reply.ParentID, parent.ID)
	}
}

func TestWikiCommentService_List_ReturnsThreadAndStats(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	parent, _ := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "top"})
	svc.Create(context.Background(), 1, "kb-1", "page-a", "u-2", "Bob", "", types.WikiCommentCreateRequest{Body: "r1", ParentID: parent.ID})
	svc.Create(context.Background(), 1, "kb-1", "page-a", "u-3", "Cara", "", types.WikiCommentCreateRequest{Body: "r2", ParentID: parent.ID})

	resp, err := svc.List(context.Background(), 1, "kb-1", "page-a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.Comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(resp.Comments))
	}
	if resp.Stats.TotalReplies != 2 {
		t.Fatalf("expected TotalReplies=2, got %d", resp.Stats.TotalReplies)
	}
	if resp.Stats.TotalOpen != 3 {
		t.Fatalf("expected TotalOpen=3, got %d", resp.Stats.TotalOpen)
	}
}

func TestWikiCommentService_Update_OnlyAuthorCanEdit(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	c, _ := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "orig"})

	if _, err := svc.Update(context.Background(), 1, "kb-1", c.ID, "u-2", types.WikiCommentUpdateRequest{Body: "hijack"}); err != interfaces.ErrWikiCommentForbidden {
		t.Fatalf("expected ErrWikiCommentForbidden, got %v", err)
	}

	updated, err := svc.Update(context.Background(), 1, "kb-1", c.ID, "u-1", types.WikiCommentUpdateRequest{Body: "edited"})
	if err != nil {
		t.Fatalf("author Update: %v", err)
	}
	if updated.Body != "edited" {
		t.Fatalf("body = %q, want %q", updated.Body, "edited")
	}
}

func TestWikiCommentService_SetResolved_AuthorCanResolveOwn(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	c, _ := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "x"})

	resolved, err := svc.SetResolved(context.Background(), 1, "kb-1", c.ID, "u-1", false, true)
	if err != nil {
		t.Fatalf("SetResolved: %v", err)
	}
	if resolved.ResolvedBy != "u-1" {
		t.Fatalf("resolved_by = %q, want u-1", resolved.ResolvedBy)
	}

	open, err := svc.SetResolved(context.Background(), 1, "kb-1", c.ID, "u-1", false, false)
	if err != nil {
		t.Fatalf("un-resolve: %v", err)
	}
	if open.ResolvedBy != "" {
		t.Fatalf("resolved_by = %q, want empty", open.ResolvedBy)
	}
}

func TestWikiCommentService_SetResolved_NonAdminCannotResolveOthers(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	c, _ := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "x"})

	if _, err := svc.SetResolved(context.Background(), 1, "kb-1", c.ID, "u-2", false, true); err != interfaces.ErrWikiCommentForbidden {
		t.Fatalf("expected ErrWikiCommentForbidden, got %v", err)
	}

	// Admin bypass: KB owner / tenant admin can resolve any thread.
	resolved, err := svc.SetResolved(context.Background(), 1, "kb-1", c.ID, "admin-1", true, true)
	if err != nil {
		t.Fatalf("admin resolve: %v", err)
	}
	if resolved.ResolvedBy != "admin-1" {
		t.Fatalf("resolved_by = %q, want admin-1", resolved.ResolvedBy)
	}
}

func TestWikiCommentService_Delete_OnlyAuthorOrAdmin(t *testing.T) {
	repo := newStubWikiCommentRepo()
	svc := NewWikiCommentService(repo)

	c, _ := svc.Create(context.Background(), 1, "kb-1", "page-a", "u-1", "Alice", "", types.WikiCommentCreateRequest{Body: "x"})

	if err := svc.Delete(context.Background(), 1, "kb-1", c.ID, "u-2", false); err != interfaces.ErrWikiCommentForbidden {
		t.Fatalf("expected ErrWikiCommentForbidden, got %v", err)
	}

	if err := svc.Delete(context.Background(), 1, "kb-1", c.ID, "u-1", false); err != nil {
		t.Fatalf("author delete: %v", err)
	}
	if _, ok := repo.rows[c.ID]; ok {
		t.Fatalf("comment still present after author delete")
	}
}
