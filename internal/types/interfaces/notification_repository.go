package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// NotificationRepository is the persistence-layer interface. It is
// intentionally thin so the service layer can drive all business
// rules without leaking GORM specifics.
type NotificationRepository interface {
	// Create inserts a new notification row. The implementation MUST
	// honor the (TenantID, RecipientUserID) compound index so the
	// bell dropdown's read query stays O(log n).
	Create(ctx context.Context, n *types.Notification) error

	// GetByID returns a single row by primary key. Used by MarkRead
	// and MarkDismissed so they can enforce ownership before mutation.
	GetByID(ctx context.Context, id uint64) (*types.Notification, error)

	// List returns the page slice for the bell dropdown. The
	// implementation MUST always scope by (TenantID, RecipientUserID)
	// — no caller is allowed to bypass tenant scope.
	List(ctx context.Context, q types.NotificationListQuery) (*types.NotificationListResult, error)

	// UnreadCount returns the count of unread rows for the
	// (tenant, user) pair. The implementation MUST use the compound
	// index, not a table scan.
	UnreadCount(ctx context.Context, tenantID uint64, userID string) (int64, error)

	// UpdateStatus mutates the status field for a single row. Returns
	// the post-update row so the service can echo ReadAt/DismissedAt
	// timestamps back to the caller.
	UpdateStatus(ctx context.Context, id uint64, status types.NotificationStatus) (*types.Notification, error)

	// MarkAllRead bulk-updates every unread row for (tenant, user)
	// to read. Returns the number of rows affected.
	MarkAllRead(ctx context.Context, tenantID uint64, userID string) (int64, error)

	// DeleteHard removes a single row permanently. The implementation
	// MUST honor the (tenant, user) scope so cross-tenant deletes
	// are impossible.
	DeleteHard(ctx context.Context, tenantID uint64, userID string, id uint64) error
}
