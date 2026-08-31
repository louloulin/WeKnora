package authz

import "context"

// This file groups the per-resource lookup type signatures that
// adapters depend on. Each lookup is a small closure capturing
// only the data the adapter needs; the container constructs the
// closures from the underlying service. Keeping the types here
// (rather than in the service package) lets the authz package
// stay import-cycle-free.

// KBCreatorLookup returns the (tenantID, creatorUserID) for a KB.
// creatorUserID is the user who owns the KB; an empty creatorUserID
// means the row exists but has no recorded creator (legacy).
type KBCreatorLookup func(ctx context.Context, kbID string) (tenantID uint64, creatorUserID string, found bool, err error)

// KBShareLookup resolves the caller's effective permission on a KB
// reached via org sharing. Returns (effectiveRole, isShared, err).
// isShared=true means the caller's tenant has at least one org-
// level share granting access; effectiveRole is the maximum role
// across all matching shares (already capped by the 3-D rule).
type KBShareLookup func(ctx context.Context, kbID string, callerTenantID uint64, callerTenantRole string) (effectiveRole string, isShared bool, err error)

// WikiPageResolveLookup wraps WikiAclService.Resolve. Returns one
// of the WikiPageAcl decision constants (allow / deny_allow_list /
// deny_private) or an empty string if the page is missing.
type WikiPageResolveLookup func(ctx context.Context, kbID string, slug string, callerUserID string) (decision string, found bool, err error)

// WikiPageOwnerLookup returns the (creatorUserID, tenantID) for a
// wiki page. The owner short-circuits ACL resolution in the
// adapter.
type WikiPageOwnerLookup func(ctx context.Context, kbID string, slug string) (creatorUserID string, tenantID uint64, found bool, err error)

// AgentCreatorLookup returns the (tenantID, creatorUserID) for an
// agent.
type AgentCreatorLookup func(ctx context.Context, agentID string) (tenantID uint64, creatorUserID string, found bool, err error)

// AgentShareLookup mirrors KBShareLookup for agents.
type AgentShareLookup func(ctx context.Context, agentID string, callerTenantID uint64, callerTenantRole string) (effectiveRole string, isShared bool, err error)

// NotificationRecipientLookup resolves (tenant, notificationID) to
// the recipient user id. Used by the notification adapter; declared
// here (not in the service package) to keep authz import-cycle-free.
type NotificationRecipientLookup func(ctx context.Context, tenantID uint64, notificationID string) (recipientUserID string, found bool, err error)

// ChatMessageSessionOwnerLookup resolves a chat message id to
// (ownerID, sessionID, found).
type ChatMessageSessionOwnerLookup func(ctx context.Context, tenantID uint64, messageID string) (ownerID string, sessionID string, found bool, err error)

// DataSourceCreatorLookup returns (tenantID, creatorUserID, kbID,
// found) for a datasource. The kbID enables KB-level inheritance
// in the adapter (a user with role X on a KB has the same role on
// every datasource under it).
type DataSourceCreatorLookup func(ctx context.Context, dsID string) (tenantID uint64, creatorUserID string, kbID string, found bool, err error)

// DataSourceShareLookup mirrors KBShareLookup for datasources.
// Cross-tenant access is granted either via a direct tenant-level
// share or via the parent KB's share — the implementation must
// union the two layers.
type DataSourceShareLookup func(ctx context.Context, dsID string, callerTenantID uint64, callerTenantRole string) (effectiveRole string, isShared bool, err error)
