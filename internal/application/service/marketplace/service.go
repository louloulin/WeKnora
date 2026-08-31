package marketplace

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors returned by the service layer. Exposed for the
// handler to map onto HTTP status codes.
var (
	ErrVendorNotFound    = errors.New("marketplace: vendor not found")
	ErrPluginNotFound    = errors.New("marketplace: plugin not found")
	ErrPluginNotPublic   = errors.New("marketplace: plugin not published")
	ErrAlreadyInstalled  = errors.New("marketplace: plugin already installed")
	ErrNotInstalled      = errors.New("marketplace: plugin not installed")
	ErrPermissionDenied  = errors.New("marketplace: permission denied for tenant")
)

// ObjectStorage is the minimal interface the registry needs to
// store and retrieve plugin artifact tarballs. Production wires
// this to internal/storageallowlist or S3.
type ObjectStorage interface {
	Upload(ctx context.Context, key string, data []byte) (string, error)
}

// Service is the application service for the Build #34.x
// Marketplace. It coordinates vendor registration, plugin publish
// (with signature verification), and per-tenant install / uninstall.
type Service struct {
	repo   interfaces.MarketplaceRepository
	store  ObjectStorage
	now    func() time.Time
}

// NewService constructs a marketplace Service.
func NewService(repo interfaces.MarketplaceRepository, store ObjectStorage) *Service {
	return &Service{
		repo:  repo,
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// SetNow lets tests freeze time.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// --- Vendors ---

// RegisterVendor creates or updates a plugin vendor. The vendor's
// public key is the trust anchor for every plugin version they
// publish.
func (s *Service) RegisterVendor(ctx context.Context, v *types.PluginVendor) error {
	if v.Slug == "" || v.Name == "" || v.PublicKey == "" {
		return ErrInvalidManifest
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = s.now()
	}
	v.UpdatedAt = s.now()
	return s.repo.UpsertVendor(ctx, v)
}

// GetVendor returns a vendor by slug.
func (s *Service) GetVendor(ctx context.Context, slug string) (*types.PluginVendor, error) {
	v, err := s.repo.GetVendorBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrVendorNotFound
	}
	return v, nil
}

// ListVendors returns every registered vendor.
func (s *Service) ListVendors(ctx context.Context) ([]*types.PluginVendor, error) {
	return s.repo.ListVendors(ctx)
}

// --- Publish ---

// Publish takes a signed manifest and (optionally) the artifact
// bytes. The manifest's signature is verified against the vendor's
// registered public key; on success the manifest is stored and
// the artifact (if provided) is uploaded to object storage. The
// resulting PluginRecord is created in "submitted" status pending
// editorial review.
func (s *Service) Publish(ctx context.Context, m *types.PluginManifest, artifact []byte) (*types.PluginRecord, error) {
	if m.ID == "" || m.Version == "" || m.Author == "" {
		return nil, ErrInvalidManifest
	}
	// Verify signature against the registered public key. We honor
	// the public key embedded in the manifest, but only if it
	// matches an already-registered vendor's key. (First publish
	// requires the admin to register the vendor separately.)
	vendor, err := s.repo.GetVendorBySlug(ctx, m.Author)
	if err != nil {
		return nil, err
	}
	if vendor == nil {
		return nil, ErrVendorNotFound
	}
	// The author's public key in the manifest must equal the
	// vendor's registered public key — otherwise a malicious actor
	// could claim a vendor slug with their own key.
	if !keysEqual(vendor.PublicKey, m.AuthorPublicKey) {
		return nil, ErrUntrustedPlugin
	}
	if err := VerifySignature(m); err != nil {
		return nil, err
	}
	// Optional artifact upload.
	var artifactURL string
	if len(artifact) > 0 && s.store != nil {
		url, err := s.store.Upload(ctx, m.ID+"/"+m.Version+".tar.gz", artifact)
		if err != nil {
			return nil, err
		}
		artifactURL = url
	}
	rec := &types.PluginRecord{
		PluginID:       m.ID,
		Version:        m.Version,
		Name:           m.Name,
		Description:    m.Description,
		Author:         m.Author,
		Homepage:       m.Homepage,
		Permissions:    m.Permissions,
		EntryPoint:     m.EntryPoint,
		ArtifactURL:    artifactURL,
		ArtifactSHA256: m.ArtifactSHA256,
		IconURL:        m.IconURL,
		Status:         types.PluginReviewSubmitted,
		TrustTier:      types.PluginTrustBasic,
		SubmittedAt:    s.now(),
		CreatedAt:      s.now(),
		UpdatedAt:      s.now(),
	}
	if err := s.repo.UpsertPlugin(ctx, rec); err != nil {
		return nil, err
	}
	_ = s.repo.AppendPluginAudit(ctx, &types.PluginAuditLog{
		Action:    types.PluginAuditPublish,
		PluginID:  m.ID,
		Version:   m.Version,
		Detail:    "submitted for review",
		Timestamp: s.now(),
	})
	return rec, nil
}

// ReviewPlugin moves a plugin through the editorial pipeline.
// Only admins call this; auth enforcement is left to the handler.
func (s *Service) ReviewPlugin(ctx context.Context, pluginID, version string, status types.PluginReviewStatus, reviewerNote string, actorID string) error {
	if !status.IsValid() {
		return ErrInvalidManifest
	}
	if err := s.repo.UpdatePluginStatus(ctx, pluginID, version, status, reviewerNote); err != nil {
		return err
	}
	_ = s.repo.AppendPluginAudit(ctx, &types.PluginAuditLog{
		ActorID:   actorID,
		Action:    types.PluginAuditReview,
		PluginID:  pluginID,
		Version:   version,
		Detail:    reviewerNote,
		Timestamp: s.now(),
	})
	return nil
}

// --- Catalog ---

// ListCatalog returns every plugin currently in the public
// marketplace catalog (status == published).
func (s *Service) ListCatalog(ctx context.Context, limit int) ([]*types.PluginRecord, error) {
	return s.repo.ListPlugins(ctx, types.PluginReviewPublished, limit)
}

// ListByStatus returns every plugin with the given editorial status
// (admin only).
func (s *Service) ListByStatus(ctx context.Context, status types.PluginReviewStatus, limit int) ([]*types.PluginRecord, error) {
	return s.repo.ListPlugins(ctx, status, limit)
}

// ListVersions returns every version of one plugin, newest first.
func (s *Service) ListVersions(ctx context.Context, pluginID string) ([]*types.PluginRecord, error) {
	return s.repo.ListVersionsByPlugin(ctx, pluginID)
}

// --- Tenant installs ---

// Install enables a plugin for a tenant. Refuses if the plugin is
// not published, or if the manifest's permission set includes
// anything the tenant admin has not allowed (caller pre-filters).
func (s *Service) Install(ctx context.Context, tenantID uint64, pluginID, version, installedBy string, grantedPermissions []string) (*types.TenantPlugin, error) {
	rec, err := s.repo.GetPlugin(ctx, pluginID, version)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrPluginNotFound
	}
	if !rec.Status.IsPublic() {
		return nil, ErrPluginNotPublic
	}
	existing, err := s.repo.GetTenantPlugin(ctx, tenantID, pluginID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, ErrAlreadyInstalled
	}
	granted := filterGranted(rec.Permissions, grantedPermissions)
	tp := &types.TenantPlugin{
		TenantID:    tenantID,
		PluginID:    pluginID,
		Version:     version,
		Enabled:     true,
		Permissions: granted,
		InstalledBy: installedBy,
		InstalledAt: s.now(),
		UpdatedAt:   s.now(),
	}
	if err := s.repo.UpsertTenantPlugin(ctx, tp); err != nil {
		return nil, err
	}
	_ = s.repo.IncrementDownloads(ctx, pluginID, version)
	_ = s.repo.AppendPluginAudit(ctx, &types.PluginAuditLog{
		TenantID:   tenantID,
		ActorID:    installedBy,
		Action:     types.PluginAuditInstall,
		PluginID:   pluginID,
		Version:    version,
		Detail:     "tenant install",
		Timestamp:  s.now(),
	})
	return tp, nil
}

