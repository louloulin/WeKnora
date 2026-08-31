package doc_integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- fakes ---

type fakeRepo struct {
	docKg      []*types.DocKgRelation
	kbWiki     []*types.KbWikiReference
	inline     []*types.InlineKBRef
}

func (f *fakeRepo) UpsertDocKgRelation(ctx context.Context, r *types.DocKgRelation) error {
	for i, existing := range f.docKg {
		if existing.SourceType == r.SourceType &&
			existing.SourceID == r.SourceID &&
			existing.TargetType == r.TargetType &&
			existing.TargetID == r.TargetID &&
			existing.Kind == r.Kind {
			f.docKg[i] = r
			return nil
		}
	}
	f.docKg = append(f.docKg, r)
	return nil
}
func (f *fakeRepo) ListDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) ([]*types.DocKgRelation, error) {
	out := []*types.DocKgRelation{}
	for _, r := range f.docKg {
		if r.SourceType == sourceType && r.SourceID == sourceID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListDocKgRelationsByTarget(ctx context.Context, targetType, targetID string) ([]*types.DocKgRelation, error) {
	out := []*types.DocKgRelation{}
	for _, r := range f.docKg {
		if r.TargetType == targetType && r.TargetID == targetID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) DeleteDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) error {
	kept := f.docKg[:0]
	for _, r := range f.docKg {
		if !(r.SourceType == sourceType && r.SourceID == sourceID) {
			kept = append(kept, r)
		}
	}
	f.docKg = kept
	return nil
}

func (f *fakeRepo) UpsertKbWikiReference(ctx context.Context, r *types.KbWikiReference) error {
	for i, existing := range f.kbWiki {
		if existing.KBChunkID == r.KBChunkID && existing.WikiPageID == r.WikiPageID {
			f.kbWiki[i] = r
			return nil
		}
	}
	f.kbWiki = append(f.kbWiki, r)
	return nil
}
func (f *fakeRepo) ListKbWikiReferencesByChunk(ctx context.Context, kbChunkID string) ([]*types.KbWikiReference, error) {
	out := []*types.KbWikiReference{}
	for _, r := range f.kbWiki {
		if r.KBChunkID == kbChunkID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListKbWikiReferencesByPage(ctx context.Context, wikiPageID string) ([]*types.KbWikiReference, error) {
	out := []*types.KbWikiReference{}
	for _, r := range f.kbWiki {
		if r.WikiPageID == wikiPageID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) DeleteKbWikiReferencesByPage(ctx context.Context, wikiPageID string) error {
	kept := f.kbWiki[:0]
	for _, r := range f.kbWiki {
		if r.WikiPageID != wikiPageID {
			kept = append(kept, r)
		}
	}
	f.kbWiki = kept
	return nil
}

func (f *fakeRepo) UpsertInlineKBRef(ctx context.Context, r *types.InlineKBRef) error {
	for i, existing := range f.inline {
		if existing.WikiPageID == r.WikiPageID &&
			existing.KBChunkID == r.KBChunkID &&
			existing.Kind == r.Kind {
			f.inline[i] = r
			return nil
		}
	}
	f.inline = append(f.inline, r)
	return nil
}
func (f *fakeRepo) ListInlineKBRefsByPage(ctx context.Context, wikiPageID string) ([]*types.InlineKBRef, error) {
	out := []*types.InlineKBRef{}
	for _, r := range f.inline {
		if r.WikiPageID == wikiPageID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRepo) DeleteInlineKBRefsByPage(ctx context.Context, wikiPageID string) error {
	kept := f.inline[:0]
	for _, r := range f.inline {
		if r.WikiPageID != wikiPageID {
			kept = append(kept, r)
		}
	}
	f.inline = kept
	return nil
}

var _ interfaces.DocIntegrationRepository = (*fakeRepo)(nil)

// --- fake collaborators ---

type fakeKB struct{ hits []KBHit }

func (f *fakeKB) Search(ctx context.Context, tenantID uint64, kbID, query string, topK int) ([]KBHit, error) {
	return f.hits, nil
}

type fakeWiki struct{ hits []WikiHit }

func (f *fakeWiki) Search(ctx context.Context, tenantID uint64, query string, topK int) ([]WikiHit, error) {
	return f.hits, nil
}

type fakeLLM struct{ answer string }

func (f *fakeLLM) Complete(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	return f.answer, nil
}

type errLLM struct{}

func (errLLM) Complete(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	return "", errors.New("llm down")
}

// --- tests ---

func newService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{}
	s := NewService(repo, nil, &fakeKB{}, &fakeWiki{}, nil)
	return s, repo
}

func TestService_LinkDocToKG_RejectsMissingFields(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if err := s.LinkDocToKG(ctx, &types.DocKgRelation{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestService_LinkDocToKG_DefaultsKindAndConfidence(t *testing.T) {
	s, repo := newService(t)
	ctx := context.Background()
	err := s.LinkDocToKG(ctx, &types.DocKgRelation{
		TenantID:   1,
		SourceType: "wiki_page",
		SourceID:   "wp-1",
		TargetType: "kg_entity",
		TargetID:   "ke-1",
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if len(repo.docKg) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(repo.docKg))
	}
	if repo.docKg[0].Kind != types.DocKgMentionsEntity {
		t.Fatalf("kind defaulted to %s, want %s", repo.docKg[0].Kind, types.DocKgMentionsEntity)
	}
	if repo.docKg[0].Confidence != 1.0 {
		t.Fatalf("confidence defaulted to %v, want 1.0", repo.docKg[0].Confidence)
	}
}

func TestService_LinkDocToKG_IsIdempotent(t *testing.T) {
	s, repo := newService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.LinkDocToKG(ctx, &types.DocKgRelation{
			TenantID: 1, SourceType: "wiki_page", SourceID: "wp-1",
			TargetType: "kg_entity", TargetID: "ke-1",
		}); err != nil {
			t.Fatalf("link %d: %v", i, err)
		}
	}
	if len(repo.docKg) != 1 {
		t.Fatalf("expected 1 row after 3 idempotent inserts, got %d", len(repo.docKg))
	}
}

func TestService_ListDocKgRelationsBySource(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_ = s.LinkDocToKG(ctx, &types.DocKgRelation{TenantID: 1, SourceType: "wiki_page", SourceID: "wp-1", TargetType: "kg_entity", TargetID: "a"})
	_ = s.LinkDocToKG(ctx, &types.DocKgRelation{TenantID: 1, SourceType: "wiki_page", SourceID: "wp-1", TargetType: "kg_entity", TargetID: "b"})
	_ = s.LinkDocToKG(ctx, &types.DocKgRelation{TenantID: 1, SourceType: "wiki_page", SourceID: "wp-2", TargetType: "kg_entity", TargetID: "c"})
	out, err := s.ListDocKgRelationsBySource(ctx, "wiki_page", "wp-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestService_DeleteDocKgRelationsBySource(t *testing.T) {
	s, repo := newService(t)
	ctx := context.Background()
	_ = s.LinkDocToKG(ctx, &types.DocKgRelation{TenantID: 1, SourceType: "wiki_page", SourceID: "wp-1", TargetType: "kg_entity", TargetID: "a"})
	_ = s.LinkDocToKG(ctx, &types.DocKgRelation{TenantID: 1, SourceType: "wiki_page", SourceID: "wp-2", TargetType: "kg_entity", TargetID: "b"})
	if err := s.DeleteDocKgRelationsBySource(ctx, "wiki_page", "wp-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(repo.docKg) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(repo.docKg))
	}
}

func TestService_LinkKbToWiki_RejectsMissingFields(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if err := s.LinkKbToWiki(ctx, &types.KbWikiReference{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestService_LinkKbToWiki_AutoStampsTimestamps(t *testing.T) {
	s, repo := newService(t)
	s.SetNow(func() time.Time { return time.Unix(1234567890, 0).UTC() })
	ctx := context.Background()
	err := s.LinkKbToWiki(ctx, &types.KbWikiReference{
		TenantID: 1, KBChunkID: "kc-1", WikiPageID: "wp-1",
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if !repo.kbWiki[0].CreatedAt.Equal(time.Unix(1234567890, 0).UTC()) {
		t.Fatalf("CreatedAt not stamped")
	}
	if !repo.kbWiki[0].UpdatedAt.Equal(time.Unix(1234567890, 0).UTC()) {
		t.Fatalf("UpdatedAt not stamped")
	}
}

func TestService_ListKbWikiReferencesByChunk(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_ = s.LinkKbToWiki(ctx, &types.KbWikiReference{TenantID: 1, KBChunkID: "kc-1", WikiPageID: "wp-1"})
	_ = s.LinkKbToWiki(ctx, &types.KbWikiReference{TenantID: 1, KBChunkID: "kc-1", WikiPageID: "wp-2"})
	_ = s.LinkKbToWiki(ctx, &types.KbWikiReference{TenantID: 1, KBChunkID: "kc-2", WikiPageID: "wp-3"})
	out, err := s.ListKbWikiReferencesByChunk(ctx, "kc-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestService_AddInlineKBRef_RejectsMissingFields(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if err := s.AddInlineKBRef(ctx, &types.InlineKBRef{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest, got %v", err)
	}
}

func TestService_ResetInlineKBRefs(t *testing.T) {
	s, repo := newService(t)
	ctx := context.Background()
	// Seed existing refs on the page.
	_ = s.AddInlineKBRef(ctx, &types.InlineKBRef{TenantID: 1, WikiPageID: "wp-1", KBChunkID: "kc-1", Position: 1})
	_ = s.AddInlineKBRef(ctx, &types.InlineKBRef{TenantID: 1, WikiPageID: "wp-1", KBChunkID: "kc-2", Position: 2})
	// Reset with new refs.
	err := s.ResetInlineKBRefs(ctx, "wp-1", []*types.InlineKBRef{
		{TenantID: 1, KBChunkID: "kc-3", Position: 1},
		{TenantID: 1, KBChunkID: "kc-4", Position: 2},
	})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	// Should have exactly 2 refs now, all new.
	for _, r := range repo.inline {
		if r.WikiPageID != "wp-1" {
			t.Fatalf("foreign ref leaked: %+v", r)
		}
		if r.KBChunkID != "kc-3" && r.KBChunkID != "kc-4" {
			t.Fatalf("stale ref: %+v", r)
		}
	}
	if len(repo.inline) != 2 {
		t.Fatalf("expected 2, got %d", len(repo.inline))
	}
}

func TestService_Assistant_EmptyModeRejected(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.Assistant(ctx, &types.DocAssistantRequest{Prompt: "hi"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for empty mode, got %v", err)
	}
}

func TestService_Assistant_InvalidModeRejected(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.Assistant(ctx, &types.DocAssistantRequest{Mode: "fly", Prompt: "hi"}); !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("want ErrUnknownMode for unknown mode, got %v", err)
	}
}

func TestService_Assistant_EmptyPromptRejected(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.Assistant(ctx, &types.DocAssistantRequest{Mode: types.AssistantModeChat}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for empty prompt, got %v", err)
	}
}

func TestService_Assistant_ChatWithoutLLMReturnsStubWithCitations(t *testing.T) {
	repo := &fakeRepo{}
	kb := &fakeKB{hits: []KBHit{{ChunkID: "kc-1", Title: "Onboarding", Snippet: "Welcome...", Score: 0.9}}}
	wiki := &fakeWiki{hits: []WikiHit{{PageID: "wp-1", Title: "Team Handbook", Snippet: "We are...", Score: 0.8}}}
	s := NewService(repo, nil, kb, wiki, nil)
	ctx := context.Background()
	resp, err := s.Assistant(ctx, &types.DocAssistantRequest{
		TenantID: 1, Mode: types.AssistantModeChat, Prompt: "How do I onboard?",
	})
	if err != nil {
		t.Fatalf("assistant: %v", err)
	}
	if resp.Mode != types.AssistantModeChat {
		t.Fatalf("mode = %s, want chat", resp.Mode)
	}
	if len(resp.Citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(resp.Citations))
	}
	if resp.Answer == "" {
		t.Fatal("expected stub answer")
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("expected non-zero usage estimate")
	}
}

func TestService_Assistant_SearchReturnsCitations(t *testing.T) {
	repo := &fakeRepo{}
	kb := &fakeKB{hits: []KBHit{{ChunkID: "kc-1", Title: "Doc A"}}}
	wiki := &fakeWiki{hits: []WikiHit{{PageID: "wp-1", Title: "Wiki B"}}}
	s := NewService(repo, nil, kb, wiki, nil)
	ctx := context.Background()
	resp, err := s.Assistant(ctx, &types.DocAssistantRequest{
		TenantID: 1, Mode: types.AssistantModeSearch, Prompt: "search me",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if resp.Mode != types.AssistantModeSearch {
		t.Fatalf("mode = %s, want search", resp.Mode)
	}
	if len(resp.Citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(resp.Citations))
	}
}

func TestService_Assistant_CreateWithoutLLMReturnsReadOnlyNotice(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil, nil, nil, nil)
	ctx := context.Background()
	resp, err := s.Assistant(ctx, &types.DocAssistantRequest{
		TenantID: 1, Mode: types.AssistantModeCreate, Prompt: "Make a wiki page",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.Answer == "" {
		t.Fatal("expected read-only notice")
	}
	if len(resp.Created) != 0 {
		t.Fatalf("expected 0 created without LLM, got %d", len(resp.Created))
	}
}

func TestService_Assistant_ChatWithLLMUsesPrompt(t *testing.T) {
	repo := &fakeRepo{}
	llm := &fakeLLM{answer: "According to [1], you should..."}
	s := NewService(repo, nil, &fakeKB{hits: []KBHit{{ChunkID: "kc-1", Title: "Doc", Snippet: "context"}}}, &fakeWiki{}, llm)
	ctx := context.Background()
	resp, err := s.Assistant(ctx, &types.DocAssistantRequest{
		TenantID: 1, Mode: types.AssistantModeChat, Prompt: "question",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Answer != "According to [1], you should..." {
		t.Fatalf("llm answer not propagated: %s", resp.Answer)
	}
}

func TestService_Assistant_LLMErrorPropagates(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil, nil, nil, errLLM{})
	ctx := context.Background()
	if _, err := s.Assistant(ctx, &types.DocAssistantRequest{
		TenantID: 1, Mode: types.AssistantModeChat, Prompt: "x",
	}); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestComposeChatPrompt_IncludesKBAndWiki(t *testing.T) {
	prompt := composeChatPrompt("How do I onboard?", []KBHit{
		{ChunkID: "kc-1", Title: "Onboarding Guide", Snippet: "Welcome to the team"},
	}, []WikiHit{
		{PageID: "wp-1", Title: "Team Handbook", Snippet: "Mission & values"},
	}, []string{"ctx-1"})
	for _, want := range []string{"Onboarding Guide", "Team Handbook", "[1]", "[2]", "How do I onboard?", "ctx-1"} {
		if !contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s\n---", want, prompt)
		}
	}
}

func TestEstimateUsage(t *testing.T) {
	u := estimateUsage("hello world", "hi")
	if u.PromptTokens != 2 {
		t.Errorf("prompt tokens = %d, want 2", u.PromptTokens)
	}
	if u.CompletionTokens != 0 {
		t.Errorf("completion tokens = %d, want 0", u.CompletionTokens)
	}
	if u.TotalTokens != 2 {
		t.Errorf("total = %d, want 2", u.TotalTokens)
	}
}

func TestParseCreatedEntities_TolerantOfMalformedJSON(t *testing.T) {
	out := parseCreatedEntities("not json at all")
	if len(out) != 0 {
		t.Fatalf("malformed should give empty, got %d", len(out))
	}
}

func TestParseCreatedEntities_ExtractsEntitiesAndRelations(t *testing.T) {
	answer := `{"entities":[{"name":"Alice","id":"e1"},{"name":"Bob","id":"e2"}],"relations":[{"type":"knows","id":"r1"}]}`
	out := parseCreatedEntities(answer)
	if len(out) != 3 {
		t.Fatalf("expected 3 (2 entities + 1 relation), got %d", len(out))
	}
	var ents, rels int
	for _, c := range out {
		switch c.Kind {
		case "kg_entity":
			ents++
		case "kg_relation":
			rels++
		}
	}
	if ents != 2 || rels != 1 {
		t.Fatalf("counts ents=%d rels=%d", ents, rels)
	}
}

func TestItoa(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "0"}, {1, "1"}, {42, "42"}, {123456789, "123456789"}, {-7, "-7"},
	} {
		if got := itoa(tc.in); got != tc.want {
			t.Errorf("itoa(%d) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
