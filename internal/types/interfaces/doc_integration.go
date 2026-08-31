package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// DocIntegrationRepository persists the Build #42 docs × KB
// integration records. It is intentionally split from the existing
// wiki / kg repositories so the doc-integration feature can be
// evolved independently.
type DocIntegrationRepository interface {
	// Doc ↔ KG relations.
	UpsertDocKgRelation(ctx context.Context, r *types.DocKgRelation) error
	ListDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) ([]*types.DocKgRelation, error)
	ListDocKgRelationsByTarget(ctx context.Context, targetType, targetID string) ([]*types.DocKgRelation, error)
	DeleteDocKgRelationsBySource(ctx context.Context, sourceType, sourceID string) error

	// KB → wiki reverse references.
	UpsertKbWikiReference(ctx context.Context, r *types.KbWikiReference) error
	ListKbWikiReferencesByChunk(ctx context.Context, kbChunkID string) ([]*types.KbWikiReference, error)
	ListKbWikiReferencesByPage(ctx context.Context, wikiPageID string) ([]*types.KbWikiReference, error)
	DeleteKbWikiReferencesByPage(ctx context.Context, wikiPageID string) error

	// Inline KB citations.
	UpsertInlineKBRef(ctx context.Context, r *types.InlineKBRef) error
	ListInlineKBRefsByPage(ctx context.Context, wikiPageID string) ([]*types.InlineKBRef, error)
	DeleteInlineKBRefsByPage(ctx context.Context, wikiPageID string) error
}
