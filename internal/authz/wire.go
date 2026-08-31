package authz

import (
	"context"
)

// NotificationRecipientLookup is the minimal lookup shape the
// notification adapter needs. It is satisfied by the notification
// service today and is duplicated here so the authz package stays
// import-cycle-free (authz must not import the service layer).
type NotificationRecipientLookup func(ctx context.Context, tenantID uint64, notificationID string) (recipientUserID string, found bool, err error)

// ChatMessageSessionOwnerLookup is the minimal lookup shape the
// chat-message adapter needs. Returns ownerID, sessionID, found.
type ChatMessageSessionOwnerLookup func(ctx context.Context, tenantID uint64, messageID string) (ownerID string, sessionID string, found bool, err error)

// WireOptions configures which adapters the default composite
// registers. Tests pass a smaller set; production passes the full
// bag. A nil field means the matching adapter is omitted.
type WireOptions struct {
	// NotificationRecipient, when non-nil, registers the
	// notification adapter. The lookup is the only function the
	// adapter needs; all other state lives in the service layer.
	NotificationRecipient NotificationRecipientLookup
	// ChatMessageSessionOwner, when non-nil, registers the
	// chat-message adapter.
	ChatMessageSessionOwner ChatMessageSessionOwnerLookup
}

// NewAuthZComposite builds the production composite with the
// tenant-role adapter always on and any provided adapters layered
// on top. New adapters are appended here as their services
// stabilise; this is the single registration point.
func NewAuthZComposite(opts WireOptions) Checker {
	adapters := []Adapter{NewTenantRoleAdapter()}
	if opts.NotificationRecipient != nil {
		adapters = append(adapters, NewNotificationAdapter(opts.NotificationRecipient))
	}
	if opts.ChatMessageSessionOwner != nil {
		adapters = append(adapters, NewChatMessageAdapter(opts.ChatMessageSessionOwner))
	}
	return NewCompositeChecker(adapters...)
}
