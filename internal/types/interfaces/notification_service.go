package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// NotificationService is the business-logic layer over
// NotificationRepository. It enforces the (tenant, recipient) scope,
// the closed Kind / Status sets, and the read/dismiss state machine.
//
// Emit-side callers (wiki comment service, agent share service, etc.)
// use Create; the bell dropdown uses List + UnreadCount + MarkRead +
// MarkDismissed.
type NotificationService interface {
	// Create emits a new notification row. The caller MUST supply
	// TenantID, RecipientUserID, Kind, Title and a non-empty Status
	// (always NotificationStatusUnread for fresh emits). ActorUserID
	// and the resource pointer are optional but recommended so the
	// frontend can deep-link.
	Create(ctx context.Context, n *types.Notification) error

	// List returns the bell dropdown page slice. The query.Status
	// filter is optional; pass nil for "any". The result is sorted
	// by CreatedAt DESC so the most recent items render at the top.
	List(ctx context.Context, q types.NotificationListQuery) (*types.NotificationListResult, error)

	// UnreadCount returns the lightweight count the bell polls every
	// 30s. It is a separate endpoint so the dropdown does not need
	// to fetch the full page just to render the badge.
	UnreadCount(ctx context.Context, tenantID uint64, userID string) (int64, error)

	// MarkRead transitions a single row from unread to read. Returns
	// ErrNotificationNotFound if no row matches (tenant, id) or
	// ErrNotificationForbidden if the row is owned by another user.
	// Idempotent: re-marking an already-read row is a no-op.
	MarkRead(ctx context.Context, tenantID uint64, userID string, id uint64) error

	// MarkDismissed transitions a single row from unread (or read) to
	// dismissed. Same ownership rule as MarkRead. Idempotent.
	MarkDismissed(ctx context.Context, tenantID uint64, userID string, id uint64) error

	// MarkAllRead bulk-transitions every unread row for (tenant, user)
	// into read. Returns the number of rows updated. Used by the
	// "mark all as read" button on the bell dropdown footer.
	MarkAllRead(ctx context.Context, tenantID uint64, userID string) (int64, error)

	// DeleteHard removes a single row permanently. Reserved for
	// admin tooling / GDPR right-to-erasure flows; the bell dropdown
	// does NOT call this (it uses MarkDismissed so the audit trail
	// stays intact).
	DeleteHard(ctx context.Context, tenantID uint64, userID string, id uint64) error
}
