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

// ErrAuthZTupleNotFound is the storage-layer sentinel for "no such
// tuple". Handlers convert this into 404; the authz.Checker treats
// it as a normal miss (no relation).
var ErrAuthZTupleNotFound = errors.New("authz tuple not found")

// ErrAuthZTupleAlreadyExists signals a duplicate tuple insert. The
// composite unique index enforces the invariant; we surface a typed
// sentinel so the admin handler can map to 409 Conflict.
var ErrAuthZTupleAlreadyExists = errors.New("authz tuple already exists")

type authzTupleRepository struct {
	db *gorm.DB
}

// NewAuthZTupleRepository constructs the repository.
func NewAuthZTupleRepository(db *gorm.DB) interfaces.AuthZTupleRepository {
	return &authzTupleRepository{db: db}
}

// Get returns the tuple by id.
func (r *authzTupleRepository) Get(ctx context.Context, id string) (*types.AuthZTuple, error) {
	var t types.AuthZTuple
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAuthZTupleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List returns tuples matching the filter. Empty filter fields are
// wildcards. Results are ordered by created_at desc so admin UIs
// can render "most recent grants first" without a second query.
func (r *authzTupleRepository) List(ctx context.Context, filter types.AuthZTupleListFilter) ([]*types.AuthZTuple, error) {
	q := r.db.WithContext(ctx).Model(&types.AuthZTuple{})
	if filter.ObjectType != "" {
		q = q.Where("object_type = ?", filter.ObjectType)
	}
	if filter.ObjectID != "" {
		q = q.Where("object_id = ?", filter.ObjectID)
	}
	if filter.SubjectType != "" {
		q = q.Where("subject_type = ?", filter.SubjectType)
	}
	if filter.SubjectID != "" {
		q = q.Where("subject_id = ?", filter.SubjectID)
	}
	if filter.Relation != "" {
		q = q.Where("relation = ?", filter.Relation)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	var out []*types.AuthZTuple
	if err := q.Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Create persists a new tuple. ID is auto-assigned when blank.
func (r *authzTupleRepository) Create(ctx context.Context, t *types.AuthZTuple) error {
	if t == nil {
		return errors.New("authz tuple is nil")
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	err := r.db.WithContext(ctx).Create(t).Error
	if err != nil && isUniqueViolation(err) {
		return ErrAuthZTupleAlreadyExists
	}
	return err
}

// Revoke marks the tuple revoked. Idempotent — re-revoking a
// already-revoked row returns nil (the change is a no-op).
func (r *authzTupleRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&types.AuthZTuple{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", &now)
	return res.Error
}

// LookupObjectRelations returns the active (non-revoked, non-expired)
// tuples for an object. The authz.Checker uses this as the input to
// its tuple-store adapter.
func (r *authzTupleRepository) LookupObjectRelations(ctx context.Context, objectType, objectID string) ([]*types.AuthZTuple, error) {
	now := time.Now()
	var out []*types.AuthZTuple
	err := r.db.WithContext(ctx).
		Where("object_type = ? AND object_id = ?", objectType, objectID).
		Where("revoked_at IS NULL").
		Where("expires_at IS NULL OR expires_at > ?", now).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LookupSubjectRelations returns active tuples for a subject.
// Useful for "what does this user have access to?" admin views.
func (r *authzTupleRepository) LookupSubjectRelations(ctx context.Context, subjectType, subjectID string) ([]*types.AuthZTuple, error) {
	now := time.Now()
	var out []*types.AuthZTuple
	err := r.db.WithContext(ctx).
		Where("subject_type = ? AND subject_id = ?", subjectType, subjectID).
		Where("revoked_at IS NULL").
		Where("expires_at IS NULL OR expires_at > ?", now).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isUniqueViolation matches the GORM "duplicate key" error across
// the SQLite and Postgres dialects so the Create path can convert
// it into a typed sentinel.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Postgres: "duplicate key value violates unique constraint"
	// SQLite:  "UNIQUE constraint failed: authz_tuples.xxx"
	return (containsCI(msg, "unique constraint") ||
		containsCI(msg, "duplicate key"))
}

func containsCI(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
