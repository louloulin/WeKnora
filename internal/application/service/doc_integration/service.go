package doc_integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors returned by the service layer. Exposed for the
// handler to map them onto HTTP status codes.
var (
	ErrInvalidRequest = errors.New("doc_integration: invalid request")
	ErrNotFound       = errors.New("doc_integration: not found")
	ErrUnknownMode    = errors.New("doc_integration: unknown assistant mode")
)

// KGLinker is the contract we need from the Build #35 KG service.
// The doc-integration service calls into it to write entity /
// relation rows produced by the linker pipeline.
type KGLinker interface {
	// EnsureEntity upserts an entity and returns its id. If the
	// entity already exists (by name within the KB scope) the id is
	// returned unchanged.
	EnsureEntity(ctx context.Context, tenantID uint64, kbID, name, supertag string) (string, error)
	// EnsureRelation upserts a directed relation between two
	// entities (resolved by name) and returns its id.
	EnsureRelation(ctx context.Context, tenantID uint64, kbID, srcName, dstName, relType string) (string, error)
}

// KBSearcher is the contract we need from the KB / RAG layer for
// the assistant panel. Returning top-k hits with title + snippet is
// enough for the citation surface.
type KBSearcher interface {
	Search(ctx context.Context, tenantID uint64, kbID, query string, topK int) ([]KBHit, error)
}

// KBHit is a minimal citation shape used by the assistant panel.
type KBHit struct {
	ChunkID string  `json:"chunk_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// WikiSearcher is the contract we need from the wiki search layer
// (BM25 + vector). Mirrors KBSearcher but for wiki pages.
type WikiSearcher interface {
	Search(ctx context.Context, tenantID uint64, query string, topK int) ([]WikiHit, error)
}

// WikiHit is a minimal citation shape used by the assistant panel.
type WikiHit struct {
	PageID  string  `json:"page_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// LLMCaller is the contract we need from the chat model backend.
// Implementations live in internal/application/service/llm.
type LLMCaller interface {
	Complete(ctx context.Context, prompt string, opts map[string]any) (string, error)
}

// Service is the application service for the Build #42 docs × KB
// integration. It composes the doc-integration repository with the
// KG / KB / Wiki / LLM collaborators injected at construction.
type Service struct {
	repo     interfaces.DocIntegrationRepository
	kg       KGLinker
	kb       KBSearcher
	wiki     WikiSearcher
	llm      LLMCaller
	now      func() time.Time
}

// NewService constructs a doc-integration Service. Pass nil for any
// collaborator that is not yet wired in dev / test environments.
func NewService(
	repo interfaces.DocIntegrationRepository,
	kg KGLinker,
	kb KBSearcher,
	wiki WikiSearcher,
	llm LLMCaller,
) *Service {
	return &Service{
		repo: repo,
		kg:   kg,
		kb:   kb,
		wiki: wiki,
		llm:  llm,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// SetNow lets tests freeze time.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// --- Doc ↔ KG relations ---

// LinkDocToKG writes a relation between a document and a KG entity
// or relation. It is idempotent on (source_type, source_id,
// target_type, target_id, kind).
func (s *Service) LinkDocToKG(ctx context.Context, rel *types.DocKgRelation) error {
	if rel.SourceType == "" || rel.SourceID == "" || rel.TargetType == "" || rel.TargetID == "" {
		return ErrInvalidRequest
	}
	if rel.Kind == "" {
		rel.Kind = types.DocKgMentionsEntity
	}
	if rel.Confidence == 0 {
		rel.Confidence = 1.0
	}
	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = s.now()
	}
	return s.repo.UpsertDocKgRelation(ctx, rel)
}

// ListDocKgRelationsBySource returns every doc-KG relation
// originating from a single document.
func (s *Service) ListDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) ([]*types.DocKgRelation, error) {
	if sourceType == "" || sourceID == "" {
		return nil, ErrInvalidRequest
	}
	return s.repo.ListDocKgRelationsBySource(ctx, sourceType, sourceID)
}

// ListDocKgRelationsByTarget returns every doc-KG relation pointing
// at a single KG entity / relation.
func (s *Service) ListDocKgRelationsByTarget(ctx context.Context, targetType, targetID string) ([]*types.DocKgRelation, error) {
	if targetType == "" || targetID == "" {
		return nil, ErrInvalidRequest
	}
	return s.repo.ListDocKgRelationsByTarget(ctx, targetType, targetID)
}

// DeleteDocKgRelationsBySource clears all doc-KG relations
// originating from a document. Called when a wiki page is deleted
// or fully rewritten.
func (s *Service) DeleteDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) error {
	return s.repo.DeleteDocKgRelationsBySource(ctx, sourceType, sourceID)
}

