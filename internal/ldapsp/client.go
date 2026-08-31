package ldapsp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// Dialer abstracts go-ldap's connection factory so tests can swap in
// an in-memory fake without bringing up a real slapd.
type Dialer interface {
	Dial(cfg *LDAPConfig) (Conn, error)
}

// Conn is the subset of *ldap.Conn the login flow uses. The interface
// stays narrow on purpose: every method here maps 1:1 to a real LDAP
// operation, so the fake can stay simple.
type Conn interface {
	Bind(username, password string) error
	Search(req *ldap.SearchRequest) (*ldap.SearchResult, error)
	Close() error
}

// DefaultDialer is the production Dialer; it dials via go-ldap.
type DefaultDialer struct{}

// Dial connects to the directory server described by cfg. It opens
// either a plaintext or TLS session depending on cfg.UseTLS. The
// returned connection is bound as the service account so it is ready
// to issue searches immediately.
func (DefaultDialer) Dial(cfg *LDAPConfig) (Conn, error) {
	if cfg == nil {
		return nil, errors.New("ldapsp: nil config")
	}
	addr := cfg.Address()
	if addr == "" {
		return nil, errors.New("ldapsp: empty address")
	}
	var (
		conn *ldap.Conn
		err  error
	)
	if cfg.UseTLS {
		conn, err = ldap.DialTLS("tcp", addr, cfg.TLSConfig())
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("ldapsp: dial %s: %w", addr, err)
	}
	conn.SetTimeout(10 * time.Second)
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldapsp: service bind: %w", err)
		}
	}
	return &realConn{Conn: conn}, nil
}

// realConn wraps *ldap.Conn so we can keep the interface tiny while
// still using the upstream type's TLS upgrade logic.
type realConn struct {
	*ldap.Conn
}

// Close forwards to the underlying connection.
func (c *realConn) Close() error {
	c.Conn.Close()
	return nil
}

// UserEntry is what the service layer needs from a directory
// search: enough to bind as the user, mint JWTs, and store a
// federation row.
type UserEntry struct {
	DN          string
	Username    string
	Email       string
	DisplayName string
	GroupDNs    []string
	RawEntry    *ldap.Entry
}

// SearchUser looks up a single user by username and returns the
// matched entry plus its attributes. It does not bind as the user —
// callers do that separately via Bind so a wrong password yields a
// distinct LDAPInvalidCredentials error from a missing user.
func SearchUser(c Conn, cfg *LDAPConfig, username string) ([]*UserEntry, error) {
	filter := cfg.EffectiveUserFilter()
	filter = strings.ReplaceAll(filter, "%s", ldap.EscapeFilter(username))
	req := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{
			"dn",
			cfg.EffectiveUsernameAttr(),
			cfg.EffectiveEmailAttr(),
			cfg.EffectiveDisplayNameAttr(),
		},
		nil,
	)
	res, err := c.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldapsp: search user: %w", err)
	}
	entries := make([]*UserEntry, 0, len(res.Entries))
	for _, e := range res.Entries {
		entries = append(entries, userEntryFromLDAP(e, cfg))
	}
	return entries, nil
}

// SearchGroups returns the group DNs the given user DN belongs to.
// Returns an empty slice (not an error) when GroupSearchBaseDN is
// empty or no matches are found — group resolution is optional.
func SearchGroups(c Conn, cfg *LDAPConfig, userDN string) ([]string, error) {
	if cfg.GroupSearchBaseDN == "" {
		return nil, nil
	}
	filter := cfg.EffectiveGroupFilter()
	filter = strings.ReplaceAll(filter, "%s", ldap.EscapeFilter(userDN))
	req := ldap.NewSearchRequest(
		cfg.GroupSearchBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{"dn"},
		nil,
	)
	res, err := c.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldapsp: search groups: %w", err)
	}
	out := make([]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, e.DN)
	}
	return out, nil
}

// LDAPInvalidCredentials is returned when a bind fails because of a
// bad password. Re-exported here so the service layer can use
// errors.Is without importing go-ldap.
var ErrInvalidCredentials = errors.New("ldapsp: invalid credentials")

// AsInvalidCredentials wraps an LDAP result code 49 (invalidCredentials)
// in our sentinel so the login flow can distinguish wrong password
// from "server unreachable".
func AsInvalidCredentials(err error) error {
	if err == nil {
		return nil
	}
	var lerr *ldap.Error
	if errors.As(err, &lerr) && lerr.ResultCode == ldap.LDAPResultInvalidCredentials {
		return fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	return err
}

// TLSConfigFor is a convenience for tests and other callers that
// want a copy of the config's tls.Config without poking into the
// struct directly.
func TLSConfigFor(cfg *LDAPConfig) *tls.Config { return cfg.TLSConfig() }

func userEntryFromLDAP(e *ldap.Entry, cfg *LDAPConfig) *UserEntry {
	if e == nil {
		return nil
	}
	attrs := e.Attributes
	get := func(name string) string {
		for _, a := range attrs {
			if strings.EqualFold(a.Name, name) && len(a.Values) > 0 {
				return a.Values[0]
			}
		}
		return ""
	}
	return &UserEntry{
		DN:          e.DN,
		Username:    get(cfg.EffectiveUsernameAttr()),
		Email:       get(cfg.EffectiveEmailAttr()),
		DisplayName: get(cfg.EffectiveDisplayNameAttr()),
		RawEntry:    e,
	}
}
