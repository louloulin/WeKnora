package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// AuthZTupleRepository persists OpenFGA-style relation tuples. The
// repository is the only place that talks to the underlying DB;
// the service layer applies validation + audit logging.
type AuthZTupleRepository interface {
	Get(ctx context.Context, id string) (*types.AuthZTuple, error)
	List(ctx context.Context, filter types.AuthZTupleListFilter) ([]*types.AuthZTuple, error)
	Create(ctx context.Context, t *types.AuthZTuple) error
	Revoke(ctx context.Context, id string) error
	// LookupObjectRelations returns active tuples for an object —
	// the input to the tuple-store authz adapter.
	LookupObjectRelations(ctx context.Context, objectType, objectID string) ([]*types.AuthZTuple, error)
	// LookupSubjectRelations returns active tuples for a subject —
	// powers the "what does this user have access to?" admin view.
	LookupSubjectRelations(ctx context.Context, subjectType, subjectID string) ([]*types.AuthZTuple, error)
}
