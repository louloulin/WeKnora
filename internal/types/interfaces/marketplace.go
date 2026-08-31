package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// MarketplaceRepository persists plugins, vendors, tenant installs,
// and the audit log. Implementations must be safe for concurrent
// use; the registry service hits this on every install / uninstall.
type MarketplaceRepository interface {
	// Vendors.
	UpsertVendor(ctx context.Context, v *types.PluginVendor) error
	GetVendorBySlug(ctx context.Context, slug string) (*types.PluginVendor, error)
	GetVendorByPublicKey(ctx context.Context, publicKey string) (*types.PluginVendor, error)
	ListVendors(ctx context.Context) ([]*types.PluginVendor, error)

	// Plugins (one row per plugin × version).
	UpsertPlugin(ctx context.Context, p *types.PluginRecord) error
	GetPlugin(ctx context.Context, pluginID, version string) (*types.PluginRecord, error)
	ListPlugins(ctx context.Context, status types.PluginReviewStatus, limit int) ([]*types.PluginRecord, error)
	ListVersionsByPlugin(ctx context.Context, pluginID string) ([]*types.PluginRecord, error)
	UpdatePluginStatus(ctx context.Context, pluginID, version string, status types.PluginReviewStatus, reviewerNote string) error
	IncrementDownloads(ctx context.Context, pluginID, version string) error

	// Tenant installs.
	UpsertTenantPlugin(ctx context.Context, t *types.TenantPlugin) error
	DeleteTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) error
	GetTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) (*types.TenantPlugin, error)
	ListTenantPlugins(ctx context.Context, tenantID uint64) ([]*types.TenantPlugin, error)

	// Audit log.
	AppendPluginAudit(ctx context.Context, a *types.PluginAuditLog) error
	ListPluginAudit(ctx context.Context, tenantID uint64, limit int) ([]*types.PluginAuditLog, error)
}
