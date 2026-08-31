package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// notificationRepository is the GORM-backed implementation of
// NotificationRepository. All methods are scoped by tenant + user at
// the SQL level so a buggy caller cannot accidentally cross tenant
// boundaries.
type notificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository wires the repository against the shared
// GORM handle. The handle is expected to be the tenant-aware
// connection (the container resolves it per request via the
// TenantContext middleware).
func NewNotificationRepository(db *gorm.DB) interfaces.NotificationRepository {
	return &notificationRepository{db: db}
}

// Create inserts a new notification row. The timestamp fields are
// filled here rather than relying on GORM auto-fill so the row is
// monotonically correct even when the caller passes a zero-time.
func (r *notificationRepository) Create(ctx context.Context, n *types.Notification) error {
	if n == nil {
		return errors.New("notification is nil")
	}
	now := time.Now().UTC()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	if n.Status == "" {
		n.Status = types.NotificationStatusUnread
	}
	if err := r.db.WithContext(ctx).Create(n).Error; err != nil {
		logger.Errorf(ctx, "notification repo Create failed: %v", err)
		return err
	}
	return nil
}

// GetByID returns the row by primary key. Callers (the service layer)
// are responsible for re-checking the tenant / recipient scope before
// mutation.
func (r *notificationRepository) GetByID(ctx context.Context, id uint64) (*types.Notification, error) {
	var n types.Notification
	if err := r.db.WithContext(ctx).First(&n, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotificationNotFound
		}
		logger.Errorf(ctx, "notification repo GetByID failed: %v", err)
		return nil, err
	}
	return &n, nil
}

// List returns the paginated slice for the bell dropdown. The query
// is scoped by (TenantID, RecipientUserID) — there is no way to call
// this without those two fields. Status / Kind / SinceDays are
// optional filters.
func (r *notificationRepository) List(
	ctx context.Context,
	q types.NotificationListQuery,
) (*types.NotificationListResult, error) {
	if q.TenantID == 0 || q.UserID == "" {
		return nil, errors.New("tenant_id and recipient_user_id are required")
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	tx := r.db.WithContext(ctx).Model(&types.Notification{}).
		Where("tenant_id = ? AND recipient_user_id = ?", q.TenantID, q.UserID)

	if q.Status != nil {
		tx = tx.Where("status = ?", *q.Status)
	}
	if q.Kind != nil {
		tx = tx.Where("kind = ?", *q.Kind)
	}
	if q.SinceDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -q.SinceDays)
		tx = tx.Where("created_at >= ?", cutoff)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		logger.Errorf(ctx, "notification repo List count failed: %v", err)
		return nil, err
	}

	var items []*types.Notification
	offset := (q.Page - 1) * q.PageSize
	if err := tx.Order("created_at DESC").
		Limit(q.PageSize).
		Offset(offset).
		Find(&items).Error; err != nil {
		logger.Errorf(ctx, "notification repo List query failed: %v", err)
		return nil, err
	}

	return &types.NotificationListResult{Items: items, Total: total}, nil
}

// UnreadCount is the lightweight count the bell polls every 30s. The
// compound index (tenant_id, recipient_user_id, status) makes this
// O(log n) regardless of how many read rows accumulate.
func (r *notificationRepository) UnreadCount(
	ctx context.Context,
	tenantID uint64,
	userID string,
) (int64, error) {
	if tenantID == 0 || userID == "" {
		return 0, errors.New("tenant_id and user_id are required")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&types.Notification{}).
		Where("tenant_id = ? AND recipient_user_id = ? AND status = ?",
			tenantID, userID, types.NotificationStatusUnread).
		Count(&count).Error; err != nil {
		logger.Errorf(ctx, "notification repo UnreadCount failed: %v", err)
		return 0, err
	}
	return count, nil
}

// UpdateStatus mutates the status field for a single row. The
// timestamp columns (ReadAt, DismissedAt) are set according to the
// target state so the row carries a complete audit trail.
func (r *notificationRepository) UpdateStatus(
	ctx context.Context,
	id uint64,
	status types.NotificationStatus,
) (*types.Notification, error) {
	if !status.IsValid() {
		return nil, types.ErrInvalidNotificationStatus
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":     status,
		"updated_at": now,
	}
	switch status {
	case types.NotificationStatusRead:
		updates["read_at"] = now
	case types.NotificationStatusDismissed:
		updates["dismissed_at"] = now
	}
	if err := r.db.WithContext(ctx).Model(&types.Notification{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		logger.Errorf(ctx, "notification repo UpdateStatus failed: %v", err)
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// MarkAllRead bulk-updates every unread row for (tenant, user) to
// read. Returns the number of rows affected.
func (r *notificationRepository) MarkAllRead(
	ctx context.Context,
	tenantID uint64,
	userID string,
) (int64, error) {
	if tenantID == 0 || userID == "" {
		return 0, errors.New("tenant_id and user_id are required")
	}
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&types.Notification{}).
		Where("tenant_id = ? AND recipient_user_id = ? AND status = ?",
			tenantID, userID, types.NotificationStatusUnread).
		Updates(map[string]any{
			"status":     types.NotificationStatusRead,
			"read_at":    now,
			"updated_at": now,
		})
	if res.Error != nil {
		logger.Errorf(ctx, "notification repo MarkAllRead failed: %v", res.Error)
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// DeleteHard removes a single row permanently. The WHERE clause
// includes both tenant_id and recipient_user_id so cross-tenant or
// cross-user deletes are impossible even if a caller passes the wrong
// id.
func (r *notificationRepository) DeleteHard(
	ctx context.Context,
	tenantID uint64,
	userID string,
	id uint64,
) error {
	if tenantID == 0 || userID == "" {
		return errors.New("tenant_id and user_id are required")
	}
	res := r.db.WithContext(ctx).Where(
		"id = ? AND tenant_id = ? AND recipient_user_id = ?",
		id, tenantID, userID,
	).Delete(&types.Notification{})
	if res.Error != nil {
		logger.Errorf(ctx, "notification repo DeleteHard failed: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected == 0 {
		return types.ErrNotificationNotFound
	}
	return nil
}
