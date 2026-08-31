package types

import "errors"

// Sentinel errors returned by the notification service. Handlers map
// these to HTTP status codes (404 / 403 / 409 / 400).
var (
	// ErrNotificationNotFound is returned when no row matches the
	// supplied id within the caller's tenant scope.
	ErrNotificationNotFound = errors.New("notification not found")

	// ErrNotificationForbidden is returned when the caller tries to
	// act on a notification owned by another user.
	ErrNotificationForbidden = errors.New("notification owned by another user")

	// ErrInvalidNotificationKind is returned by Validate when the
	// Kind field is empty or not in the closed set.
	ErrInvalidNotificationKind = errors.New("invalid notification kind")

	// ErrInvalidNotificationStatus is returned by Validate when the
	// Status field is not one of unread/read/dismissed.
	ErrInvalidNotificationStatus = errors.New("invalid notification status")

	// ErrInvalidNotificationTitle is returned by Validate when the
	// Title field is empty.
	ErrInvalidNotificationTitle = errors.New("invalid notification title")
)

// Notification service-level validation errors. These complement the
// notification-specific sentinels above and are reused across other
// services (tenant invitation, member, audit log, etc).
var (
	// ErrInvalidTenant is returned when the caller supplies a zero
	// tenant id. The notification center is always tenant-scoped.
	ErrInvalidTenant = errors.New("invalid tenant id")
	// ErrInvalidUser is returned when the caller supplies an empty
	// user id. The notification center is always user-scoped.
	ErrInvalidUser = errors.New("invalid user id")
)
