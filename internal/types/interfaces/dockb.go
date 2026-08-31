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

// WKDatabaseRepository persists database schemas and rows.
type WKDatabaseRepository interface {
	Create(ctx context.Context, db *types.WKDatabase) error
	Update(ctx context.Context, db *types.WKDatabase) error
	Get(ctx context.Context, tenantID string, id uint64) (*types.WKDatabase, error)
	List(ctx context.Context, tenantID string, limit, offset int) ([]*types.WKDatabase, int, error)
	DeleteDatabase(ctx context.Context, tenantID string, id uint64) error

	InsertRow(ctx context.Context, row *types.WKDatabaseRow) error
	UpdateRow(ctx context.Context, row *types.WKDatabaseRow) error
	GetRow(ctx context.Context, tenantID string, id uint64) (*types.WKDatabaseRow, error)
	ListRows(ctx context.Context, tenantID string, databaseID uint64, limit, offset int) ([]*types.WKDatabaseRow, int, error)
	DeleteRow(ctx context.Context, tenantID string, id uint64) error
}

// Summariser is the LLM boundary the dockb service depends on. Keeping
// it as an interface lets the production code inject the real chat
// pipeline while tests inject a stub that returns deterministic text.
type Summariser interface {
	Summarize(ctx context.Context, text string) (summary string, keyphrases []string, tags []string, err error)
}
