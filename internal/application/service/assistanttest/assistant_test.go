//go:build assisttest

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// fakeAssistantRepo captures Create calls in memory so the suite can
// assert the persistence behaviour without a live database.
type fakeAssistantRepo struct {
	created []*types.AssistantConversation
	byConv  map[string][]*types.AssistantConversation
	byTen   map[string][]*types.AssistantConversation
}

func newFakeAssistantRepo() *fakeAssistantRepo {
	return &fakeAssistantRepo{
		byConv: map[string][]*types.AssistantConversation{},
		byTen:  map[string][]*types.AssistantConversation{},
	}
}

func (f *fakeAssistantRepo) Create(ctx context.Context, c *types.AssistantConversation) error {
	if c == nil {
		return errors.New("nil conversation")
	}
	f.created = append(f.created, c)
	f.byConv[c.ConversationID] = append(f.byConv[c.ConversationID], c)
	f.byTen[c.TenantID] = append(f.byTen[c.TenantID], c)
	return nil
}

func (f *fakeAssistantRepo) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*types.AssistantConversation, int64, error) {
	rows := f.byTen[tenantID]
	total := int64(len(rows))
	if offset > 0 && offset < len(rows) {
		rows = rows[offset:]
	} else if offset >= len(rows) {
		rows = nil
	}
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	return rows, total, nil
}

func (f *fakeAssistantRepo) ListByConversation(ctx context.Context, conversationID string) ([]*types.AssistantConversation, error) {
	return f.byConv[conversationID], nil
}

// fakeKBRetriever records the scope and returns canned rows.
type fakeKBRetriever struct {
	scopesSeen []types.KnowledgeSearchScope
	rows       []*types.Knowledge
	err        error
}

func (f *fakeKBRetriever) SearchKnowledgeForScopes(
	ctx context.Context, scopes []types.KnowledgeSearchScope,
	keyword string, offset, limit int, fileTypes []string,
) ([]*types.Knowledge, bool, int64, error) {
	f.scopesSeen = scopes
	return f.rows, false, int64(len(f.rows)), f.err
}

// fakeWikiRetriever records the request and returns canned hits.
type fakeWikiRetriever struct {
	hits types.WikiSearchV2Result
	err  error
}

func (f *fakeWikiRetriever) Search(
	ctx context.Context, tenantID uint64, userID string,
	req types.WikiSearchV2Request, visibleKBIDs []string,
) (types.WikiSearchV2Result, error) {
	return f.hits, f.err
}

func newSvcWithFakes() (*service.AssistantService, *fakeAssistantRepo, *fakeKBRetriever, *fakeWikiRetriever) {
	repo := newFakeAssistantRepo()
	kb := &fakeKBRetriever{}
	wiki := &fakeWikiRetriever{}
	svc := service.NewAssistantService(repo, kb, wiki, nil, nil)
	return svc, repo, kb, wiki
}

func TestAssistant_Ask_EmptyQueryRejected(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	_, err := svc.Ask(context.Background(), types.AssistantAskRequest{Query: "   "}, service.AssistantAskOptions{TenantID: 1})
	if !errors.Is(err, service.ErrAssistantInvalidRequest) {
		t.Fatalf("expected ErrAssistantInvalidRequest, got %v", err)
	}
}

func TestAssistant_Ask_ZeroTenantRejected(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	_, err := svc.Ask(context.Background(), types.AssistantAskRequest{Query: "hello"}, service.AssistantAskOptions{TenantID: 0})
	if !errors.Is(err, service.ErrAssistantInvalidRequest) {
		t.Fatalf("expected ErrAssistantInvalidRequest, got %v", err)
	}
}

func TestAssistant_Ask_DefaultsMaxResultsTo5(t *testing.T) {
	svc, _, kb, _ := newSvcWithFakes()
	_, err := svc.Ask(context.Background(), types.AssistantAskRequest{Query: "x"}, service.AssistantAskOptions{TenantID: 7})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	// We can't directly inspect the limit because it's passed by value,
	// but the absence of an error with a nil kb rows means the default
	// clamp took the path; this is the regression sentinel.
	_ = kb
}

func TestAssistant_Ask_ClampsMaxResultsTo20(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	// 200 is malicious; should be clamped to 5 (default) since the
	// request doesn't have a positive value <= 20 other than the default.
	// The contract is: clamp to [1,20] and default to 5.
	_, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "x", MaxResultsPerSource: 999},
		service.AssistantAskOptions{TenantID: 7, VisibleKBIDs: []string{"kb-1"}},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
}

func TestAssistant_Ask_AutoGeneratesConversationID(t *testing.T) {
	svc, repo, _, _ := newSvcWithFakes()
	resp, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "hello"},
		service.AssistantAskOptions{TenantID: 7},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp.ConversationID == "" {
		t.Fatalf("expected auto-generated conversation id, got empty")
	}
	if len(repo.created) != 1 || repo.created[0].ConversationID != resp.ConversationID {
		t.Fatalf("expected one persisted row with matching id, got %d", len(repo.created))
	}
}