// --- KB → wiki reverse references ---

// LinkKbToWiki writes a reverse reference from a KB chunk back to a
// wiki page that cites it.
func (s *Service) LinkKbToWiki(ctx context.Context, ref *types.KbWikiReference) error {
	if ref.KBChunkID == "" || ref.WikiPageID == "" {
		return ErrInvalidRequest
	}
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = s.now()
	}
	ref.UpdatedAt = s.now()
	return s.repo.UpsertKbWikiReference(ctx, ref)
}

// ListKbWikiReferencesByChunk returns every wiki page that cites a
// given KB chunk. Drives the "open in wiki" link on KB answers.
func (s *Service) ListKbWikiReferencesByChunk(ctx context.Context, kbChunkID string) ([]*types.KbWikiReference, error) {
	if kbChunkID == "" {
		return nil, ErrInvalidRequest
	}
	return s.repo.ListKbWikiReferencesByChunk(ctx, kbChunkID)
}

// ListKbWikiReferencesByPage returns every KB chunk cited by a
// given wiki page. Used by the editor sidebar.
func (s *Service) ListKbWikiReferencesByPage(ctx context.Context, wikiPageID string) ([]*types.KbWikiReference, error) {
	if wikiPageID == "" {
		return nil, ErrInvalidRequest
	}
	return s.repo.ListKbWikiReferencesByPage(ctx, wikiPageID)
}

// --- Inline KB citations ---

// AddInlineKBRef records a new inline KB citation on a wiki page.
func (s *Service) AddInlineKBRef(ctx context.Context, ref *types.InlineKBRef) error {
	if ref.WikiPageID == "" || ref.KBChunkID == "" {
		return ErrInvalidRequest
	}
	if ref.Kind == "" {
		ref.Kind = types.InlineKBRefText
	}
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = s.now()
	}
	ref.UpdatedAt = s.now()
	return s.repo.UpsertInlineKBRef(ctx, ref)
}

// ListInlineKBRefsByPage returns the inline KB citations on a wiki
// page, ordered by position.
func (s *Service) ListInlineKBRefsByPage(ctx context.Context, wikiPageID string) ([]*types.InlineKBRef, error) {
	if wikiPageID == "" {
		return nil, ErrInvalidRequest
	}
	return s.repo.ListInlineKBRefsByPage(ctx, wikiPageID)
}

