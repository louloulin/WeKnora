package types

import (
	"time"

	"gorm.io/gorm"
)

// SAMLIdPConfig is the per-tenant Identity Provider descriptor for
// SAML 2.0 single sign-on. Each row is one tenant's IdP; a tenant
// can have at most one active IdP (enforced at the service layer).
//
// The certificate is stored base64-encoded; in production the
// payload is encrypted at rest via the tenant secret KMS (envelope
// encryption) before it lands in this column. The encryption layer
// is applied by the repository implementation, not by this model.
type SAMLIdPConfig struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      uint64         `gorm:"not null;uniqueIndex:uniq_saml_idp_tenant" json:"tenant_id"`
	Name          string         `gorm:"size:128;not null" json:"name"`
	EntityID      string         `gorm:"size:512;not null" json:"entity_id"`
	SSOURL        string         `gorm:"size:1024;not null" json:"sso_url"`
	SLOURL        string         `gorm:"size:1024" json:"slo_url,omitempty"`
	Certificate   string         `gorm:"type:text;not null" json:"certificate"` // base64-encoded DER
	NameIDFormat  string         `gorm:"size:32;not null;default:email" json:"name_id_format"`
	AttributeMap  JSONMap        `gorm:"type:json" json:"attribute_map"`
	Enabled       bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt     time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name so GORM does not pluralize it.
func (SAMLIdPConfig) TableName() string {
	return "saml_idp_configs"
}

// SAMLIdPConfigCreateRequest is the typed body for POST /saml/idps.
type SAMLIdPConfigCreateRequest struct {
	Name         string            `json:"name"          binding:"required,max=128"`
	EntityID     string            `json:"entity_id"     binding:"required,max=512"`
	SSOURL       string            `json:"sso_url"       binding:"required,url,max=1024"`
	SLOURL       string            `json:"slo_url"       binding:"omitempty,url,max=1024"`
	Certificate  string            `json:"certificate"   binding:"required"` // PEM or base64 DER
	NameIDFormat string            `json:"name_id_format" binding:"omitempty,oneof=email persistent transient unspecified"`
	AttributeMap map[string]string `json:"attribute_map"`
	Enabled      *bool             `json:"enabled"`
}

// SAMLIdPConfigUpdateRequest is the typed body for PUT /saml/idps/:id.
type SAMLIdPConfigUpdateRequest struct {
	Name         *string            `json:"name"`
	SSOURL       *string            `json:"sso_url"`
	SLOURL       *string            `json:"slo_url"`
	Certificate  *string            `json:"certificate"`
	NameIDFormat *string            `json:"name_id_format"`
	AttributeMap *map[string]string `json:"attribute_map"`
	Enabled      *bool              `json:"enabled"`
}
