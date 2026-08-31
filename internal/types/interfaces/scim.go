package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SCIMTokenRepository persists per-tenant SCIM bearer tokens. The
// plaintext token never reaches storage; only the SHA-256 hash is
// written. Lookup by hash is the auth hot path.
type SCIMTokenRepository interface {
	Create(ctx context.Context, tok *types.SCIMToken) error
	GetByID(ctx context.Context, id uint64) (*types.SCIMToken, error)
	GetByTokenHash(ctx context.Context, hash string) (*types.SCIMToken, error)
	ListByTenant(ctx context.Context, tenantID uint64) ([]*types.SCIMToken, error)
	Revoke(ctx context.Context, id uint64) error
	Touch(ctx context.Context, id uint64) error
}

// SCIMSyncLogRepository persists one row per SCIM request for
// diagnostics and audit. Write-only from the handler's perspective;
// reads are admin-side.
type SCIMSyncLogRepository interface {
	Create(ctx context.Context, entry *types.SCIMSyncLog) error
	ListByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.SCIMSyncLog, error)
}
