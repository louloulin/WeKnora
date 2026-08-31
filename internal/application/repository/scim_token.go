package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrSCIMTokenNotFound is returned by SCIMTokenRepository when a
// lookup matches no rows.
var ErrSCIMTokenNotFound = errors.New("scim: token not found")

// ErrSCIMTokenRevoked signals the token exists but has been
// revoked; the handler translates this to 401.
var ErrSCIMTokenRevoked = errors.New("scim: token is revoked")

// ErrSCIMTokenExpired signals the token exists and is not revoked
// but its ExpiresAt is in the past; the handler translates this
// to 401.
var ErrSCIMTokenExpired = errors.New("scim: token is expired")

// ErrSCIMUserNotFound is returned when a SCIM User lookup matches
// no rows inside the scoped tenant. The handler maps it to 404.
var ErrSCIMUserNotFound = errors.New("scim: user not found")

// ErrSCIMUserAlreadyExists is returned when a SCIM Create request
// collides with an existing user in a different tenant. The
// handler maps it to 409 (RFC 7644 §3.3).
var ErrSCIMUserAlreadyExists = errors.New("scim: user already exists")

// SCIMTokenRepository is the GORM-backed implementation of
// interfaces.SCIMTokenRepository.
type SCIMTokenRepository struct {
	db *gorm.DB
}

// NewSCIMTokenRepository constructs the repository.
func NewSCIMTokenRepository(db *gorm.DB) *SCIMTokenRepository {
	return &SCIMTokenRepository{db: db}
}

// Create inserts a new token row.
func (r *SCIMTokenRepository) Create(ctx context.Context, tok *types.SCIMToken) error {
	if tok == nil {
		return errors.New("scim token: nil")
	}
	return r.db.WithContext(ctx).Create(tok).Error
}

// GetByID loads a token by primary key. Does not enforce revocation
// or expiry — the caller decides what to do with the row.
func (r *SCIMTokenRepository) GetByID(ctx context.Context, id uint64) (*types.SCIMToken, error) {
	var out types.SCIMToken
	err := r.db.WithContext(ctx).First(&out, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSCIMTokenNotFound
		}
		return nil, err
	}
	return &out, nil
}

// GetByTokenHash is the auth hot path. Returns ErrSCIMTokenNotFound
// when no hash matches; the handler maps that to 401.
func (r *SCIMTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*types.SCIMToken, error) {
	var out types.SCIMToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&out).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSCIMTokenNotFound
		}
		return nil, err
	}
	if out.Revoked {
		return nil, ErrSCIMTokenRevoked
	}
	if out.ExpiresAt != nil && out.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrSCIMTokenExpired
	}
	return &out, nil
}

// ListByTenant returns every non-deleted token for a tenant.
func (r *SCIMTokenRepository) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.SCIMToken, error) {
	var rows []*types.SCIMToken
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at desc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Revoke marks the token revoked (soft-delete signal).
func (r *SCIMTokenRepository) Revoke(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&types.SCIMToken{}).
		Where("id = ?", id).
		Updates(map[string]any{"revoked": true, "revoked_at": now}).Error
}

// Touch stamps LastUsedAt so the admin view can spot stale tokens.
func (r *SCIMTokenRepository) Touch(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&types.SCIMToken{}).
		Where("id = ?", id).
		Update("last_used_at", now).Error
}
