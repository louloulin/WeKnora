package types

import (
	"time"

	"gorm.io/gorm"
)

// LDAPConfig is the per-tenant directory server descriptor. One row
// per tenant — the service layer rejects creating a second one for
// the same tenant_id. The bind_password is encrypted at rest in the
// repository implementation; here it is plaintext-friendly so the
// type can be reused in tests and requests.
type LDAPConfig struct {
	ID                uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID          uint64         `gorm:"not null;uniqueIndex:uniq_ldap_tenant" json:"tenant_id"`
	Name              string         `gorm:"size:128;not null" json:"name"`
	Host              string         `gorm:"size:255;not null" json:"host"`
	Port              int            `gorm:"not null;default:389" json:"port"`
	UseTLS            bool           `gorm:"not null;default:false" json:"use_tls"`
	SkipVerify        bool           `gorm:"not null;default:false" json:"skip_verify"`
	BindDN            string         `gorm:"size:512;not null" json:"bind_dn"`
	BindPassword      string         `gorm:"type:text;not null" json:"-"` // encrypted at rest
	BaseDN            string         `gorm:"size:1024;not null" json:"base_dn"`
	UserFilter        string         `gorm:"size:512" json:"user_filter,omitempty"`
	UsernameAttr      string         `gorm:"size:64" json:"username_attr,omitempty"`
	EmailAttr         string         `gorm:"size:64" json:"email_attr,omitempty"`
	DisplayNameAttr   string         `gorm:"size:64" json:"display_name_attr,omitempty"`
	GroupAttr         string         `gorm:"size:64" json:"group_attr,omitempty"`
	GroupSearchBaseDN string         `gorm:"size:1024" json:"group_search_base_dn,omitempty"`
	GroupFilter       string         `gorm:"size:512" json:"group_filter,omitempty"`
	Vendor            string         `gorm:"size:32" json:"vendor,omitempty"`
	Enabled           bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedAt         time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name so GORM does not pluralize it.
func (LDAPConfig) TableName() string { return "ldap_configs" }

// LDAPConfigCreateRequest is the typed body for POST /ldap/configs.
type LDAPConfigCreateRequest struct {
	Name              string `json:"name"               binding:"required,max=128"`
	Host              string `json:"host"               binding:"required,max=255"`
	Port              int    `json:"port"               binding:"required,min=1,max=65535"`
	UseTLS            bool   `json:"use_tls"`
	SkipVerify        bool   `json:"skip_verify"`
	BindDN            string `json:"bind_dn"            binding:"required,max=512"`
	BindPassword      string `json:"bind_password"      binding:"required,min=1"`
	BaseDN            string `json:"base_dn"            binding:"required,max=1024"`
	UserFilter        string `json:"user_filter"        binding:"omitempty,max=512"`
	UsernameAttr      string `json:"username_attr"      binding:"omitempty,max=64"`
	EmailAttr         string `json:"email_attr"         binding:"omitempty,max=64"`
	DisplayNameAttr   string `json:"display_name_attr"  binding:"omitempty,max=64"`
	GroupAttr         string `json:"group_attr"         binding:"omitempty,max=64"`
	GroupSearchBaseDN string `json:"group_search_base_dn" binding:"omitempty,max=1024"`
	GroupFilter       string `json:"group_filter"       binding:"omitempty,max=512"`
	Vendor            string `json:"vendor"             binding:"omitempty,oneof=ad openldap auto"`
	Enabled           *bool  `json:"enabled"`
}

// LDAPConfigUpdateRequest is the typed body for PUT /ldap/configs/:id.
type LDAPConfigUpdateRequest struct {
	Name              *string `json:"name"`
	Host              *string `json:"host"`
	Port              *int    `json:"port"`
	UseTLS            *bool   `json:"use_tls"`
	SkipVerify        *bool   `json:"skip_verify"`
	BindDN            *string `json:"bind_dn"`
	BindPassword      *string `json:"bind_password"`
	BaseDN            *string `json:"base_dn"`
	UserFilter        *string `json:"user_filter"`
	UsernameAttr      *string `json:"username_attr"`
	EmailAttr         *string `json:"email_attr"`
	DisplayNameAttr   *string `json:"display_name_attr"`
	GroupAttr         *string `json:"group_attr"`
	GroupSearchBaseDN *string `json:"group_search_base_dn"`
	GroupFilter       *string `json:"group_filter"`
	Vendor            *string `json:"vendor"`
	Enabled           *bool   `json:"enabled"`
}
