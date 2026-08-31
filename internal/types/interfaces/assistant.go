package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AssistantConversationRepository is the persistence contract for
// AI Assistant conversation history. The hot write path is Create
// (one row per Ask turn); the read paths are ListByTenant (admin
// audit) and ListByConversation (follow-up UI).
type AssistantConversationRepository interface {
	// Create inserts one assistant conversation row. The handler
	// calls call after a successful Ask so the audit log retains
	// the query + citations even if the LLM answer drifts.
	Create(ctx context.Context, c *types.AssistantConversation) error

	// ListByTenant returns the most recent N conversations for the
	// tenant in created_at DESC order. limit > 0 is honoured; a
	// zero limit means "no cap" (rare, mostly tests).
	ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*types.AssistantConversation, int64, error)

	// ListByConversation returns all turns for a single
	// conversation_id, ordered by created_at ASC. Used by the
	// follow-up UI to render the thread.
	ListByConversation(ctx context.Context, conversationID string) ([]*types.AssistantConversation, error)
}

// KBRetriever is the minimum contract the AssistantService needs to
// run a KB-side search. The real implementation is
// KnowledgeService.SearchKnowledgeForScopes; we keep the interface
// minimal so the assistant can be unit-tested with a fake without
// pulling in the entire chat_pipeline.
type KBRetriever interface {
	SearchKnowledgeForScopes(
		ctx context.Context,
		scopes []types.KnowledgeSearchScope,
		keyword string,
		offset, limit int,
		fileTypes []string,
	) ([]*types.Knowledge, bool, int64, error)
}

// WikiRetriever is the minimum contract the AssistantService needs
// to run a Wiki-side search. The real implementation is
// WikiSearchV2Service.Search.
type WikiRetriever interface {
	Search(
		ctx context.Context,
		tenantID uint64,
		userID string,
		req types.WikiSearchV2Request,
		visibleKBIDs []string,
	) (types.WikiSearchV2Result, error)
}
