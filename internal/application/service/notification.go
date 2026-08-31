package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/authz"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// notificationService is the business-logic layer over
// NotificationRepository. It enforces the (tenant, recipient) scope,
// the closed Kind / Status sets, and the read/dismiss state machine.
//
// Emit-side callers (wiki comment service, agent share service, etc.)
// use Create; the bell dropdown uses List + UnreadCount + MarkRead +
// MarkDismissed.
type notificationService struct {
	repo interfaces.NotificationRepository
	// authzChecker, when set, is consulted when a non-recipient caller
	// tries to act on a notification. The existing recipient-equality
	// fast-path stays — the checker only fires for admins / system
	// users who should be allowed to bypass the recipient check.
	// Optional so unit tests can leave it nil.
	authzChecker authz.Checker
}

// NewNotificationService wires the service against the supplied
// repository. Container.go owns the lifecycle.
func NewNotificationService(repo interfaces.NotificationRepository) interfaces.NotificationService {
	return &notificationService{repo: repo}
}

// SetAuthzChecker wires the AuthZ checker. Called by the container
// after both providers are constructed; tests may skip this to keep
// notificationService focused on persistence behaviour.
func (s *notificationService) SetAuthzChecker(c authz.Checker) {
	s.authzChecker = c
}

// Create validates the notification and persists it. Validation is
// duplicated at the model layer (Validate) so emitters that build
// the struct by hand get fast-fail errors before the DB round-trip.
func (s *notificationService) Create(ctx context.Context, n *types.Notification) error {
	if n == nil {
		return errors.New("notification is nil")
	}
	if err := n.Validate(); err != nil {
		return err
	}
	return s.repo.Create(ctx, n)
}

// List resolves the typed query into a paginated result. The
// service does not add extra business logic on top of the repository
// today; future filters (e.g. "hide system alerts older than 30
// days") belong here.
func (s *notificationService) List(
	ctx context.Context,
	q types.NotificationListQuery,
) (*types.NotificationListResult, error) {
	if q.TenantID == 0 {
		return nil, types.ErrInvalidTenant
	}
	if q.UserID == "" {
		return nil, types.ErrInvalidUser
	}
	return s.repo.List(ctx, q)
}

// UnreadCount returns the count for the bell badge.
func (s *notificationService) UnreadCount(
	ctx context.Context,
	tenantID uint64,
	userID string,
) (int64, error) {
	if tenantID == 0 {
		return 0, types.ErrInvalidTenant
	}
	if userID == "" {
		return 0, types.ErrInvalidUser
	}
	return s.repo.UnreadCount(ctx, tenantID, userID)
}

// MarkRead enforces ownership before delegating to the repository.
// Returns ErrNotificationNotFound when the id does not exist within
// the caller's tenant scope, ErrNotificationForbidden when the row
// belongs to another user.
func (s *notificationService) MarkRead(
	ctx context.Context,
	tenantID uint64,
	userID string,
	id uint64,
) error {
	if tenantID == 0 {
		return types.ErrInvalidTenant
	}
	if userID == "" {
		return types.ErrInvalidUser
	}
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if n.TenantID != tenantID {
		return types.ErrNotificationNotFound
	}
	if n.RecipientUserID != userID {
		// Not the recipient — fall back to AuthZ for admin / system
		// bypass. The recipient fast-path is the common case; this
		// branch is hit only when an operator is acting on someone
		// else's notification.
		if !s.allowNonRecipient(ctx, tenantID, userID, fmtUint64(n.ID)) {
			return types.ErrNotificationForbidden
		}
	}
	if n.Status == types.NotificationStatusRead {
		return nil
	}
	if _, err := s.repo.UpdateStatus(ctx, id, types.NotificationStatusRead); err != nil {
		logger.Errorf(ctx, "notification service MarkRead update failed: %v", err)
		return err
	}
	return nil
}

// MarkDismissed enforces ownership before delegating to the
// repository. Idempotent: re-dismissing a dismissed row is a no-op.
func (s *notificationService) MarkDismissed(
	ctx context.Context,
	tenantID uint64,
	userID string,
	id uint64,
) error {
	if tenantID == 0 {
		return types.ErrInvalidTenant
	}
	if userID == "" {
		return types.ErrInvalidUser
	}
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if n.TenantID != tenantID {
		return types.ErrNotificationNotFound
	}
	if n.RecipientUserID != userID {
		if !s.allowNonRecipient(ctx, tenantID, userID, fmtUint64(n.ID)) {
			return types.ErrNotificationForbidden
		}
	}
	if n.Status == types.NotificationStatusDismissed {
		return nil
	}
	if _, err := s.repo.UpdateStatus(ctx, id, types.NotificationStatusDismissed); err != nil {
		logger.Errorf(ctx, "notification service MarkDismissed update failed: %v", err)
		return err
	}
	return nil
}

// MarkAllRead bulk-transitions every unread row for (tenant, user)
// into read. Returns the number of rows updated.
func (s *notificationService) MarkAllRead(
	ctx context.Context,
	tenantID uint64,
	userID string,
) (int64, error) {
	if tenantID == 0 {
		return 0, types.ErrInvalidTenant
	}
	if userID == "" {
		return 0, types.ErrInvalidUser
	}
	return s.repo.MarkAllRead(ctx, tenantID, userID)
}

// DeleteHard removes a single row permanently. Used by admin tooling
// / GDPR flows; the bell dropdown does NOT call this.
func (s *notificationService) DeleteHard(
	ctx context.Context,
	tenantID uint64,
	userID string,
	id uint64,
) error {
	if tenantID == 0 {
		return types.ErrInvalidTenant
	}
	if userID == "" {
		return types.ErrInvalidUser
	}
	return s.repo.DeleteHard(ctx, tenantID, userID, id)
}

// allowNonRecipient consults the AuthZ checker to decide whether a
// non-recipient caller can act on another user's notification. Used
// by MarkRead / MarkDismissed when the recipient-equality fast-path
// fails. Returns true when:
//   - the AuthZ checker is not configured (test path / pre-rollout)
//   - the checker allows the request explicitly (Admin role or
//     system principal)
// Returns false otherwise so the caller surfaces ErrNotificationForbidden.
//
// We deliberately do NOT log the authz decision here; the caller
// already has enough context (the http handler maps the error to a
// 403 and the audit log captures who tried what on whose row).
func (s *notificationService) allowNonRecipient(
	ctx context.Context,
	tenantID uint64,
	userID string,
	notificationID string,
) bool {
	if s.authzChecker == nil {
		return false
	}
	req := authz.CheckRequest{
		User: authz.User{
			Type:     authz.UserTypeUser,
			ID:       userID,
			TenantID: tenantID,
			// Role is filled in by the AuthZ composite from context when
			// the caller did not pass it explicitly; empty is fine for
			// the notification adapter because the adapter looks up
			// the recipient and only consults role for admin bypass.
		},
		Object: authz.Object{
			Type:     authz.ObjectTypeNotification,
			ID:       notificationID,
			TenantID: tenantID,
		},
		Relation: authz.RelationAdmin,
	}
	d := s.authzChecker.Check(ctx, req)
	return d.Allowed
}

// Compile-time assertion that the GORM dependency is imported so the
// container wiring stays trivial. The repository implementation
// already pulls gorm.io/gorm in directly; this line keeps it that
// way even after the service layer drops the import.
var _ = gorm.ErrRecordNotFound

// fmtUint64 is a tiny uint64→string helper to keep this file
// allocation-light. The notification ID is rarely needed as a
// string outside of the AuthZ object key, so importing strconv
// across the whole file is not worth it.
func fmtUint64(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
