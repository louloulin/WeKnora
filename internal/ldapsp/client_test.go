package ldapsp

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

// fakeConn is the test double for Conn. It records every Bind /
// Search / Close so assertions can check the call shape without
// bringing up a real slapd.
type fakeConn struct {
	mu sync.Mutex

	bindCalls     []bindCall
	searchResults []*ldap.SearchResult
	searchCalls   []*ldap.SearchRequest
	searchErr     error
	bindErr       error
	closed        bool
}

type bindCall struct{ User, Password string }

func (f *fakeConn) Bind(u, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bindCalls = append(f.bindCalls, bindCall{u, p})
	return f.bindErr
}

func (f *fakeConn) Search(r *ldap.SearchRequest) (*ldap.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls = append(f.searchCalls, r)
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	if len(f.searchResults) == 0 {
		return &ldap.SearchResult{Entries: nil}, nil
	}
	r0 := f.searchResults[0]
	f.searchResults = f.searchResults[1:]
	return r0, nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func validConfig() *LDAPConfig {
	return &LDAPConfig{
		TenantID:     1,
		Name:         "Corporate AD",
		Host:         "ldap.corp.example.com",
		Port:         389,
		UseTLS:       false,
		BindDN:       "CN=svc,DC=corp,DC=example,DC=com",
		BindPassword: "redacted",
		BaseDN:       "DC=corp,DC=example,DC=com",
		Vendor:       "ad",
		Enabled:      true,
	}
}

func TestLDAPConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*LDAPConfig)
		want bool
	}{
		{"valid", func(*LDAPConfig) {}, true},
		{"missing tenant", func(c *LDAPConfig) { c.TenantID = 0 }, false},
		{"missing name", func(c *LDAPConfig) { c.Name = "" }, false},
		{"missing host", func(c *LDAPConfig) { c.Host = "" }, false},
		{"missing port", func(c *LDAPConfig) { c.Port = 0 }, false},
		{"bad port", func(c *LDAPConfig) { c.Port = 70000 }, false},
		{"missing bind dn", func(c *LDAPConfig) { c.BindDN = "" }, false},
		{"missing base dn", func(c *LDAPConfig) { c.BaseDN = "" }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.mut(c)
			err := c.Validate()
			got := err == nil
			if got != tc.want {
				t.Fatalf("Validate() ok=%v want=%v err=%v", got, tc.want, err)
			}
		})
	}
}

func TestEffectiveFilters(t *testing.T) {
	c := validConfig()
	if got := c.EffectiveUsernameAttr(); got != "sAMAccountName" {
		t.Fatalf("AD vendor should map to sAMAccountName, got %q", got)
	}
	if got := c.EffectiveUserFilter(); !strings.Contains(got, "sAMAccountName=%s") {
		t.Fatalf("default filter should reference sAMAccountName, got %q", got)
	}
	c.Vendor = "openldap"
	if got := c.EffectiveUsernameAttr(); got != "uid" {
		t.Fatalf("OpenLDAP vendor should map to uid, got %q", got)
	}
	c.UserFilter = "(mail=%s)"
	if got := c.EffectiveUserFilter(); got != "(mail=%s)" {
		t.Fatalf("explicit UserFilter should win, got %q", got)
	}
	if c.EffectiveEmailAttr() != "mail" {
		t.Fatalf("default email attr should be mail")
	}
	if c.EffectiveDisplayNameAttr() != "cn" {
		t.Fatalf("default displayName attr should be cn")
	}
}

func TestSearchUserBuildsRequest(t *testing.T) {
	fc := &fakeConn{
		searchResults: []*ldap.SearchResult{{Entries: nil}},
	}
	cfg := validConfig()
	cfg.UsernameAttr = ""
	cfg.Vendor = "ad"
	if _, err := SearchUser(fc, cfg, "alice*"); err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(fc.searchCalls) != 1 {
		t.Fatalf("expected 1 search, got %d", len(fc.searchCalls))
	}
	req := fc.searchCalls[0]
	if req.BaseDN != cfg.BaseDN {
		t.Fatalf("BaseDN: got %q want %q", req.BaseDN, cfg.BaseDN)
	}
	if !strings.Contains(req.Filter, "sAMAccountName=") {
		t.Fatalf("filter should include sAMAccountName, got %q", req.Filter)
	}
	// LDAP filter escape should neutralise the asterisk.
	if strings.Contains(req.Filter, "*alice") && !strings.Contains(req.Filter, `\*`) {
		t.Fatalf("filter did not escape wildcard: %q", req.Filter)
	}
}

func TestSearchGroupsSkippedWhenNoBase(t *testing.T) {
	fc := &fakeConn{}
	cfg := validConfig()
	if cfg.GroupSearchBaseDN != "" {
		t.Fatalf("fixture should not configure group base DN")
	}
	got, err := SearchGroups(fc, cfg, "uid=alice,ou=users")
	if err != nil {
		t.Fatalf("SearchGroups should be a no-op, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil groups, got %v", got)
	}
	if len(fc.searchCalls) != 0 {
		t.Fatalf("SearchGroups should not hit the wire when base DN is empty")
	}
}

func TestAsInvalidCredentialsWraps(t *testing.T) {
	if AsInvalidCredentials(nil) != nil {
		t.Fatalf("nil in must yield nil out")
	}
	ldapErr := &ldap.Error{ResultCode: ldap.LDAPResultInvalidCredentials, Err: errors.New("bad pw")}
	out := AsInvalidCredentials(ldapErr)
	if !errors.Is(out, ErrInvalidCredentials) {
		t.Fatalf("LDAPResultInvalidCredentials must wrap our sentinel, got %v", out)
	}
	other := &ldap.Error{ResultCode: ldap.LDAPResultNoSuchObject}
	if AsInvalidCredentials(other) == nil || errors.Is(other, ErrInvalidCredentials) {
		t.Fatalf("other result codes must NOT wrap our sentinel")
	}
}
