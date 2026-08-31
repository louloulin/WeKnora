package scimsp

import "errors"

// Standard SCIM 2.0 schema URIs.
const (
	SchemaUser               = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup              = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaEnterpriseUser     = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	SchemaListResponse       = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaError              = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaPatchOp            = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaServiceProviderCfg = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SchemaResourceType       = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
)

// SCIM Content-Type. RFC 7644 §8.1 mandates clients send this on
// every write; servers can answer with plain application/json but
// most enterprise IdPs (Okta, Azure AD) honour the explicit type.
const ContentType = "application/scim+json"

// Name is the canonical handle for the SCIM representation of a
// WeKnora user. SCIM userName is required, unique within the
// tenant, and case-sensitive in the RFC — we mirror that.
type Name struct {
	Formatted       string `json:"formatted,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

// Email is the multi-valued email representation. SCIM allows many
// per user; we surface the primary via the `primary` flag (RFC
// 7643 §4.1.2).
type Email struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// User is the SCIM 2.0 representation of a local user account.
// Fields we do not currently model (addresses, phoneNumbers, photos,
// x509Certificates, entitlements, roles) are intentionally absent
// from the wire shape — emitting empty arrays signals "supported,
// empty" to enterprise IdPs and we do not yet support them.
type User struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	ExternalID  string   `json:"externalId,omitempty"`
	UserName    string   `json:"userName"`
	Name        *Name    `json:"name,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Emails      []Email  `json:"emails,omitempty"`
	Active      bool     `json:"active"`
	// Meta is required by RFC 7643 §4.1; Location points at the
	// canonical URL so Okta/Azure AD can resolve the resource.
	Meta *Meta `json:"meta"`
}

// Group is the SCIM 2.0 representation of a tenant-scoped role
// mapping. WeKnora does not have a first-class Group entity, so the
// SCIM Group maps onto the tenant membership table: id == tenantID
// (each tenant is one SCIM Group), displayName == tenant name, and
// members == tenant memberships.
type Group struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	Members     []Member `json:"members"`
	Meta        *Meta    `json:"meta"`
}

// Member references a User inside a Group.
type Member struct {
	Value   string `json:"value"`             // user id
	Ref     string `json:"$ref,omitempty"`    // canonical URL (RFC 7643 §4.4)
	Display string `json:"display,omitempty"` // best-effort display name
}

// Meta is the common metadata envelope required by RFC 7643 §4.5.
type Meta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
	Version      string `json:"version,omitempty"`
}

// ListResponse is the envelope for GET /Users and GET /Groups.
// totalResults is required; we always return an exact count
// because SCIM's filter expressions are cheap to evaluate in memory
// at our scale (per-tenant, never global).
type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	ItemsPerPage int      `json:"itemsPerPage"`
	StartIndex   int      `json:"startIndex"`
	Resources    []any    `json:"Resources"`
}

// Error is the RFC 7644 §3.7.3 error envelope. Status is the HTTP
// status; scimType refines it (invalidValue, uniqueness, tooMany,
// mutability, etc.).
type Error struct {
	Schemas  []string `json:"schemas"`
	Status   int      `json:"status"`
	Detail   string   `json:"detail,omitempty"`
	ScimType string   `json:"scimType,omitempty"`
}

// NewError builds a wire error with the mandatory schemas array.
func NewError(status int, detail, scimType string) *Error {
	return &Error{
		Schemas:  []string{SchemaError},
		Status:   status,
		Detail:   detail,
		ScimType: scimType,
	}
}

// ErrInvalidFilter signals a filter expression that did not parse.
// Translated to HTTP 400 with scimType="invalidFilter".
var ErrInvalidFilter = errors.New("scim: invalid filter expression")

// ErrUnsupportedFilterOp is returned when the filter uses an
// operator WeKnora does not yet implement (ge, le, gt, lt).
var ErrUnsupportedFilterOp = errors.New("scim: unsupported filter operator")

// PatchOp is one operation in a PATCH request body (RFC 7644 §3.5.2).
type PatchOp struct {
	Op    string `json:"op"` // "add" | "remove" | "replace"
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}

// PatchRequest is the full PATCH body.
type PatchRequest struct {
	Schemas    []string  `json:"schemas"`
	Operations []PatchOp `json:"Operations"`
}

// ServiceProviderConfig is the discovery document exposed at
// /ServiceProviderConfig. RFC 7643 §6.5 mandates a fixed shape.
type ServiceProviderConfig struct {
	Schemas               []string               `json:"schemas"`
	DocumentationURI      string                 `json:"documentationUri,omitempty"`
	Patch                 FeatureSupport         `json:"patch"`
	Bulk                  BulkSupport            `json:"bulk"`
	Filter                FeatureSupport         `json:"filter"`
	ETag                  FeatureSupport         `json:"etag"`
	SortSupported         bool                   `json:"sortSupported"`
	AuthenticationSchemes []AuthenticationScheme `json:"authenticationSchemes"`
}

// FeatureSupport is the standard "supported" flag block.
type FeatureSupport struct {
	Supported bool `json:"supported"`
}

// BulkSupport extends FeatureSupport with the max payload size and
// the max number of operations per request.
type BulkSupport struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations,omitempty"`
	MaxPayloadSize int  `json:"maxPayloadSize,omitempty"`
}

// AuthenticationScheme advertises a single auth mechanism. We
// support only Bearer tokens today.
type AuthenticationScheme struct {
	Type             string `json:"type"` // "oauth2" | "httpbasic" | "httpbearer" | ...
	Name             string `json:"name,omitempty"`
	SpecURI          string `json:"specUri,omitempty"`
	DocumentationURI string `json:"documentationUri,omitempty"`
}
