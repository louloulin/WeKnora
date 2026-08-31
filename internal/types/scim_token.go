package types

import (
	"time"

	"gorm.io/gorm"
)

// SCIMToken is the per-tenant bearer token used by SCIM 2.0
// provisioning clients (Okta, Azure AD, OneLogin, JumpCloud). One
// row per tenant. The token value is hashed at rest (SHA-256); the
// plaintext is shown to the operator exactly once at creation time.
//
// Token storage is hashed so a database leak cannot be replayed
// against the SCIM endpoints. The IdP is responsible for refreshing
// its own credential; we expose Revoked + ExpiresAt for revocation
// flow but do not auto-expire active tokens.
type SCIMToken struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    uint64         `gorm:"not null;uniqueIndex:uniq_scim_token_tenant" json:"tenant_id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	TokenHash   string         `gorm:"size:128;not null" json:"-"`           // SHA-256 hex
	TokenPrefix string         `gorm:"size:16;not null" json:"token_prefix"` // first 8 chars for display
	CreatedBy   string         `gorm:"type:varchar(36);not null" json:"created_by"`
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	Revoked     bool           `gorm:"not null;default:false" json:"revoked"`
	RevokedAt   *time.Time     `json:"revoked_at,omitempty"`
	CreatedAt   time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name.
func (SCIMToken) TableName() string { return "scim_tokens" }

// SCIMTokenCreateRequest is the body for POST /scim/admin/tokens.
type SCIMTokenCreateRequest struct {
	Name      string     `json:"name"       binding:"required,max=128"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// SCIMTokenCreateResponse is returned once at creation. Token is the
// plaintext; the caller must store it because we never store the
// plaintext ourselves.
type SCIMTokenCreateResponse struct {
	ID          uint64     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token"`
	TokenPrefix string     `json:"token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
