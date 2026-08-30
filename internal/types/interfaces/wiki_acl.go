package interfaces

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiAclRepositoryStore is the write/read surface used by WikiAclService.
type WikiAclRepositoryStore interface {
	GetAclBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPageAcl, error)
	UpdateAclWithRevision(ctx context.Context, kbID string, slug string,
		newAcl types.WikiPageAcl, expectedRevision int64, snapshotHash string,
		actorUserID string, actorRole string, action string) (*types.WikiPageAcl, error)
	PageOwnerAndAdmin(ctx context.Context, kbID string, slug string, callerUserID string) (ownerID string, isAdmin bool, err error)
	GroupMembers(ctx context.Context, tenantID uint64, groupIDs []string) ([]string, error)
}

// WikiAclRepository is the complete ACL repository contract used by both
// page ACL decisions and the unified wiki audit feed.
type WikiAclRepository interface {
	WikiAclRepositoryStore
	ListAudit(ctx context.Context, kbID string, since time.Time, page, pageSize int) ([]*types.WikiAclAuditEntry, int64, error)
}
