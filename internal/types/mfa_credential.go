package types

import (
	"time"

	"gorm.io/gorm"
)

// MFACredredential stores a per-user MFA enrolment. One row per
// enrolled factor (a user can have TOTP + WebAuthn in the future).
// Today only TOTP is supported; the schema leaves room for
// WebAuthn without a migration.
//
// SecretHash stores the base32 TOTP secret encrypted at rest by
// the repository layer (envelope encryption via the tenant secret
// KMS). The plaintext is never written here. RecoveryCodes is a
// JSON array of SHA-256 hex hashes — same shape we use for the
// SCIM bearer-token hashes.
//
// LastUsedCounter tracks the highest TOTP counter this user has
// successfully verified, so we can reject replay attempts even
// within the drift window.
type MFACredential struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          string         `gorm:"type:varchar(36);not null;index" json:"user_id"`
	Type            string         `gorm:"size:32;not null" json:"type"`              // "totp" | "webauthn"
	SecretHash      string         `gorm:"type:text;not null" json:"-"`               // encrypted
	RecoveryCodes   StringList     `gorm:"type:jsonb;not null;default:'[]'" json:"-"` // hashed
	Name            string         `gorm:"size:64;not null" json:"name"`              // user-set label, e.g. "iPhone"
	Enabled         bool           `gorm:"not null;default:true" json:"enabled"`
	LastUsedCounter int64          `gorm:"not null;default:0" json:"last_used_counter"`
	LastUsedAt      *time.Time     `json:"last_used_at,omitempty"`
	EnrolledAt      time.Time      `gorm:"not null" json:"enrolled_at"`
	RevokedAt       *time.Time     `json:"revoked_at,omitempty"`
	CreatedAt       time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name.
func (MFACredential) TableName() string { return "user_mfa_credentials" }

// MFAEnrollRequest is the body for POST /api/v1/mfa/enroll.
type MFAEnrollRequest struct {
	Name string `json:"name" binding:"required,max=64"`
}

// MFAEnrollResponse is returned once at enrollment. The
// ProvisioningURI is what the user scans with their authenticator
// app; the RecoveryCodes are shown verbatim and never again.
type MFAEnrollResponse struct {
	ID              uint64   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	ProvisioningURI string   `json:"provisioning_uri"`
	RecoveryCodes   []string `json:"recovery_codes"`
}

// MFAVerifyRequest is the body for POST /api/v1/mfa/verify.
type MFAVerifyRequest struct {
	CredentialID uint64 `json:"credential_id"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}
