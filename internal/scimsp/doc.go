// Package scimsp implements the WeKnora-side endpoint surface for
// SCIM 2.0 (RFC 7644). It is the protocol counterpart to
// internal/samlsp and internal/ldapsp: holds the type definitions
// (User, Group, ListResponse, PatchOp, ServiceProviderConfig) plus
// the small helpers every layer above needs.
//
// The package depends only on the standard library; encoding is
// delegated to internal/application/service/scim.go which owns
// the persistence mapping (display name ↔ user table, members ↔
// tenant membership). Keeping the wire types here means the
// RFC-shaped JSON we serve is in one place and easy to diff against
// the spec when we revise.
package scimsp
