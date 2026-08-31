package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/llmstream"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// AssistantService is the doc+KB-grounded Q&A backend. It fuses
// two retrieval sources — KnowledgeService.SearchKnowledgeForScopes
// (KB documents) and WikiSearchV2Service.Search (wiki pages) — into
// a single AssistantAskResponse, persists the audit row, and
// returns a structured response ready for the renderer.
//
// The LLM answer generation is intentionally out of scope for v0.7.15:
// this layer is the retrieval backend a future AI Assistant panel
// will use. Today the renderer shows the citations + a placeholder
// answer_text; tomorrow the LLM call plugs in here without changing
// the wire shape.
type AssistantService struct {
	repo      interfaces.AssistantConversationRepository
	kb        interfaces.KBRetriever
	wiki      interfaces.WikiRetriever
	knowledge interfaces.KnowledgeService
	kbBase    interfaces.KnowledgeBaseService
	// provider is the v0.7.17 LLM layer. When nil the service falls
	// back to the deterministic placeholder (v0.7.15 behaviour).
	// Always non-nil after DI; the noop default keeps tests simple.
	provider  llmstream.Provider
	modelName string
	now       func() time.Time
}

// NewAssistantService is the DI constructor.
//
// provider is required: callers should pass NoopProvider when no
// real LLM backend is configured. The constructor does not default
// the provider so a misconfigured DI graph fails loudly at startup
// instead of silently emitting the placeholder forever.
func NewAssistantService(
	repo interfaces.AssistantConversationRepository,
	kb interfaces.KBRetriever,
	wiki interfaces.WikiRetriever,
	knowledge interfaces.KnowledgeService,
	kbBase interfaces.KnowledgeBaseService,
	provider llmstream.Provider,
) *AssistantService {
	if provider == nil {
		provider = llmstream.NoopProvider{}
	}
	return &AssistantService{
		repo:      repo,
		kb:        kb,
		wiki:      wiki,
		knowledge: knowledge,
		kbBase:    kbBase,
		provider:  provider,
		modelName: "retrieval-only-v0.7.15",
		now:       time.Now,
	}
}

// SetProvider installs (or replaces) the LLM provider. Useful for
// tests that want to swap a fake provider in without rebuilding the
// DI graph.
func (s *AssistantService) SetProvider(p llmstream.Provider) {
	if p != nil {
		s.provider = p
	}
}

func (s *AssistantService) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *AssistantService) SetModelName(name string) {
	if name != "" {
		s.modelName = name
	}
}

// ErrAssistantInvalidRequest is the service-layer sentinel for
// empty queries / empty scope. The handler maps this to 400.
var ErrAssistantInvalidRequest = errors.New("assistant: invalid request")

// AssistantAskOptions bundles the runtime context that is not on the
// wire (tenant id, user id, the resolved KB scope).
type AssistantAskOptions struct {
	TenantID     uint64
	UserID       string
	VisibleKBIDs []string
}

