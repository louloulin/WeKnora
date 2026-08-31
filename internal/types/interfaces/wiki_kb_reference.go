package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiKBReferenceRepository is the persistence contract for the wiki
// page ↔ KB document bidirectional bridge. Implementations are expected
// to use GORM's soft-delete semantics (deleted_at IS NULL for live rows);
// the resolver service translates the tombstone states into the
// ReferenceStatus enum that the renderer consumes.
//
// All methods take ctx so repository implementations can honour call
// deadlines; service-level timeouts come from the request context.
type WikiKBReferenceRepository interface {
	// Upsert inserts or refreshes the reference row identified by
	// (wiki_page_id, knowledge_id). The updated_at timestamp is bumped
	// even if no field changed so that "last backfill" can be audited.
	Upsert(ctx context.Context, ref *types.WikiKBReference) error

	// GetByPair returns the live row for the given pair, or
	// ErrWikiKBReferenceNotFound if no row exists (or has been soft-deleted).
	GetByPair(ctx context.Context, tenantID, wikiPageID, knowledgeID string) (*types.WikiKBReference, error)

	// ListByWikiPage returns every live reference row attached to the
	// given wiki page, ordered by updated_at DESC. Limit/Offset are honoured;
	// a zero Limit means "no cap" (the service layer applies its own default).
	ListByWikiPage(ctx context.Context, tenantID, wikiPageID string, limit, offset int) ([]*types.WikiKBReference, error)

	// ListByKnowledge returns every live reference row that points at
	// the given KB document, ordered by updated_at DESC. This is the
	// primary input for the KB document viewer's "Mentioned in Wiki
	// Pages" section.
	ListByKnowledge(ctx context.Context, tenantID, knowledgeID string, limit, offset int) ([]*types.WikiKBReference, error)

	// SoftDelete marks the pair as deleted (deleted_at = now) but keeps
	// the row for audit. Idempotent — calling it twice is a no-op.
	SoftDelete(ctx context.Context, tenantID, wikiPageID, knowledgeID string) error
}

// Sentinel errors raised by the repository. The service translates these
// into HTTP status codes at the handler boundary.
var (
	ErrWikiKBReferenceNotFound = constErr("wiki_kb_reference_not_found")
)

type constErr string

func (e constErr) Error() string { return string(e) }
