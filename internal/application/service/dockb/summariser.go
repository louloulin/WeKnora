// Package dockb implements the v0.7.23 Doc ↔ KB AI Bridge. The service
// is responsible for taking a raw chunk produced by the knowledge
// pipeline and producing an AI summary + keyphrase list + tag list
// that is independently searchable. The summary is stored in
// doc_kb_summaries and re-used by the AI Assistant so it can answer
// "what is this document about?" without re-running retrieval over
// every raw chunk.
//
// The Summariser is injected as an interface (interfaces.Summariser)
// so we can swap the real LLM-backed implementation for a stub in
// tests without standing up a model server.
package dockb

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors.
var (
	ErrEmptyChunkText = errors.New("dockb: chunk text is empty")
)

// SummariserService orchestrates LLM summarisation and persistence.
type SummariserService struct {
	repo       interfaces.DocKBSummaryRepository
	summariser interfaces.Summariser
}

// NewSummariserService wires the service.
func NewSummariserService(
	repo interfaces.DocKBSummaryRepository,
	summariser interfaces.Summariser,
) *SummariserService {
	return &SummariserService{repo: repo, summariser: summariser}
}

// SummariseChunk generates and persists the summary for one chunk.
// Idempotent: re-running produces the same row, updated_at bumped.
func (s *SummariserService) SummariseChunk(ctx context.Context,
	tenantID, knowledgeID, chunkID, text, modelName string,
) (*types.DocKBSummary, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyChunkText
	}
	summary, keyphrases, tags, err := s.summariser.Summarize(ctx, text)
	if err != nil {
		return nil, err
	}
	row := &types.DocKBSummary{
		TenantID:    tenantID,
		KnowledgeID: knowledgeID,
		ChunkID:     chunkID,
		Summary:     summary,
		Keyphrases:  keyphrases,
		AutoTags:    tags,
		ModelName:   modelName,
		Confidence:  1.0,
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// ListByKnowledge returns all summaries for one knowledge entry.
func (s *SummariserService) ListByKnowledge(ctx context.Context, tenantID, knowledgeID string) ([]*types.DocKBSummary, error) {
	return s.repo.ListByKnowledge(ctx, tenantID, knowledgeID)
}

// GetByChunk returns one summary if it exists.
func (s *SummariserService) GetByChunk(ctx context.Context, tenantID, knowledgeID, chunkID string) (*types.DocKBSummary, error) {
	return s.repo.GetByChunk(ctx, tenantID, knowledgeID, chunkID)
}

// Delete removes one summary. Used when a chunk is removed.
func (s *SummariserService) Delete(ctx context.Context, tenantID string, id uint64) error {
	return s.repo.DeleteSummary(ctx, tenantID, id)
}