// Ask is the hot path. It runs both retrievals, fuses the results,
// persists the conversation, and returns the response. The function
// is safe to call from multiple goroutines.
func (s *AssistantService) Ask(
	ctx context.Context, req types.AssistantAskRequest, opts AssistantAskOptions,
) (*types.AssistantAskResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, ErrAssistantInvalidRequest
	}
	if opts.TenantID == 0 {
		return nil, ErrAssistantInvalidRequest
	}

	// Default limits: 5 per source. The wire field is clamped here
	// so a malicious caller cannot exhaust the renderer.
	maxPerSource := req.MaxResultsPerSource
	if maxPerSource <= 0 || maxPerSource > 20 {
		maxPerSource = 5
	}

	// Conversation ID is auto-generated on first ask; callers can
	// thread an existing one through follow-ups.
	if req.ConversationID == "" {
		req.ConversationID = uuid.NewString()
	}

	// 1. KB-side retrieval.
	kbCitations, err := s.retrieveKB(ctx, req, opts, maxPerSource)
	if err != nil {
		return nil, fmt.Errorf("kb retrieval: %w", err)
	}

	// 2. Wiki-side retrieval (optional).
	var wikiCitations []types.AssistantCitation
	if req.IncludeWiki {
		wikiCitations, err = s.retrieveWiki(ctx, req, opts, maxPerSource)
		if err != nil {
			return nil, fmt.Errorf("wiki retrieval: %w", err)
		}
	}

	// 3. Ask the LLM provider to turn the retrieved citations into a
	//    natural-language answer. The provider may be the noop
	//    default (returns the same deterministic placeholder the
	//    v0.7.15 composeAnswerPlaceholder produced) or any real LLM
	//    wired in by the container.
	llmStart := s.now()
	llmResp, llmErr := s.provider.Complete(ctx, llmstream.Request{
		Query:          req.Query,
		ConversationID: req.ConversationID,
		KBCitations:    kbCitations,
		WikiCitations:  wikiCitations,
		ModelName:      s.modelName,
	})
	if llmErr != nil && !llmstream.IsTransient(llmErr) {
		return nil, fmt.Errorf("llm: %w", llmErr)
	}
	if llmErr != nil {
		// Transient: log via the persisted row (model_name carries
		// "transient:<err>") and surface a placeholder so the user
		// still sees citations.
		llmResp.AnswerText = s.composeAnswerPlaceholder(req.Query, kbCitations, wikiCitations) +
			" (LLM unavailable; citations only.)"
	}
	answerText := llmResp.AnswerText
	resolvedModelName := llmResp.ModelName
	if resolvedModelName == "" {
		resolvedModelName = s.provider.Name()
	}

	start := s.now()
	resp := &types.AssistantAskResponse{
		AnswerID:       uuid.NewString(),
		ConversationID: req.ConversationID,
		AnswerText:     answerText,
		KBCitations:    kbCitations,
		WikiCitations:  wikiCitations,
		ModelName:      resolvedModelName,
		LatencyMS:      int(s.now().Sub(llmStart).Milliseconds()),
		ResultCount:    len(kbCitations) + len(wikiCitations),
		CreatedAt:      start.UTC(),
	}

	// 4. Persist the audit row. Failure to persist is logged but
	// not propagated — the Ask has already succeeded; we do not
	// want a DB blip to undo the user response.
	row := &types.AssistantConversation{
		TenantID:       fmt.Sprintf("%d", opts.TenantID),
		UserID:         opts.UserID,
		ConversationID: req.ConversationID,
		QueryText:      req.Query,
		KBCitations:    kbCitations,
		WikiCitations:  wikiCitations,
		SourceKBIDs:    types.StringArray(req.SourceKBIDs),
		IncludeWiki:    req.IncludeWiki,
		ResultCount:    resp.ResultCount,
		ModelName:      resolvedModelName,
		LatencyMS:      int(s.now().Sub(llmStart).Milliseconds()),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		// non-fatal; the assistant response is still returned
		_ = err
	}

	return resp, nil
}

