package types

import (
	"time"

	"gorm.io/gorm"
)

// LDAPFederationIdentity binds a directory entry to a WeKnora user.
// The pair (TenantID + LDAPConfigID + EntryDN) is unique — we look up
// by it on every login to find the local user to mint JWTs for.
//
// EntryDN is the canonical anchor: even if a directory rename moves
// a user between OUs, the DN stays stable. For directories that
// rewrite DNs (rare; some AD migration tools), operators can switch
// to EntryUUID by populating it from entryUUID; the (ConfigID, DN)
// uniqueness is still the lookup key so a re-bind with a new DN
// would create a second row rather than collide.
//
// Revoked is the soft-delete signal; once revoked the user must log
// in via a different federation (or password) to come back.
type LDAPFederationIdentity struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     uint64         `gorm:"not null;uniqueIndex:uniq_ldap_fed,priority:1" json:"tenant_id"`
	LDAPConfigID uint64         `gorm:"not null;uniqueIndex:uniq_ldap_fed,priority:2" json:"ldap_config_id"`
	EntryDN      string         `gorm:"size:1024;not null;uniqueIndex:uniq_ldap_fed,priority:3" json:"entry_dn"`
	EntryUUID    string         `gorm:"size:128;index" json:"entry_uuid,omitempty"`
	UserID       string         `gorm:"type:varchar(36);not null;index" json:"user_id"`
	Username     string         `gorm:"size:256;not null" json:"username"`
	Email        string         `gorm:"size:255;index" json:"email,omitempty"`
	Revoked      bool           `gorm:"not null;default:false" json:"revoked"`
	LastLoginAt  *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt    time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name so GORM does not pluralize it.
func (LDAPFederationIdentity) TableName() string { return "user_ldap_federation_identities" }

// LDAPIdentityInfo is the typed payload LoginWithLDAPCredentials
// needs. It is intentionally decoupled from the go-ldap types so the
// handler does not leak the package boundary into other layers.
type LDAPIdentityInfo struct {
	// LDAPConfigID identifies which directory the user was
	// authenticated against. Combined with EntryDN it forms the
	// unique lookup key for the federation identity row.
	LDAPConfigID uint64

	// EntryDN is the LDAP distinguished name of the user.
	EntryDN string

	// EntryUUID is the directory's entryUUID; populated when the
	// server returns it but not required.
	EntryUUID string

	// Username is the value the user typed into the login form.
	Username string

	// Email is read from the entry; required for JIT provisioning.
	Email string

	// DisplayName is read from the entry; used as the default
	// display name when JIT provisioning.
	DisplayName string

	// GroupDNs is the set of group DNs the user belongs to;
	// optional — only populated when GroupSearchBaseDN is set.
	GroupDNs []string
}