// ResetInlineKBRefs replaces all inline KB citations on a wiki
// page. Called by the editor on save.
func (s *Service) ResetInlineKBRefs(ctx context.Context, wikiPageID string, refs []*types.InlineKBRef) error {
	if wikiPageID == "" {
		return ErrInvalidRequest
	}
	if err := s.repo.DeleteInlineKBRefsByPage(ctx, wikiPageID); err != nil {
		return err
	}
	for _, r := range refs {
		if r == nil {
			continue
		}
		r.WikiPageID = wikiPageID
		if r.Kind == "" {
			r.Kind = types.InlineKBRefText
		}
		if r.CreatedAt.IsZero() {
			r.CreatedAt = s.now()
		}
		r.UpdatedAt = s.now()
		if err := s.repo.UpsertInlineKBRef(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// --- AI Assistant Panel ---

// Assistant runs the unified AI Assistant Panel. The mode field
// drives which sub-pipeline is invoked (chat / search / create).
// Returns ErrUnknownMode if mode is not one of the registered
// DocAssistantMode values.
func (s *Service) Assistant(ctx context.Context, req *types.DocAssistantRequest) (*types.DocAssistantResponse, error) {
	if req == nil || req.Prompt == "" {
		return nil, ErrInvalidRequest
	}
	switch req.Mode {
	case types.AssistantModeChat:
		return s.assistantChat(ctx, req)
	case types.AssistantModeSearch:
		return s.assistantSearch(ctx, req)
	case types.AssistantModeCreate:
		return s.assistantCreate(ctx, req)
	case "":
		return nil, ErrInvalidRequest
	default:
		return nil, ErrUnknownMode
	}
}

// assistantChat handles the chat mode: it gathers KB + wiki + KG
// hits, composes a RAG prompt, and (if LLM is wired) delegates to
// the chat model. Citations are returned verbatim so the UI can
// render the source list.
func (s *Service) assistantChat(ctx context.Context, req *types.DocAssistantRequest) (*types.DocAssistantResponse, error) {
	resp := &types.DocAssistantResponse{Mode: types.AssistantModeChat}
	kbHits := s.searchKB(ctx, req)
	wikiHits := s.searchWiki(ctx, req)
	for _, h := range kbHits {
		resp.Citations = append(resp.Citations, types.DocAssistantCitation{
			SourceType: "kb_chunk", SourceID: h.ChunkID,
			Title: h.Title, Snippet: h.Snippet, Score: h.Score,
		})
	}
	for _, h := range wikiHits {
		resp.Citations = append(resp.Citations, types.DocAssistantCitation{
			SourceType: "wiki_page", SourceID: h.PageID,
			Title: h.Title, Snippet: h.Snippet, Score: h.Score,
		})
	}
	if s.llm == nil {
		// Dev / test fallback: return a deterministic stub so the
		// pipeline still produces useful test coverage.
		resp.Answer = stubAnswer(req.Prompt, kbHits, wikiHits)
		resp.Usage = estimateUsage(req.Prompt, resp.Answer)
		return resp, nil
	}
	prompt := composeChatPrompt(req.Prompt, kbHits, wikiHits, req.ContextIDs)
	answer, err := s.llm.Complete(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	resp.Answer = answer
	resp.Usage = estimateUsage(prompt, answer)
	return resp, nil
}

// assistantSearch handles the search mode: returns the unified KB
// + Wiki hit list. The Answer field is a short natural-language
// summary of the top hits.
func (s *Service) assistantSearch(ctx context.Context, req *types.DocAssistantRequest) (*types.DocAssistantResponse, error) {
	resp := &types.DocAssistantResponse{Mode: types.AssistantModeSearch}
	kbHits := s.searchKB(ctx, req)
	wikiHits := s.searchWiki(ctx, req)
	for _, h := range kbHits {
		resp.Citations = append(resp.Citations, types.DocAssistantCitation{
			SourceType: "kb_chunk", SourceID: h.ChunkID,
			Title: h.Title, Snippet: h.Snippet, Score: h.Score,
		})
	}
	for _, h := range wikiHits {
		resp.Citations = append(resp.Citations, types.DocAssistantCitation{
			SourceType: "wiki_page", SourceID: h.PageID,
			Title: h.Title, Snippet: h.Snippet, Score: h.Score,
		})
	}
	resp.Answer = summariseHits(req.Prompt, kbHits, wikiHits)
	return resp, nil
}

// assistantCreate handles the create mode: delegates to the LLM to
// produce structured output (Wiki page draft, KG entity, KG
// relation). Real KG / wiki writes are deferred to the caller's
// transaction; the service returns the proposed entities in
// DocAssistantResponse.Created so the editor can show a preview.
func (s *Service) assistantCreate(ctx context.Context, req *types.DocAssistantRequest) (*types.DocAssistantResponse, error) {
	resp := &types.DocAssistantResponse{Mode: types.AssistantModeCreate}
	if s.llm == nil {
		// Without an LLM we cannot generate content. Surface a
		// deterministic preview so the UI can render the empty state.
		resp.Answer = "LLM not configured; create mode is read-only."
		return resp, nil
	}
	prompt := composeCreatePrompt(req.Prompt, req.ContextIDs)
	answer, err := s.llm.Complete(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	resp.Answer = answer
	resp.Created = parseCreatedEntities(answer)
	resp.Usage = estimateUsage(prompt, answer)
	return resp, nil
}

// --- private helpers ---

func (s *Service) searchKB(ctx context.Context, req *types.DocAssistantRequest) []KBHit {
	if s.kb == nil {
		return nil
	}
	kbID := ""
	if len(req.ContextIDs) > 0 {
		kbID = req.ContextIDs[0]
	}
	hits, err := s.kb.Search(ctx, req.TenantID, kbID, req.Prompt, 5)
	if err != nil {
		return nil
	}
	return hits
}

func (s *Service) searchWiki(ctx context.Context, req *types.DocAssistantRequest) []WikiHit {
	if s.wiki == nil {
		return nil
	}
	hits, err := s.wiki.Search(ctx, req.TenantID, req.Prompt, 5)
	if err != nil {
		return nil
	}
	return hits
}

// stubAnswer produces a deterministic answer string for tests / dev
// environments without an LLM. It lists the top hits so the UI can
// still render a meaningful response.
func stubAnswer(prompt string, kb []KBHit, wiki []WikiHit) string {
	var sb strings.Builder
	sb.WriteString("Stub answer for: ")
	sb.WriteString(prompt)
	sb.WriteString("\n\n")
	if len(kb) > 0 {
		sb.WriteString("KB hits:\n")
		for _, h := range kb {
			sb.WriteString("  - ")
			sb.WriteString(h.Title)
			sb.WriteString("\n")
		}
	}
	if len(wiki) > 0 {
		sb.WriteString("Wiki hits:\n")
		for _, h := range wiki {
			sb.WriteString("  - ")
			sb.WriteString(h.Title)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func summariseHits(prompt string, kb []KBHit, wiki []WikiHit) string {
	return stubAnswer(prompt, kb, wiki)
}

// composeChatPrompt builds the RAG prompt sent to the LLM. It is
// deterministic so tests can assert on it.
func composeChatPrompt(prompt string, kb []KBHit, wiki []WikiHit, pinned []string) string {
	var sb strings.Builder
	sb.WriteString("Answer the question using only the provided context. Cite each fact with [n].\n\n")
	if len(pinned) > 0 {
		sb.WriteString("Pinned context IDs: ")
		sb.WriteString(strings.Join(pinned, ", "))
		sb.WriteString("\n\n")
	}
	if len(kb) > 0 {
		sb.WriteString("KB context:\n")
		for i, h := range kb {
			sb.WriteString("[")
			sb.WriteString(itoa(i + 1))
			sb.WriteString("] ")
			sb.WriteString(h.Title)
			sb.WriteString(" — ")
			sb.WriteString(h.Snippet)
			sb.WriteString("\n")
		}
	}
	if len(wiki) > 0 {
		sb.WriteString("\nWiki context:\n")
		for i, h := range wiki {
			sb.WriteString("[")
			sb.WriteString(itoa(len(kb) + i + 1))
			sb.WriteString("] ")
			sb.WriteString(h.Title)
			sb.WriteString(" — ")
			sb.WriteString(h.Snippet)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\nQuestion: ")
	sb.WriteString(prompt)
	return sb.String()
}

// composeCreatePrompt instructs the LLM to emit structured JSON
// describing entities / relations to create.
func composeCreatePrompt(prompt string, pinned []string) string {
	var sb strings.Builder
	sb.WriteString("Produce JSON describing entities and relations to create. Use the schema: {entities: [{name, supertag}], relations: [{src, dst, type}]}.\n\nPrompt: ")
	sb.WriteString(prompt)
	if len(pinned) > 0 {
		sb.WriteString("\nPinned: ")
		sb.WriteString(strings.Join(pinned, ", "))
	}
	return sb.String()
}

// parseCreatedEntities extracts structured entities from the LLM
// response. It is intentionally tolerant: malformed JSON is
// ignored so dev environments without a real LLM can still run.
func parseCreatedEntities(answer string) []types.DocAssistantCreated {
	var out []types.DocAssistantCreated
	dec := json.NewDecoder(strings.NewReader(answer))
	for dec.More() {
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			break
		}
		if ents, ok := raw["entities"].([]any); ok {
			for _, e := range ents {
				m, _ := e.(map[string]any)
				out = append(out, types.DocAssistantCreated{
					Kind:  "kg_entity",
					ID:    stringOf(m, "id"),
					Title: stringOf(m, "name"),
				})
			}
		}
		if rels, ok := raw["relations"].([]any); ok {
			for _, r := range rels {
				m, _ := r.(map[string]any)
				out = append(out, types.DocAssistantCreated{
					Kind:  "kg_relation",
					Title: stringOf(m, "type"),
				})
			}
		}
	}
	return out
}

func stringOf(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// estimateUsage is a very rough token estimator: 4 chars ≈ 1 token.
// It exists so the assistant response carries a non-zero Usage
// block even when the LLM doesn't report one.
func estimateUsage(prompt, answer string) types.DocAssistantUsage {
	p := len(prompt) / 4
	a := len(answer) / 4
	return types.DocAssistantUsage{
		PromptTokens:     p,
		CompletionTokens: a,
		TotalTokens:      p + a,
	}
}

func itoa(i int) string {
	// Avoid pulling strconv into this file just for the test surface.
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
