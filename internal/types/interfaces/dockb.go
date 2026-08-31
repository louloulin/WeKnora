package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// DocKBSummaryRepository persists the AI-generated doc ↔ KB summaries.
//
// All methods are tenant-scoped: a missing tenant_id returns "not
// found" rather than leaking cross-tenant data.
type DocKBSummaryRepository interface {
	Upsert(ctx context.Context, s *types.DocKBSummary) error
	GetByChunk(ctx context.Context, tenantID, knowledgeID, chunkID string) (*types.DocKBSummary, error)
	ListByKnowledge(ctx context.Context, tenantID, knowledgeID string) ([]*types.DocKBSummary, error)
	DeleteSummary(ctx context.Context, tenantID string, id uint64) error
}

// Summariser is the LLM boundary the dockb service depends on. Keeping
// it as an interface lets the production code inject the real chat
// pipeline while tests inject a stub that returns deterministic text.
type Summariser interface {
	Summarize(ctx context.Context, text string) (summary string, keyphrases []string, tags []string, err error)
}