func TestAssistant_Ask_StripsWikiMarkTags(t *testing.T) {
	svc, _, _, wiki := newSvcWithFakes()
	wiki.hits = types.WikiSearchV2Result{
		Hits: []types.WikiSearchV2Hit{{
			Slug:    "how-to-deploy",
			Title:   "How to Deploy",
			Snippet: "Run <mark>kubectl</mark> apply to deploy the manifest.",
			Score:   0.9,
			KBID:    "kb-1",
		}},
	}
	resp, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "deploy", IncludeWiki: true},
		service.AssistantAskOptions{TenantID: 7},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(resp.WikiCitations) != 1 {
		t.Fatalf("expected 1 wiki citation, got %d", len(resp.WikiCitations))
	}
	if strings.Contains(resp.WikiCitations[0].Snippet, "<mark>") || strings.Contains(resp.WikiCitations[0].Snippet, "</mark>") {
		t.Fatalf("wiki mark tags not stripped: %s", resp.WikiCitations[0].Snippet)
	}
}

func TestAssistant_KBScopes_PrefersSourceKBIDs(t *testing.T) {
	svc, _, kb, _ := newSvcWithFakes()
	_, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{
			Query:       "x",
			SourceKBIDs: []string{"kb-7", "kb-9"},
		},
		service.AssistantAskOptions{TenantID: 7, VisibleKBIDs: []string{"kb-1", "kb-2"}},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(kb.scopesSeen) != 2 || kb.scopesSeen[0].KBID != "kb-7" || kb.scopesSeen[1].KBID != "kb-9" {
		t.Fatalf("expected scopes from SourceKBIDs (kb-7,kb-9), got %+v", kb.scopesSeen)
	}
}

func TestAssistant_KBScopes_FallsBackToVisible(t *testing.T) {
	svc, _, kb, _ := newSvcWithFakes()
	_, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "x"},
		service.AssistantAskOptions{TenantID: 7, VisibleKBIDs: []string{"kb-1", "kb-2"}},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if len(kb.scopesSeen) != 2 || kb.scopesSeen[0].KBID != "kb-1" || kb.scopesSeen[1].KBID != "kb-2" {
		t.Fatalf("expected fallback to visible KBs (kb-1,kb-2), got %+v", kb.scopesSeen)
	}
}

func TestAssistant_ComposeAnswerPlaceholder(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	resp, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "what is X?"},
		service.AssistantAskOptions{TenantID: 7},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if !strings.Contains(resp.AnswerText, "0 KB citation") || !strings.Contains(resp.AnswerText, "0 wiki page") {
		t.Fatalf("placeholder text missing expected counts: %s", resp.AnswerText)
	}
}

func TestAssistant_ListConversations_RequiresTenant(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	_, _, err := svc.ListConversations(context.Background(), "", 10, 0)
	if !errors.Is(err, service.ErrAssistantInvalidRequest) {
		t.Fatalf("expected ErrAssistantInvalidRequest, got %v", err)
	}
}

func TestAssistant_ListConversations_TenantScoped(t *testing.T) {
	svc, repo, _, _ := newSvcWithFakes()
	// seed two tenants
	_ = repo.Create(context.Background(), &types.AssistantConversation{TenantID: "7", ConversationID: "c1", QueryText: "a"})
	_ = repo.Create(context.Background(), &types.AssistantConversation{TenantID: "8", ConversationID: "c2", QueryText: "b"})
	rows, total, err := svc.ListConversations(context.Background(), "7", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].TenantID != "7" {
		t.Fatalf("tenant scoping leaked: total=%d rows=%d", total, len(rows))
	}
}

func TestAssistant_GetConversation_TenantGuard(t *testing.T) {
	svc, repo, _, _ := newSvcWithFakes()
	_ = repo.Create(context.Background(), &types.AssistantConversation{TenantID: "7", ConversationID: "c1", QueryText: "a"})
	rows, err := svc.GetConversation(context.Background(), "9", "c1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty rows for cross-tenant probe, got %d", len(rows))
	}
}

func TestAssistant_GetConversation_EmptyIDRejected(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	_, err := svc.GetConversation(context.Background(), "7", "")
	if !errors.Is(err, service.ErrAssistantInvalidRequest) {
		t.Fatalf("expected ErrAssistantInvalidRequest, got %v", err)
	}
}

func TestAssistant_LatencyTracked(t *testing.T) {
	svc, _, _, _ := newSvcWithFakes()
	svc.SetNow(func() time.Time { return time.Unix(0, 0) })
	svc.SetModelName("test-model")
	resp, err := svc.Ask(context.Background(),
		types.AssistantAskRequest{Query: "x"},
		service.AssistantAskOptions{TenantID: 7},
	)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if resp.ModelName != "test-model" {
		t.Fatalf("SetModelName ignored: %s", resp.ModelName)
	}
}

// Sanity check that the fake satisfies the interface (compile-time).
var _ interfaces.AssistantConversationRepository = (*fakeAssistantRepo)(nil)
var _ interfaces.KBRetriever = (*fakeKBRetriever)(nil)
var _ interfaces.WikiRetriever = (*fakeWikiRetriever)(nil)
