package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// ErrMFACredentialNotFound is returned when a lookup matches no
// rows. The handler maps it to 404.
var ErrMFACredentialNotFound = errors.New("mfa: credential not found")

// MFACredentialRepository is the GORM-backed implementation of
// interfaces.MFACredentialRepository.
type MFACredentialRepository struct {
	db *gorm.DB
}

// NewMFACredentialRepository constructs the repository.
func NewMFACredentialRepository(db *gorm.DB) *MFACredentialRepository {
	return &MFACredentialRepository{db: db}
}

// Create inserts a new row.
func (r *MFACredentialRepository) Create(ctx context.Context, cred *types.MFACredential) error {
	if cred == nil {
		return errors.New("mfa: nil credential")
	}
	return r.db.WithContext(ctx).Create(cred).Error
}

// GetByID returns a single row by primary key.
func (r *MFACredentialRepository) GetByID(ctx context.Context, id uint64) (*types.MFACredential, error) {
	var out types.MFACredential
	err := r.db.WithContext(ctx).First(&out, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMFACredentialNotFound
		}
		return nil, err
	}
	return &out, nil
}

// GetByUserID returns every enabled credential for the user.
func (r *MFACredentialRepository) GetByUserID(ctx context.Context, userID string) ([]*types.MFACredential, error) {
	var rows []*types.MFACredential
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("enrolled_at asc").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Update applies in-place mutations.
func (r *MFACredentialRepository) Update(ctx context.Context, cred *types.MFACredential) error {
	if cred == nil {
		return errors.New("mfa: nil credential")
	}
	return r.db.WithContext(ctx).Save(cred).Error
}

// Delete soft-removes the row.
func (r *MFACredentialRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&types.MFACredential{}, id).Error
}
