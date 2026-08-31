package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// MFACredentialRepository persists per-user MFA enrolments. The
// SecretHash is stored encrypted at rest; the repository is the
// layer responsible for envelope encryption (mirrors the SAML /
// LDAP credential repos).
type MFACredentialRepository interface {
	Create(ctx context.Context, cred *types.MFACredential) error
	GetByID(ctx context.Context, id uint64) (*types.MFACredential, error)
	GetByUserID(ctx context.Context, userID string) ([]*types.MFACredential, error)
	// Update applies in-place mutations: enabling / disabling,
	// touching LastUsedCounter / LastUsedAt, removing a used
	// recovery code.
	Update(ctx context.Context, cred *types.MFACredential) error
	Delete(ctx context.Context, id uint64) error
}
