package authz

import (
	"context"
	"strings"
)

// TenantRoleAdapter answers checks against tenant-scoped relations
// using the caller's role. The matrix is intentionally minimal:
//
//	relation      minimum role required
//	----------    ----------------------
//	admin         Admin or Owner
//	owner         Owner only
//	editor        Contributor, Admin, or Owner
//	viewer        Viewer, Contributor, Admin, or Owner
//	read          same as viewer
//	comment       Contributor, Admin, or Owner
//	share         Admin or Owner
//	delete        Owner only
//	mention       Viewer or above (tenant members can be @mentioned)
//
// SystemAdmin principals are treated as Owner for everything. The
// adapter is pure (no I/O) so it is safe to register globally.
type TenantRoleAdapter struct{}

// NewTenantRoleAdapter constructs the role adapter.
func NewTenantRoleAdapter() *TenantRoleAdapter { return &TenantRoleAdapter{} }

// ObjectType returns the tenant object type. The tenant adapter
// answers checks for tenant-scoped objects (KB, agent, datasource,
// notification) by reading the role on the User. Direct tenant
// objects use this adapter too.
func (a *TenantRoleAdapter) ObjectType() ObjectType { return ObjectTypeTenant }

// roleRank returns the numeric rank for a role string. Unknown
// roles rank below Viewer so a missing role still resolves
// conservatively.
func roleRank(role string) int {
	switch strings.ToLower(role) {
	case "owner":
		return 40
	case "admin":
		return 30
	case "contributor":
		return 20
	case "viewer":
		return 10
	}
	return 0
}

// requiredRankForRelation encodes the matrix above.
func requiredRankForRelation(rel Relation) int {
	switch rel {
	case RelationOwner, RelationDelete:
		return 40 // Owner
	case RelationAdmin, RelationShare:
		return 30 // Admin+
	case RelationEditor, RelationComment:
		return 20 // Contributor+
	case RelationViewer, RelationRead, RelationMention:
		return 10 // Viewer+
	}
	// Unknown relations require Admin by default so an untyped
	// Check("foo") does not accidentally allow everyone.
	return 30
}

// Check answers the request using User.Role only. Cross-tenant has
// already been short-circuited by the composite, but we double-check
// here so this adapter is also safe to call directly in tests.
func (a *TenantRoleAdapter) Check(_ context.Context, req CheckRequest) Decision {
	source := "tenant_role"
	if req.User.TenantID != 0 && req.Object.TenantID != 0 &&
		req.User.TenantID != req.Object.TenantID {
		return Deny(CodeWrongTenant, source,
			"principal tenant differs from resource tenant")
	}
	if req.User.Type == UserTypeSystem {
		return Allow(source, "system principal is always allowed")
	}
	rank := roleRank(req.User.Role)
	required := requiredRankForRelation(req.Relation)
	if rank >= required {
		return Allow(source, "role "+req.User.Role+" satisfies "+string(req.Relation))
	}
	return Deny(CodeRoleTooLow, source,
		"role "+req.User.Role+" is below required for "+string(req.Relation))
}

// Invalidate is a no-op for the role adapter because role changes
// are tenant-wide and the role adapter re-reads User.Role on every
// Check. The composite-level cache handles per-request eviction.
func (a *TenantRoleAdapter) Invalidate(_ context.Context, _ Object) error {
	return nil
}

// NotificationAdapter answers checks against Notification objects.
// The model is intentionally minimal: the only relation we model
// today is "viewer" (the recipient can read the notification).
// MarkRead / MarkDismissed use the same relation — the service
// layer maps them onto the underlying state machine.
type NotificationAdapter struct {
	// RecipientLookup resolves (tenant, notificationID) to the
	// recipient user id. When nil the adapter cannot answer and
	// falls back to CodeNoSuchResource so the caller surfaces
	// 404 instead of 403 (we do not want to leak existence).
	RecipientLookup func(ctx context.Context, tenantID uint64, notificationID string) (recipientUserID string, found bool, err error)
}

