package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrOIDCIdentityNotFound = errors.New("oidc identity not found")

type oidcIdentityRepository struct {
	db *gorm.DB
}

func NewOIDCIdentityRepository(db *gorm.DB) interfaces.OIDCIdentityRepository {
	return &oidcIdentityRepository{db: db}
}

func (r *oidcIdentityRepository) GetByIssuerSubject(ctx context.Context, issuer, subject string) (*types.OIDCIdentity, error) {
	var identity types.OIDCIdentity
	err := r.db.WithContext(ctx).
		Where("issuer = ? AND subject = ?", issuer, subject).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOIDCIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *oidcIdentityRepository) Create(ctx context.Context, identity *types.OIDCIdentity) error {
	if identity.ID == "" {
		identity.ID = uuid.NewString()
	}
	return r.db.WithContext(ctx).Create(identity).Error
}

func (r *oidcIdentityRepository) Touch(ctx context.Context, id, email string) error {
	result := r.db.WithContext(ctx).Model(&types.OIDCIdentity{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]interface{}{"email_at_last_login": email, "last_login_at": time.Now(), "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOIDCIdentityNotFound
	}
	return nil
}
