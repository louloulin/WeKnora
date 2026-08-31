package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// SAMLIdentityRepository persists stable bindings between a local
// WeKnora user and a SAML 2.0 Identity Provider. The composite
// (IdPEntityID, NameID) is the unique lookup key on the ACS path:
// when the IdP POSTs a SAML Response we look up the binding by
// (IdP entity id + NameID) and load the corresponding WeKnora user.
type SAMLIdentityRepository interface {
	// GetByIdPAndNameID returns the binding for the given IdP + NameID
	// tuple, or ErrSAMLIdentityNotFound when no row exists.
	GetByIdPAndNameID(ctx context.Context, idpEntityID, nameID string) (*types.SAMLFederationIdentity, error)
	// Create persists a new binding row. ID is auto-generated when empty.
	Create(ctx context.Context, identity *types.SAMLFederationIdentity) error
	// Touch updates the last-login snapshot (email + display name +
	// timestamp). Refuses to update a row that has been revoked.
	Touch(ctx context.Context, id, email, displayName string) error
	// Revoke marks the binding as revoked. Future ACS hits on this
	// (IdP, NameID) tuple will reject the assertion even if the IdP
	// keeps sending it.
	Revoke(ctx context.Context, id string) error
	// ListByUser returns every SAML binding belonging to a user — the
	// admin UI uses this to render 'which IdPs is this user federated
	// with?'.
	ListByUser(ctx context.Context, userID string) ([]*types.SAMLFederationIdentity, error)
}
