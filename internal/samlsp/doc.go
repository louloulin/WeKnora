// Package samlsp implements the WeKnora Service Provider (SP) side of
// SAML 2.0 single sign-on.
//
// WeKnora plays the SP role: enterprise customers configure their own
// Identity Providers (IdP — ADFS, Okta, Azure AD, Feishu, Keycloak,
// ...) per tenant via the admin UI, and WeKnora consumes signed
// SAML assertions to mint its own session JWTs.
//
// The package is deliberately thin on top of github.com/crewjam/saml:
//   - SPConfig         local WeKnora-side config (entity ID, cert,
//                      ACS / SLO URLs).
//   - IdPConfig        per-tenant IdP descriptor (entity ID, SSO URL,
//                      X509 cert, NameID format).
//   - Metadata         generates the SP XML metadata document.
//   - AuthnRequest     builds a base64-encoded AuthnRequest and the
//                      redirect URL the browser is sent to.
//   - ParseResponse    decodes + signature-validates an inbound SAML
//                      Response and extracts the NameID + attribute
//                      statements.
//
// What this package deliberately does NOT do:
//   - Encrypt assertions (SAML EncryptedAssertion). We require the
//     IdP to send signed-but-not-encrypted assertions for the v1
//     release; encryption lands in a follow-up.
//   - Artifact binding (only HTTP-Redirect and HTTP-POST are wired).
//   - Identity Provider Discovery (each tenant has a fixed IdP).
//
// The package has no dependency on the application service layer:
// the tenant IdP lookup is injected by the container as a closure,
// mirroring the authz package pattern.
package samlsp
