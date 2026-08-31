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

// ErrSAMLIdentityNotFound is returned when the (IdP, NameID) lookup
// does not match any row. Handlers convert this into a 401/403 so the
// ACS path can distinguish "no binding yet" from "DB error".
var ErrSAMLIdentityNotFound = errors.New("saml identity not found")

// ErrSAMLIdentityRevoked is returned when the binding exists but has
// been revoked. It is a distinct sentinel so the ACS handler can
// surface a different error message ("your federation was revoked;
// contact your admin") rather than the generic not-found message.
var ErrSAMLIdentityRevoked = errors.New("saml identity has been revoked")

type samlIdentityRepository struct {
	db *gorm.DB
}

// NewSAMLIdentityRepository constructs the repository.
func NewSAMLIdentityRepository(db *gorm.DB) interfaces.SAMLIdentityRepository {
	return &samlIdentityRepository{db: db}
}

// GetByIdPAndNameID looks up the binding row by the (IdPEntityID,
// NameID) composite. The unique index uniq_saml_fed on
// (idp_entity_id, name_id) makes this an index-only lookup.
func (r *samlIdentityRepository) GetByIdPAndNameID(
	ctx context.Context,
	idpEntityID, nameID string,
) (*types.SAMLFederationIdentity, error) {
	var identity types.SAMLFederationIdentity
	err := r.db.WithContext(ctx).
		Where("idp_entity_id = ? AND name_id = ?", idpEntityID, nameID).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSAMLIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	if identity.RevokedAt != nil {
		// Surface the revoked sentinel so the caller can give the
		// end-user a meaningful error message instead of "we don't
		// recognise this assertion".
		return &identity, ErrSAMLIdentityRevoked
	}
	return &identity, nil
}

// Create persists a new binding. ID is auto-assigned when the caller
// left it blank.
func (r *samlIdentityRepository) Create(ctx context.Context, identity *types.SAMLFederationIdentity) error {
	if identity == nil {
		return errors.New("saml identity is nil")
	}
	if identity.ID == "" {
		identity.ID = uuid.NewString()
	}
	now := time.Now()
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = now
	}
	identity.UpdatedAt = now
	if identity.LastLoginAt.IsZero() {
		identity.LastLoginAt = now
	}
	return r.db.WithContext(ctx).Create(identity).Error
}

// Touch refreshes the last-login snapshot. The revoked-row guard
// turns a no-op update into ErrSAMLIdentityNotFound so the ACS handler
// can fail-closed on a race between Revoke and Touch.
func (r *samlIdentityRepository) Touch(ctx context.Context, id, email, displayName string) error {
	updates := map[string]interface{}{
		"email_at_last_login": email,
		"display_name":        displayName,
		"last_login_at":       time.Now(),
		"updated_at":          time.Now(),
	}
	result := r.db.WithContext(ctx).Model(&types.SAMLFederationIdentity{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrSAMLIdentityNotFound
	}
	return nil
}

// Revoke marks the binding as revoked. Idempotent: re-revoking an
// already-revoked row is a no-op (RowsAffected=0 but no error).
func (r *samlIdentityRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&types.SAMLFederationIdentity{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]interface{}{
			"revoked_at": &now,
			"updated_at": now,
		})
	return result.Error
}

// ListByUser returns every binding for a user. The composite index
// on (user_id, tenant_id) keeps this efficient even for users with
// many federated IdPs.
func (r *samlIdentityRepository) ListByUser(ctx context.Context, userID string) ([]*types.SAMLFederationIdentity, error) {
	var identities []*types.SAMLFederationIdentity
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&identities).Error
	if err != nil {
		return nil, err
	}
	return identities, nil
}
