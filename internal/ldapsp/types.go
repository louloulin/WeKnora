package ldapsp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// LDAPConfig describes how WeKnora reaches one tenant's directory
// server. A tenant can have at most one active LDAPConfig (enforced
// at the service layer, same constraint as SAMLIdPConfig).
//
// The bind credentials are stored encrypted at rest (envelope
// encryption via the tenant secret KMS). Encryption happens in the
// repository, not here — this struct stays plaintext-friendly so it
// can be unit-tested without KMS fixtures.
type LDAPConfig struct {
	// ID is the primary key. Zero means "new".
	ID uint64

	// TenantID is the owning tenant.
	TenantID uint64

	// Name is the human label shown in the admin UI ("Corporate AD").
	Name string

	// Host is the LDAP server hostname or IP. Do not include the
	// scheme; we add ldap:// or ldaps:// based on UseTLS.
	Host string

	// Port is the LDAP server port (389 plain, 636 LDAPS, 3268/3269
	// for AD Global Catalog).
	Port int

	// UseTLS toggles LDAPS (StartTLS on the plain port, or full TLS
	// on the LDAPS port). Production deployments MUST set it.
	UseTLS bool

	// SkipVerify disables certificate chain validation. Only useful
	// for development with self-signed certs; never in production.
	SkipVerify bool

	// BindDN is the service-account DN used for the initial search
	// (e.g. "CN=svc-weknora,OU=Service,DC=corp,DC=example,DC=com").
	BindDN string

	// BindPassword is the service-account password.
	BindPassword string

	// BaseDN is the search root for user lookups.
	BaseDN string

	// UserFilter is the LDAP filter used to find a user by the
	// submitted username. The literal "%s" is replaced by the
	// escaped username. The default if empty is
	// "(sAMAccountName=%s)" for AD or "(uid=%s)" for OpenLDAP; the
	// service layer picks the right one based on Vendor.
	UserFilter string

	// UsernameAttr is the attribute the login form puts into the
	// filter. Defaults to "sAMAccountName" (AD) or "uid"
	// (OpenLDAP); see Vendor.
	UsernameAttr string

	// EmailAttr is the attribute that carries the user's email
	// (typically "mail").
	EmailAttr string

	// DisplayNameAttr is the attribute that carries the user's
	// display name (typically "displayName" or "cn").
	DisplayNameAttr string

	// GroupAttr is the attribute on the user entry that holds
	// group memberships (typically "memberOf"). Optional — only
	// required when GroupSearchBaseDN is set.
	GroupAttr string

	// GroupSearchBaseDN lets us resolve groups by their DN
	// (e.g. "OU=Groups,DC=corp,DC=example,DC=com"). When set, after
	// a successful user bind we issue a follow-up search under this
	// base and return the matched group DNs alongside the user.
	GroupSearchBaseDN string

	// GroupFilter is the filter applied under GroupSearchBaseDN.
	// "%s" is replaced with the user DN; the default when empty is
	// "(member=%s)".
	GroupFilter string

	// Vendor hints at a default schema so operators do not have to
	// spell out every attribute for the common case. One of
	// "auto" (no hint), "ad" (Active Directory), "openldap".
	Vendor string

	// Enabled lets admins soft-disable a config without losing the
	// federation rows already bound to it.
	Enabled bool
}

// Address returns "host:port" with no scheme; helpful when
// building go-ldap URIs.
func (c *LDAPConfig) Address() string {
	if c.Host == "" {
		return ""
	}
	if c.Port == 0 {
		return c.Host
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// EffectiveUserFilter picks the right default if UserFilter is
// empty, applying the Vendor hint.
func (c *LDAPConfig) EffectiveUserFilter() string {
	if c.UserFilter != "" {
		return c.UserFilter
	}
	attr := c.EffectiveUsernameAttr()
	return fmt.Sprintf("(%s=%%s)", attr)
}

// EffectiveUsernameAttr resolves the search attribute from Vendor.
func (c *LDAPConfig) EffectiveUsernameAttr() string {
	if c.UsernameAttr != "" {
		return c.UsernameAttr
	}
	switch strings.ToLower(c.Vendor) {
	case "ad", "active-directory", "active_directory":
		return "sAMAccountName"
	default:
		return "uid"
	}
}

// EffectiveEmailAttr returns the attribute holding the user email.
func (c *LDAPConfig) EffectiveEmailAttr() string {
	if c.EmailAttr != "" {
		return c.EmailAttr
	}
	return "mail"
}

// EffectiveDisplayNameAttr returns the attribute holding the display
// name; falls back to "cn" if unset because every directory ships cn.
func (c *LDAPConfig) EffectiveDisplayNameAttr() string {
	if c.DisplayNameAttr != "" {
		return c.DisplayNameAttr
	}
	return "cn"
}

// EffectiveGroupFilter picks the default group filter.
func (c *LDAPConfig) EffectiveGroupFilter() string {
	if c.GroupFilter != "" {
		return c.GroupFilter
	}
	if c.GroupAttr != "" {
		return fmt.Sprintf("(%s=%%s)", c.GroupAttr)
	}
	return "(member=%s)"
}

// Validate enforces the invariants the service layer relies on.
func (c *LDAPConfig) Validate() error {
	if c.TenantID == 0 {
		return errors.New("ldapsp: TenantID is required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("ldapsp: Name is required")
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("ldapsp: Host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("ldapsp: Port must be in (0, 65535]; got %d", c.Port)
	}
	if strings.TrimSpace(c.BindDN) == "" {
		return errors.New("ldapsp: BindDN is required")
	}
	if strings.TrimSpace(c.BaseDN) == "" {
		return errors.New("ldapsp: BaseDN is required")
	}
	if _, err := url.Parse(c.Address()); err != nil {
		return fmt.Errorf("ldapsp: address parse: %w", err)
	}
	return nil
}

// TLSConfig returns the tls.Config the client should use when
// UseTLS is true. Centralised so tests can construct a matching one
// against a test server's certificate.
func (c *LDAPConfig) TLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: c.SkipVerify,
		ServerName:         c.Host,
		MinVersion:         tls.VersionTLS12,
	}
}