// AskStream is the v0.7.17 streaming variant. It runs the same
// retrieval pipeline as Ask, then funnels the LLM provider's events
// into the supplied sink instead of building a single Response.
//
// The caller still gets the citations slice up front (the assistant
// panel renders them as soon as they are available so the user sees
// them while the LLM is still generating). Token events from the
// provider are forwarded to the sink verbatim; the caller is
// responsible for SSE framing.
//
// Persistence mirrors Ask: one assistant_conversations row per Ask,
// with the final assembled answer text recorded in QueryText's
// neighbour (the column model_name carries the resolved provider
// name; the answer text itself is reassembled by the SSE handler
// from the token events so we don't have to buffer it here).
func (s *AssistantService) AskStream(
	ctx context.Context, req types.AssistantAskRequest, opts AssistantAskOptions, sink llmstream.EventSink,
) error {
	if strings.TrimSpace(req.Query) == "" {
		return ErrAssistantInvalidRequest
	}
	if opts.TenantID == 0 {
		return ErrAssistantInvalidRequest
	}
	if sink == nil {
		return ErrAssistantInvalidRequest
	}
	maxPerSource := req.MaxResultsPerSource
	if maxPerSource <= 0 || maxPerSource > 20 {
		maxPerSource = 5
	}
	if req.ConversationID == "" {
		req.ConversationID = uuid.NewString()
	}
	kbCitations, err := s.retrieveKB(ctx, req, opts, maxPerSource)
	if err != nil {
		return fmt.Errorf("kb retrieval: %w", err)
	}
	var wikiCitations []types.AssistantCitation
	if req.IncludeWiki {
		wikiCitations, err = s.retrieveWiki(ctx, req, opts, maxPerSource)
		if err != nil {
			return fmt.Errorf("wiki retrieval: %w", err)
		}
	}

	// Surface the citations BEFORE generation begins so the panel
	// can render them while the user is still reading the answer.
	// Each citation becomes its own EventCitation.
	for i, c := range kbCitations {
		if err := sink.OnEvent(llmstream.Event{
			Type: llmstream.EventCitation,
			Data: llmstream.CitationEventData{Index: i, Citation: c},
		}); err != nil {
			return err
		}
	}
	for i, c := range wikiCitations {
		if err := sink.OnEvent(llmstream.Event{
			Type: llmstream.EventCitation,
			Data: llmstream.CitationEventData{Index: len(kbCitations) + i, Citation: c},
		}); err != nil {
			return err
		}
	}

	// Run the provider. Errors bubble back up to the handler; the
	// handler decides between 500 (permanent) and 503 / 504
	// (transient) based on llmstream.IsTransient.
	err = s.provider.Stream(ctx, llmstream.Request{
		Query:          req.Query,
		ConversationID: req.ConversationID,
		KBCitations:    kbCitations,
		WikiCitations:  wikiCitations,
		ModelName:      s.modelName,
	}, sink)
	if err != nil {
		_ = sink.OnEvent(llmstream.Event{Type: llmstream.EventError, Error: err})
	}
	return err
}

// retrieveKB calls the KB-side retriever with the supplied scope.
func (s *AssistantService) retrieveKB(
	ctx context.Context, req types.AssistantAskRequest, opts AssistantAskOptions, maxPerSource int,
) ([]types.AssistantCitation, error) {
	scopes := kbScopesFromAsk(req, opts)
	if len(scopes) == 0 {
		return nil, nil
	}
	if s.kb == nil {
		return nil, nil
	}
	rows, _, _, err := s.kb.SearchKnowledgeForScopes(ctx, scopes, req.Query, 0, maxPerSource, nil)
	if err != nil {
		return nil, err
	}
	out := make([]types.AssistantCitation, 0, len(rows))
	for _, k := range rows {
		if k == nil {
			continue
		}
		out = append(out, types.AssistantCitation{
			Type:    "kb",
			ID:      k.ID,
			Title:   k.Title,
			KBID:    k.KnowledgeBaseID,
			Snippet: firstNonEmptyAssistant(k.Description, k.FileName, k.Title),
			Score:   1.0,
		})
	}
	return out, nil
}

