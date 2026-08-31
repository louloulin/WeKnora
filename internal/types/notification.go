package types

import (
	"time"

	"gorm.io/gorm"
)

// NotificationKind enumerates the categories of notification the system
// emits. New kinds can be added without migration as long as the
// frontend renders a fallback chip; the constraint is enforced at the
// service layer where emitters must pick from this set.
type NotificationKind string

const (
	// NotificationKindWikiCommentCreated is emitted when a new wiki
	// page comment is created and the recipient is the page owner,
	// a previous commenter, or @mentioned in the comment body.
	NotificationKindWikiCommentCreated NotificationKind = "wiki.comment.created"
	// NotificationKindWikiCommentReply is emitted when a comment
	// receives a reply (a child comment on the same page slug).
	NotificationKindWikiCommentReply NotificationKind = "wiki.comment.reply"
	// NotificationKindWikiMentioned is emitted when a user is
	// @mentioned in a comment or chat message.
	NotificationKindWikiMentioned NotificationKind = "wiki.mentioned"
	// NotificationKindAgentShared is emitted when an agent is shared
	// with the recipient (granted viewer / contributor access).
	NotificationKindAgentShared NotificationKind = "agent.shared"
	// NotificationKindKBShared is emitted when a knowledge base is
	// shared with the recipient.
	NotificationKindKBShared NotificationKind = "kb.shared"
	// NotificationKindSystemAlert is emitted by system-level events
	// (quota exceeded, plan changed, maintenance window, etc).
	NotificationKindSystemAlert NotificationKind = "system.alert"
)

// NotificationStatus represents the read state of a single
// notification row. The legal transitions are:
//
//	unread -> read | dismissed
//
// read and dismissed are terminal; once a row leaves unread it stays
// in that terminal state forever (for audit trail).
type NotificationStatus string

const (
	// NotificationStatusUnread is the initial state: the recipient
	// has not yet seen or acted on the notification.
	NotificationStatusUnread NotificationStatus = "unread"
	// NotificationStatusRead means the recipient opened the
	// notification row.
	NotificationStatusRead NotificationStatus = "read"
	// NotificationStatusDismissed means the recipient explicitly
	// dismissed the notification without opening it (e.g. clicked
	// the X button on the bell dropdown).
	NotificationStatusDismissed NotificationStatus = "dismissed"
)

// IsTerminal reports whether s is a non-unread state.
func (s NotificationStatus) IsTerminal() bool {
	return s == NotificationStatusRead || s == NotificationStatusDismissed
}

// Notification is a single in-app notification row. The (tenant_id,
// recipient_user_id, created_at) index is the canonical read-side
// index for the bell dropdown. (tenant_id, recipient_user_id,
// status) supports the unread-count query.
//
// The Payload field is a free-form JSON object keyed by Kind; the
// frontend dispatches on Kind and reads Payload accordingly. Keep
// payload keys stable once a Kind ships; treat them as a contract.
type Notification struct {
	ID              uint64           `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID        uint64           `gorm:"not null;index:idx_notif_tenant_recipient_created,priority:1" json:"tenant_id"`
	RecipientUserID string           `gorm:"size:64;not null;index:idx_notif_tenant_recipient_created,priority:2" json:"recipient_user_id"`
	Kind            NotificationKind `gorm:"size:64;not null;index" json:"kind"`
	Title           string           `gorm:"size:255;not null" json:"title"`
	Body            string           `gorm:"type:text" json:"body"`
	Payload         JSONMap          `gorm:"type:json" json:"payload"`
	Status          NotificationStatus `gorm:"size:16;not null;default:unread;index:idx_notif_tenant_recipient_status" json:"status"`
	ActorUserID     *string          `gorm:"size:64;index" json:"actor_user_id,omitempty"`
	ResourceType    string           `gorm:"size:64;index" json:"resource_type,omitempty"`
	ResourceID      string           `gorm:"size:128;index" json:"resource_id,omitempty"`
	ReadAt          *time.Time       `json:"read_at,omitempty"`
	DismissedAt     *time.Time       `json:"dismissed_at,omitempty"`
	CreatedAt       time.Time        `gorm:"not null;index:idx_notif_tenant_recipient_created,priority:3,sort:desc" json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	DeletedAt       gorm.DeletedAt   `gorm:"index" json:"-"`
}

// TableName pins the table name so GORM does not pluralize it
// (default would be `notifications`; we keep `notifications` to
// match the migration filename).
func (Notification) TableName() string {
	return "notifications"
}

// NotificationListQuery is the typed query shape for the bell
// dropdown's list endpoint. Status is optional; when nil the service
// returns all rows (regardless of read/dismissed).
type NotificationListQuery struct {
	TenantID  uint64
	UserID    string
	Status    *NotificationStatus
	Kind      *NotificationKind
	Page      int
	PageSize  int
	SinceDays int
}

// NotificationListResult bundles the page slice with the total count
// for the pagination footer in the bell dropdown.
type NotificationListResult struct {
	Items []*Notification `json:"items"`
	Total int64           `json:"total"`
}

// NotificationUnreadCount is the lightweight payload the bell polls
// every 30s to render the red dot + count badge.
type NotificationUnreadCount struct {
	Count int64 `json:"count"`
}

// Validate checks the basic invariants before a service-level Create.
// The repository layer also re-validates; this is a fast-fail for
// emitters.
func (n *Notification) Validate() error {
	if n.TenantID == 0 {
		return ErrInvalidTenant
	}
	if n.RecipientUserID == "" {
		return ErrInvalidUser
	}
	if n.Kind == "" {
		return ErrInvalidNotificationKind
	}
	if n.Title == "" {
		return ErrInvalidNotificationTitle
	}
	if !n.Status.IsValid() {
		return ErrInvalidNotificationStatus
	}
	return nil
}

// IsValid reports whether s is one of the known status values.
func (s NotificationStatus) IsValid() bool {
	switch s {
	case NotificationStatusUnread, NotificationStatusRead, NotificationStatusDismissed:
		return true
	}
	return false
}

// IsValid reports whether k is one of the known kind values. The set
// is intentionally closed so the frontend can ship a default chip per
// kind without guessing.
func (k NotificationKind) IsValid() bool {
	switch k {
	case NotificationKindWikiCommentCreated,
		NotificationKindWikiCommentReply,
		NotificationKindWikiMentioned,
		NotificationKindAgentShared,
		NotificationKindKBShared,
		NotificationKindSystemAlert:
		return true
	}
	return false
}