// Uninstall removes a tenant's plugin.
func (s *Service) Uninstall(ctx context.Context, tenantID uint64, pluginID, actorID string) error {
	if err := s.repo.DeleteTenantPlugin(ctx, tenantID, pluginID); err != nil {
		if errors.Is(err, errNotFound{}) {
			return ErrNotInstalled
		}
		return err
	}
	_ = s.repo.AppendPluginAudit(ctx, &types.PluginAuditLog{
		TenantID:   tenantID,
		ActorID:    actorID,
		Action:     types.PluginAuditUninstall,
		PluginID:   pluginID,
		Timestamp:  s.now(),
	})
	return nil
}

// ListInstalled returns every plugin installed by a tenant.
func (s *Service) ListInstalled(ctx context.Context, tenantID uint64) ([]*types.TenantPlugin, error) {
	return s.repo.ListTenantPlugins(ctx, tenantID)
}

// GetInstalled returns one tenant plugin install.
func (s *Service) GetInstalled(ctx context.Context, tenantID uint64, pluginID string) (*types.TenantPlugin, error) {
	tp, err := s.repo.GetTenantPlugin(ctx, tenantID, pluginID)
	if err != nil {
		return nil, err
	}
	if tp == nil {
		return nil, ErrNotInstalled
	}
	return tp, nil
}

// HasPermission reports whether the tenant has installed the plugin
// AND the install grants the named permission. Used by the runtime
// to gate plugin-injected API calls.
func (s *Service) HasPermission(ctx context.Context, tenantID uint64, pluginID, permission string) (bool, error) {
	tp, err := s.repo.GetTenantPlugin(ctx, tenantID, pluginID)
	if err != nil {
		return false, err
	}
	if tp == nil || !tp.Enabled {
		return false, nil
	}
	for _, p := range tp.Permissions {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

// --- Audit ---

// ListAudit returns the most recent audit entries for a tenant.
func (s *Service) ListAudit(ctx context.Context, tenantID uint64, limit int) ([]*types.PluginAuditLog, error) {
	return s.repo.ListPluginAudit(ctx, tenantID, limit)
}

// --- helpers ---

// filterGranted returns the intersection of the manifest permissions
// and the tenant-admin-granted subset. Empty granted means "deny all".
func filterGranted(manifest, granted []string) []string {
	if len(granted) == 0 {
		return nil
	}
	gm := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		gm[g] = struct{}{}
	}
	out := []string{}
	for _, m := range manifest {
		if _, ok := gm[m]; ok {
			out = append(out, m)
		}
	}
	return out
}

// keysEqual is a constant-time-ish comparison of two PEM blocks.
// Sufficient for matching public keys; the Signature field provides
// the actual cryptographic trust guarantee.
func keysEqual(a, b string) bool {
	strip := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "\n", "")
		s = strings.ReplaceAll(s, "\r", "")
		return s
	}
	return strip(a) == strip(b)
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
