package region

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ErrResidencyViolation is returned when a cross-region action would
// violate the tenant's data-residency policy.
var ErrResidencyViolation = errors.New("region: residency policy violation")

// ErrInvalidRegion is returned when a request asks for an unknown region.
var ErrInvalidRegion = errors.New("region: invalid region")

// GeoIP is the minimal interface the resolver needs from the GeoIP
// lookup. Implemented by internal/geoiplookup.CountryResolver.
type GeoIP interface {
	Lookup(ip string) (string, error) // returns ISO country code
}

// Resolver maps (tenant, ip) → region and enforces residency policy.
// It caches tenant bindings in memory for fast middleware access and
// reloads them periodically to pick up admin updates without restart.
type Resolver struct {
	repo      interfaces.RegionRepository
	geoIP     GeoIP
	defaultR  types.Region

	mu        sync.RWMutex
	bindings  map[uint64]*types.TenantRegionBinding
	loadedAt  time.Time
	cacheTTL  time.Duration
}

// NewResolver constructs a region Resolver.
func NewResolver(repo interfaces.RegionRepository, geoIP GeoIP, defaultRegion types.Region) *Resolver {
	return &Resolver{
		repo:     repo,
		geoIP:    geoIP,
		defaultR: defaultRegion,
		bindings: make(map[uint64]*types.TenantRegionBinding),
		cacheTTL: 30 * time.Second,
	}
}

// ReloadBindings refreshes the in-memory tenant-binding cache from the
// repository. Safe to call from a goroutine on a timer.
func (r *Resolver) ReloadBindings(ctx context.Context) error {
	all, err := r.repo.ListAllTenantBindings(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings = make(map[uint64]*types.TenantRegionBinding, len(all))
	for _, b := range all {
		r.bindings[b.TenantID] = b
	}
	r.loadedAt = time.Now()
	return nil
}

// ensureFresh reloads the cache if it's older than TTL.
func (r *Resolver) ensureFresh(ctx context.Context) {
	r.mu.RLock()
	stale := time.Since(r.loadedAt) > r.cacheTTL
	r.mu.RUnlock()
	if stale {
		_ = r.ReloadBindings(ctx)
	}
}

// Resolve returns the target region for a request. Priority is:
//  1. Tenant explicit binding (TenantRegionBinding.PrimaryRegion)
//  2. GeoIP lookup → nearest region
//  3. Default region (config)
func (r *Resolver) Resolve(ctx context.Context, tenantID uint64, ip string) (types.Region, error) {
	r.ensureFresh(ctx)
	r.mu.RLock()
	if b, ok := r.bindings[tenantID]; ok && b != nil {
		r.mu.RUnlock()
		return b.PrimaryRegion, nil
	}
	r.mu.RUnlock()

	if r.geoIP != nil && ip != "" {
		country, err := r.geoIP.Lookup(ip)
		if err == nil && country != "" {
			if reg := regionForCountry(country); reg != "" {
				return types.Region(reg), nil
			}
		}
	}
	if !r.defaultR.IsValid() {
		return "", ErrInvalidRegion
	}
	return r.defaultR, nil
}

// ComplianceGate returns nil when src→dst is permitted by the
// tenant's residency policy. Returns ErrResidencyViolation otherwise.
func (r *Resolver) ComplianceGate(ctx context.Context, tenantID uint64, src, dst types.Region) error {
	r.ensureFresh(ctx)
	r.mu.RLock()
	binding, ok := r.bindings[tenantID]
	r.mu.RUnlock()
	if !ok || binding == nil {
		// No explicit binding = open posture (same as the legacy
		// single-region behaviour). Operators can opt-in to strict
		// mode by setting a binding.
		return nil
	}
	if !binding.ResidencyPolicy.AllowsCrossRegion(src, dst) {
		return ErrResidencyViolation
	}
	return nil
}

// regionForCountry returns the closest supported region for an ISO
// country code. Mapping is intentionally simple — production
// deployments will provide a richer GeoIP→region config.
func regionForCountry(cc string) string {
	switch cc {
	case "DE", "FR", "IT", "ES", "NL", "PL", "SE", "FI", "DK", "NO", "BE", "AT", "IE", "PT", "CZ", "HU", "RO", "GR", "CH":
		return string(types.RegionEU)
	case "JP", "KR", "CN", "HK", "TW", "SG", "MY", "TH", "VN", "PH", "ID", "IN", "AU", "NZ":
		return string(types.RegionAPAC)
	case "US", "CA", "MX":
		return string(types.RegionUSEast)
	default:
		return ""
	}
}
