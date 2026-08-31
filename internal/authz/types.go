package authz

import (
	"context"
	"fmt"
)

// ObjectType enumerates the resource kinds that the authorisation
// layer understands. Adding a new ObjectType requires registering an
// adapter on the composite Checker; the unknown-type path returns
// CodeNoSuchAdapter so callers can distinguish "this object has no
// policy" from "user is not allowed".
type ObjectType string

const (
	ObjectTypeTenant       ObjectType = "tenant"
	ObjectTypeKB           ObjectType = "kb"
	ObjectTypeWikiPage     ObjectType = "wiki_page"
	ObjectTypeAgent        ObjectType = "agent"
	ObjectTypeDatasource   ObjectType = "datasource"
	ObjectTypeNotification ObjectType = "notification"
	ObjectTypeChatMessage  ObjectType = "chat_message"
)

// UserType distinguishes principal shapes. Tenant users, API keys,
// and agent identities share the same Check surface so route handlers
// do not have to special-case each caller.
type UserType string

const (
	UserTypeUser   UserType = "user"
	UserTypeAPIKey UserType = "api_key"
	UserTypeAgent  UserType = "agent"
	UserTypeSystem UserType = "system"
)

// Relation is the verb we are asking about: "can this user `relation`
// this object?". The closed set here is a deliberate mirror of the
// relations the built-in adapters implement today; expand only when
// the new relation has at least one adapter covering it.
type Relation string

const (
	RelationOwner    Relation = "owner"
	RelationEditor   Relation = "editor"
	RelationViewer   Relation = "viewer"
	RelationAdmin    Relation = "admin"
	RelationMention  Relation = "mention"   // user can be @mentioned
	RelationComment  Relation = "comment"   // user can read/comment
	RelationShare    Relation = "share"     // user can be granted access
	RelationDelete   Relation = "delete"
	RelationRead     Relation = "read"
)

// Object identifies a resource under authorisation. The (Type, ID)
// pair is the primary key; TenantID is denormalised so a Check can
// short-circuit on cross-tenant access without round-tripping to
// the resource store.
type Object struct {
	Type     ObjectType `json:"type"`
	ID       string     `json:"id"`
	TenantID uint64     `json:"tenant_id,omitempty"`
}

// String renders the canonical Object identifier.
func (o Object) String() string {
	if o.TenantID != 0 {
		return fmt.Sprintf("%s:%s@%d", o.Type, o.ID, o.TenantID)
	}
	return fmt.Sprintf("%s:%s", o.Type, o.ID)
}

// User identifies the principal making the request. TenantID is the
// caller's tenant — used to short-circuit cross-tenant checks. Role
// is the caller's tenant role (Owner/Admin/Contributor/Viewer) when
// known; the adapter may use it to grant relations cheaply.
type User struct {
	Type     UserType `json:"type"`
	ID       string   `json:"id"`
	TenantID uint64   `json:"tenant_id,omitempty"`
	// Role is optional; when empty the adapter fetches it itself.
	Role string `json:"role,omitempty"`
	// Capabilities are optional; populated for APIKey principals to
	// let the api-key adapter answer without a store round-trip.
	Capabilities []string `json:"capabilities,omitempty"`
}

// String renders the canonical User identifier.
func (u User) String() string {
	if u.TenantID != 0 {
		return fmt.Sprintf("%s:%s@%d", u.Type, u.ID, u.TenantID)
	}
	return fmt.Sprintf("%s:%s", u.Type, u.ID)
}

// CheckRequest is the input to Checker.Check. Caller fills Object and
// the desired Relation; Checker returns a Decision.
type CheckRequest struct {
	User     User     `json:"user"`
	Object   Object   `json:"object"`
	Relation Relation `json:"relation"`
}

// ReasonCode is a stable, programmatic identifier for the decision.
// It is safe to log, return over the wire (when the caller is
// internal), and aggregate in dashboards. Human-readable explanations
// go in Decision.Message.
type ReasonCode string

const (
	// CodeAllowed is the only "happy path" code. Everything else
	// is a deny of some flavour.
	CodeAllowed ReasonCode = "allowed"
	// CodeNoRelation means the principal has no tuple relating
	// them to the object (no membership, no share, no role).
	CodeNoRelation ReasonCode = "no_relation"
	// CodeWrongTenant means the principal is in a different
	// tenant from the object — a hard deny.
	CodeWrongTenant ReasonCode = "wrong_tenant"
	// CodeRoleTooLow means a role-based gate would let the
	// caller in but their role is below the required threshold.
	CodeRoleTooLow ReasonCode = "role_too_low"
	// CodeOwnerOnly means only the resource owner / creator
	// is allowed; the caller is not the owner.
	CodeOwnerOnly ReasonCode = "owner_only"
	// CodeNotShared means the resource requires an explicit
	// share grant (KB Share, Agent Share, Wiki allow_list) and
	// the caller has none.
	CodeNotShared ReasonCode = "not_shared"
	// CodeNoSuchResource means the object does not exist; this
	// is distinct from CodeNotShared because the right HTTP
	// answer is 404 rather than 403.
	CodeNoSuchResource ReasonCode = "no_such_resource"
	// CodeNoSuchAdapter means no adapter registered for the
	// object type — surfaces integration gaps early.
	CodeNoSuchAdapter ReasonCode = "no_such_adapter"
	// CodeError is the catch-all for unexpected errors from
	// the underlying store; the decision is conservative-deny.
	CodeError ReasonCode = "error"
)

// Decision is the result of a Check. Always non-nil. Allowed is the
// single source of truth; Code + Source + Message are explainability
// metadata for logs, audit, and admin debugging.
type Decision struct {
	Allowed bool       `json:"allowed"`
	Code    ReasonCode `json:"code"`
	// Source identifies which adapter produced the decision
	// (e.g. "kb_share", "wiki_acl", "tenant_role"). Multiple
	// adapters may be consulted; Source reflects the one whose
	// outcome dominated (the first allow OR the most specific
	// deny).
	Source   string `json:"source,omitempty"`
	// Message is a human-readable explanation suitable for an
	// admin-facing audit log. Never expose to end users verbatim
	// because it can leak resource existence.
	Message string `json:"message,omitempty"`
}

// Allow constructs a positive Decision. Use the helper so the
// allowed + source + message trio is always populated consistently.
func Allow(source, message string) Decision {
	return Decision{Allowed: true, Code: CodeAllowed, Source: source, Message: message}
}

// Deny constructs a negative Decision.
func Deny(code ReasonCode, source, message string) Decision {
	return Decision{Allowed: false, Code: code, Source: source, Message: message}
}

// Checker is the entry point for authorisation queries. Every route
// guard, every service-layer permission check, and every audit log
// eventually calls one of these methods.
type Checker interface {
	// Check answers whether the request is allowed. Errors are
	// mapped to a conservative-deny Decision with CodeError —
	// callers can detect this case via Decision.Code and decide
	// whether to surface the underlying error.
	Check(ctx context.Context, req CheckRequest) Decision

	// CheckBulk fans out across many requests with bounded
	// concurrency. Order of returned decisions matches the order
	// of input requests. Used by search-result fan-out paths
	// where the same caller is checked against many objects.
	CheckBulk(ctx context.Context, reqs []CheckRequest) []Decision

	// Invalidate drops any cached decisions tied to the given
	// object. Called by the underlying services when a relation
	// changes (share granted / revoked, ACL edited, role
	// changed) so the next Check re-evaluates from scratch.
	Invalidate(ctx context.Context, obj Object) error
}
