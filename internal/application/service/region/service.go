package region

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Service is the application service for region management. It
// coordinates the Resolver cache with the persistent bindings and
// writes cross-region audit entries.
type Service struct {
	repo     interfaces.RegionRepository
	resolver *Resolver
	now      func() time.Time
}

// NewService constructs a region Service.
func NewService(repo interfaces.RegionRepository, resolver *Resolver) *Service {
	return &Service{repo: repo, resolver: resolver, now: func() time.Time { return time.Now().UTC() }}
}

// SetNow lets tests freeze time.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// --- Region catalog ---

// ListRegions returns every region record, sorted by id.
func (s *Service) ListRegions(ctx context.Context) ([]*types.RegionRecord, error) {
	return s.repo.ListRegions(ctx)
}

// GetRegion returns one region record by id.
func (s *Service) GetRegion(ctx context.Context, id string) (*types.RegionRecord, error) {
	if !types.Region(id).IsValid() {
		return nil, ErrInvalidRegion
	}
	return s.repo.GetRegion(ctx, id)
}

// UpsertRegion creates or updates a region catalog entry. Operators
// use this to mark a region as degraded or to update capacity
// telemetry.
func (s *Service) UpsertRegion(ctx context.Context, rec *types.RegionRecord) error {
	if !types.Region(rec.ID).IsValid() {
		return ErrInvalidRegion
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = s.now()
	}
	rec.UpdatedAt = s.now()
	return s.repo.UpsertRegion(ctx, rec)
}

// --- Tenant bindings ---

// GetTenantBinding returns the current binding (or nil if none).
func (s *Service) GetTenantBinding(ctx context.Context, tenantID uint64) (*types.TenantRegionBinding, error) {
	return s.repo.GetTenantBinding(ctx, tenantID)
}

// BindTenant installs (or replaces) the region binding for a tenant.
// It validates region + policy combinations and reloads the resolver
// cache so the change is visible to the middleware immediately.
func (s *Service) BindTenant(ctx context.Context, b *types.TenantRegionBinding) error {
	if !b.PrimaryRegion.IsValid() {
		return ErrInvalidRegion
	}
	if b.ResidencyPolicy == "" {
		b.ResidencyPolicy = types.ResidencyStrictLocal
	}
	if !b.ResidencyPolicy.IsValid() {
		return ErrInvalidRegion
	}
	for _, reg := range b.ReplicaRegions {
		if !reg.IsValid() {
			return ErrInvalidRegion
		}
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = s.now()
	}
	b.UpdatedAt = s.now()
	if err := s.repo.UpsertTenantBinding(ctx, b); err != nil {
		return err
	}
	return s.resolver.ReloadBindings(ctx)
}

// UnbindTenant removes the binding and reloads the cache.
func (s *Service) UnbindTenant(ctx context.Context, tenantID uint64) error {
	if err := s.repo.DeleteTenantBinding(ctx, tenantID); err != nil {
		return err
	}
	return s.resolver.ReloadBindings(ctx)
}

// --- Cross-region audit ---

// AuditCrossRegion records a cross-region action. Used by the
// RegionEnforcer middleware and by admin tools that trigger manual
// replication.
func (s *Service) AuditCrossRegion(ctx context.Context, log *types.CrossRegionAuditLog) error {
	if log.Timestamp.IsZero() {
		log.Timestamp = s.now()
	}
	return s.repo.AppendAudit(ctx, log)
}

// ListAuditByTenant returns the most recent audit entries for a tenant.
func (s *Service) ListAuditByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.CrossRegionAuditLog, error) {
	return s.repo.ListAuditByTenant(ctx, tenantID, limit)
}

// ListDeniedAudit returns the most recent denied entries (admin only).
func (s *Service) ListDeniedAudit(ctx context.Context, limit int) ([]*types.CrossRegionAuditLog, error) {
	return s.repo.ListDeniedAudit(ctx, limit)
}

// Resolve exposes the underlying Resolver to handlers/middleware that
// need to look up the region for a request.
func (s *Service) Resolve(ctx context.Context, tenantID uint64, ip string) (types.Region, error) {
	return s.resolver.Resolve(ctx, tenantID, ip)
}

// ComplianceGate delegates to the Resolver.
func (s *Service) ComplianceGate(ctx context.Context, tenantID uint64, src, dst types.Region) error {
	return s.resolver.ComplianceGate(ctx, tenantID, src, dst)
}

// ErrNotFound is returned when a region is not found.
var ErrNotFound = errors.New("region: not found")