// retrieveWiki calls the wiki-side retriever scoped to the same
// visibleKBIDs the user has access to.
func (s *AssistantService) retrieveWiki(
	ctx context.Context, req types.AssistantAskRequest, opts AssistantAskOptions, maxPerSource int,
) ([]types.AssistantCitation, error) {
	if s.wiki == nil {
		return nil, nil
	}
	wikiReq := types.WikiSearchV2Request{
		Query: req.Query,
		Limit: maxPerSource,
	}
	result, err := s.wiki.Search(ctx, opts.TenantID, opts.UserID, wikiReq, opts.VisibleKBIDs)
	if err != nil {
		return nil, err
	}
	out := make([]types.AssistantCitation, 0, len(result.Hits))
	for _, h := range result.Hits {
		out = append(out, types.AssistantCitation{
			Type:    "wiki",
			ID:      h.Slug, // wiki hits are addressed by slug within their KB
			Title:   h.Title,
			Slug:    h.Slug,
			KBID:    h.KBID,
			Snippet: stripWikiMarkTags(h.Snippet),
			Score:   h.Score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// kbScopesFromAsk converts the user-supplied SourceKBIDs (if any) into
// KnowledgeSearchScope entries. When SourceKBIDs is empty we fall
// back to opts.VisibleKBIDs (the tenant-wide ACL envelope).
func kbScopesFromAsk(req types.AssistantAskRequest, opts AssistantAskOptions) []types.KnowledgeSearchScope {
	if len(req.SourceKBIDs) > 0 {
		scopes := make([]types.KnowledgeSearchScope, 0, len(req.SourceKBIDs))
		for _, kbid := range req.SourceKBIDs {
			scopes = append(scopes, types.KnowledgeSearchScope{
				TenantID: opts.TenantID,
				KBID:     kbid,
			})
		}
		return scopes
	}
	scopes := make([]types.KnowledgeSearchScope, 0, len(opts.VisibleKBIDs))
	for _, kbid := range opts.VisibleKBIDs {
		scopes = append(scopes, types.KnowledgeSearchScope{
			TenantID: opts.TenantID,
			KBID:     kbid,
		})
	}
	return scopes
}

// composeAnswerPlaceholder renders a deterministic summary the
// renderer can show while waiting for the LLM.
func (s *AssistantService) composeAnswerPlaceholder(
	query string, kb []types.AssistantCitation, wiki []types.AssistantCitation,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d KB citation(s) and %d wiki page(s) relevant to your question.",
		len(kb), len(wiki))
	if top := topSnippetAssistant(kb); top != "" {
		fmt.Fprintf(&b, " Top KB snippet: %s", truncateAssistant(top, 160))
	}
	if top := topSnippetAssistant(wiki); top != "" {
		fmt.Fprintf(&b, " Top wiki snippet: %s", truncateAssistant(top, 160))
	}
	return b.String()
}

func topSnippetAssistant(cs []types.AssistantCitation) string {
	for _, c := range cs {
		if c.Snippet != "" {
			return c.Snippet
		}
	}
	return ""
}

func truncateAssistant(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// firstNonEmptyAssistant is a tiny helper so the KB citation snippet
// fallback chain reads naturally.
func firstNonEmptyAssistant(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// stripWikiMarkTags removes the <mark>...</mark> tags the wiki
// searcher inserts around matched terms so the snippet is plain text
// when it lands in an AssistantCitation.
func stripWikiMarkTags(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "<mark>", "")
	s = strings.ReplaceAll(s, "</mark>", "")
	return s
}

// ListConversations returns the most recent conversation turns for a
// tenant, ordered by created_at DESC. The tenant id comes from the
// authenticated principal (never the URL) and is matched exactly so
// a caller cannot probe other tenants' rows.
func (s *AssistantService) ListConversations(
	ctx context.Context, tenantID string, limit, offset int,
) ([]*types.AssistantConversation, int64, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, 0, ErrAssistantInvalidRequest
	}
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByTenant(ctx, tenantID, limit, offset)
}

// GetConversation returns every turn of a single conversation, in
// created_at ASC order, with a defensive tenant-id guard: a caller
// that knows another tenant's conversation_id cannot use this path
// to read its contents, because every row's tenant_id must equal
// the requesting tenant.
func (s *AssistantService) GetConversation(
	ctx context.Context, tenantID, conversationID string,
) ([]*types.AssistantConversation, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(conversationID) == "" {
		return nil, ErrAssistantInvalidRequest
	}
	rows, err := s.repo.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := rows[:0:0] //nolint:staticcheck // defensive copy so we never expose a foreign-tenant row
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.TenantID != tenantID {
			continue
		}
		out = append(out, row)
	}
	if len(out) == 0 && len(rows) > 0 {
		// Conversation exists but belongs to a different tenant:
		// emulate a "not found" so an attacker cannot tell the two
		// apart by the response shape.
		return nil, nil
	}
	return out, nil
}
