// Package ldapsp is the low-level LDAP client used by WeKnora to
// authenticate users against an external directory server. It is
// the LDAP counterpart of internal/samlsp/ — that package wraps the
// SAML 2.0 browser-redirect dance; this one wraps the simple-bind
// dance WeKnora performs on behalf of the user.
//
// Like samlsp, the package only depends on go-ldap/ldap and the
// standard library. No knowledge of tenants, users, repositories,
// or HTTP. Higher layers (internal/application/service/ldap_login.go
// and internal/handler/ldap.go) compose the primitives here into
// the actual login flow.
package ldapsp
