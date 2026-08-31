package types

import (
	"time"
)

// AuthZTuple is the persistent row for an OpenFGA-style
// "object#relation@subject" relationship. The schema is wide enough
// to express both direct grants (subject_relation="") and
// computed grants via group membership (subject_relation="member").
//
// Example rows:
//
//	kb:42 / viewer          user:abc  / ""           → "abc can view kb 42"
//	kb:42 / editor          group:eng / member       → "any eng member can edit kb 42"
//	agent:7 / viewer        user:abc  / ""           → "abc can use agent 7"
//
// The composite unique index on
// (object_type, object_id, relation, subject_type, subject_id, subject_relation)
// makes "does this tuple exist?" an index-only lookup.
type AuthZTuple struct {
	ID              string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64     `json:"tenant_id" gorm:"not null;index"`
	ObjectType      string     `json:"object_type" gorm:"type:varchar(64);not null;index:uniq_authz_tuple,priority:1"`
	ObjectID        string     `json:"object_id" gorm:"type:varchar(64);not null;index:uniq_authz_tuple,priority:2"`
	Relation        string     `json:"relation" gorm:"type:varchar(32);not null;index:uniq_authz_tuple,priority:3"`
	SubjectType     string     `json:"subject_type" gorm:"type:varchar(32);not null;index:uniq_authz_tuple,priority:4"`
	SubjectID       string     `json:"subject_id" gorm:"type:varchar(64);not null;index:uniq_authz_tuple,priority:5"`
	SubjectRelation string     `json:"subject_relation,omitempty" gorm:"type:varchar(32);not null;default:'';index:uniq_authz_tuple,priority:6"`
	GrantedBy       string     `json:"granted_by" gorm:"type:varchar(36);not null;default:''"`
	CreatedAt       time.Time  `json:"created_at" gorm:"not null"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty" gorm:"index"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty" gorm:"index"`
}

// TableName pins the table name so GORM does not pluralize it.
func (AuthZTuple) TableName() string { return "authz_tuples" }

// IsActive reports whether the tuple is currently in force. A
// revoked or expired tuple is treated as absent by the lookup path.
func (t *AuthZTuple) IsActive(now time.Time) bool {
	if t == nil {
		return false
	}
	if t.RevokedAt != nil && !t.RevokedAt.After(now) {
		return false
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return false
	}
	return true
}

// AuthZTupleCreateRequest is the typed body for POST /authz/tuples.
// TenantID is server-controlled — clients cannot assign their own
// tenancy to a tuple.
type AuthZTupleCreateRequest struct {
	ObjectType      string `json:"object_type"      binding:"required,max=64"`
	ObjectID        string `json:"object_id"        binding:"required,max=64"`
	Relation        string `json:"relation"         binding:"required,max=32"`
	SubjectType     string `json:"subject_type"     binding:"required,oneof=user group api_key agent"`
	SubjectID       string `json:"subject_id"       binding:"required,max=64"`
	SubjectRelation string `json:"subject_relation" binding:"omitempty,max=32"`
	// ExpiresAt is optional; nil means "indefinite". Set by an admin
	// to bound a temporary share (e.g. an auditor who needs access
	// for a single quarter).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AuthZTupleListFilter scopes the list endpoint. Empty fields are
// wildcards. The composite indexes make every supported combo an
// index lookup.
type AuthZTupleListFilter struct {
	ObjectType  string
	ObjectID    string
	SubjectType string
	SubjectID   string
	Relation    string
	Limit       int
	Offset      int
}

// AuthZCheckRequest is the body for POST /authz/check — the admin
// debug endpoint that returns the same Decision the runtime guard
// would produce. Lets admins answer "why is this 403 happening?"
// without diving into the source code.
type AuthZCheckRequest struct {
	UserID   string `json:"user_id"   binding:"required"`
	UserType string `json:"user_type,omitempty"`
	Relation string `json:"relation"  binding:"required"`
	Object   struct {
		Type     string `json:"type"     binding:"required"`
		ID       string `json:"id"       binding:"required"`
		TenantID uint64 `json:"tenant_id,omitempty"`
	} `json:"object" binding:"required"`
}