// NewNotificationAdapter constructs the adapter.
func NewNotificationAdapter(lookup func(ctx context.Context, tenantID uint64, notificationID string) (string, bool, error)) *NotificationAdapter {
	return &NotificationAdapter{RecipientLookup: lookup}
}

// ObjectType returns the notification object type.
func (a *NotificationAdapter) ObjectType() ObjectType { return ObjectTypeNotification }

// Check answers whether the principal is the recipient of the
// notification. Admin-level checks fall through to the tenant role
// adapter (registered separately) so an Admin can mark all read.
func (a *NotificationAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "notification_recipient"
	if a.RecipientLookup == nil {
		return Deny(CodeError, source, "recipient lookup is not configured")
	}
	recipient, found, err := a.RecipientLookup(ctx, req.Object.TenantID, req.Object.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !found {
		return Deny(CodeNoSuchResource, source, "notification does not exist")
	}
	if req.User.Type == UserTypeSystem {
		return Allow(source, "system principal allowed")
	}
	if req.User.ID == recipient {
		return Allow(source, "user is the recipient")
	}
	// Admins can manage all notifications in their tenant.
	rank := roleRank(req.User.Role)
	if rank >= roleRank("admin") {
		return Allow(source, "admin role allowed on any notification")
	}
	return Deny(CodeNotShared, source,
		"user is not the recipient and does not have admin role")
}

// Invalidate is a no-op today; the notification cache lives in the
// service layer and is invalidated when rows transition state.
func (a *NotificationAdapter) Invalidate(_ context.Context, _ Object) error {
	return nil
}

// ChatMessageAdapter answers checks against chat messages. The
// model: viewer/comment is allowed when the principal is the
// session owner (creator) or has been added as a participant. The
// recipient lookup is captured via a closure so the adapter stays
// dependency-free at this layer.
type ChatMessageAdapter struct {
	// SessionOwnerLookup resolves (tenant, messageID) to the
	// session owner (creator) user id.
	SessionOwnerLookup func(ctx context.Context, tenantID uint64, messageID string) (ownerID string, sessionID string, found bool, err error)
}

// NewChatMessageAdapter constructs the adapter.
func NewChatMessageAdapter(lookup func(ctx context.Context, tenantID uint64, messageID string) (string, string, bool, error)) *ChatMessageAdapter {
	return &ChatMessageAdapter{SessionOwnerLookup: lookup}
}

// ObjectType returns the chat-message object type.
func (a *ChatMessageAdapter) ObjectType() ObjectType { return ObjectTypeChatMessage }

// Check answers whether the principal can read/comment on the
// chat message. Owner and Admin roles can always read; other roles
// must be the session owner. Mention notifications deep-link into
// the chat, so the recipient of a mention can also read.
func (a *ChatMessageAdapter) Check(ctx context.Context, req CheckRequest) Decision {
	source := "chat_message"
	if a.SessionOwnerLookup == nil {
		return Deny(CodeError, source, "session owner lookup is not configured")
	}
	ownerID, _, found, err := a.SessionOwnerLookup(ctx, req.Object.TenantID, req.Object.ID)
	if err != nil {
		return Deny(CodeError, source, err.Error())
	}
	if !found {
		return Deny(CodeNoSuchResource, source, "chat message does not exist")
	}
	if req.User.Type == UserTypeSystem {
		return Allow(source, "system principal allowed")
	}
	if req.User.ID == ownerID {
		return Allow(source, "user owns the session")
	}
	rank := roleRank(req.User.Role)
	if rank >= roleRank("admin") {
		return Allow(source, "admin role allowed on any chat message")
	}
	return Deny(CodeNotShared, source,
		"user is not the session owner and does not have admin role")
}

// Invalidate is a no-op today; chat-message cache lives in the
// session service.
func (a *ChatMessageAdapter) Invalidate(_ context.Context, _ Object) error {
	return nil
}
