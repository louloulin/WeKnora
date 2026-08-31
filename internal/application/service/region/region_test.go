package region

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- fake repo ---

type fakeRegionRepo struct {
	regions   map[string]*types.RegionRecord
	bindings  map[uint64]*types.TenantRegionBinding
	audits    []*types.CrossRegionAuditLog
	failAudit bool
}

func newFakeRegionRepo() *fakeRegionRepo {
	return &fakeRegionRepo{
		regions:  make(map[string]*types.RegionRecord),
		bindings: make(map[uint64]*types.TenantRegionBinding),
	}
}

func (f *fakeRegionRepo) ListRegions(ctx context.Context) ([]*types.RegionRecord, error) {
	out := make([]*types.RegionRecord, 0, len(f.regions))
	for _, r := range f.regions {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeRegionRepo) GetRegion(ctx context.Context, id string) (*types.RegionRecord, error) {
	r, ok := f.regions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}
func (f *fakeRegionRepo) UpsertRegion(ctx context.Context, r *types.RegionRecord) error {
	f.regions[r.ID] = r
	return nil
}
func (f *fakeRegionRepo) GetTenantBinding(ctx context.Context, tenantID uint64) (*types.TenantRegionBinding, error) {
	return f.bindings[tenantID], nil
}
func (f *fakeRegionRepo) UpsertTenantBinding(ctx context.Context, b *types.TenantRegionBinding) error {
	f.bindings[b.TenantID] = b
	return nil
}
func (f *fakeRegionRepo) DeleteTenantBinding(ctx context.Context, tenantID uint64) error {
	delete(f.bindings, tenantID)
	return nil
}
func (f *fakeRegionRepo) ListAllTenantBindings(ctx context.Context) ([]*types.TenantRegionBinding, error) {
	out := make([]*types.TenantRegionBinding, 0, len(f.bindings))
	for _, b := range f.bindings {
		out = append(out, b)
	}
	return out, nil
}
func (f *fakeRegionRepo) AppendAudit(ctx context.Context, l *types.CrossRegionAuditLog) error {
	if f.failAudit {
		return errors.New("audit boom")
	}
	f.audits = append(f.audits, l)
	return nil
}
func (f *fakeRegionRepo) ListAuditByTenant(ctx context.Context, tenantID uint64, limit int) ([]*types.CrossRegionAuditLog, error) {
	out := make([]*types.CrossRegionAuditLog, 0)
	for _, a := range f.audits {
		if a.TenantID == tenantID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRegionRepo) ListDeniedAudit(ctx context.Context, limit int) ([]*types.CrossRegionAuditLog, error) {
	out := make([]*types.CrossRegionAuditLog, 0)
	for _, a := range f.audits {
		if !a.Allowed {
			out = append(out, a)
		}
	}
	return out, nil
}

// Compile-time check.
var _ interfaces.RegionRepository = (*fakeRegionRepo)(nil)

// --- fake GeoIP ---

type fakeGeoIP struct {
	mapping map[string]string
}

func (f *fakeGeoIP) Lookup(ip string) (string, error) {
	if c, ok := f.mapping[ip]; ok {
		return c, nil
	}
	return "", errors.New("not found")
}

// --- tests ---

func TestResolver_PrefersTenantBindingOverGeoIP(t *testing.T) {
	repo := newFakeRegionRepo()
	geo := &fakeGeoIP{mapping: map[string]string{"1.2.3.4": "DE"}}
	r := NewResolver(repo, geo, types.RegionUSEast)
	ctx := context.Background()

	if err := r.ReloadBindings(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	repo.bindings[42] = &types.TenantRegionBinding{
		TenantID:      42,
		PrimaryRegion: types.RegionAPAC,
	}
	if err := r.ReloadBindings(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}

	got, err := r.Resolve(ctx, 42, "1.2.3.4")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != types.RegionAPAC {
		t.Fatalf("want APAC, got %s", got)
	}
}

func TestResolver_GeoIPFallback(t *testing.T) {
	repo := newFakeRegionRepo()
	geo := &fakeGeoIP{mapping: map[string]string{"5.6.7.8": "FR"}}
	r := NewResolver(repo, geo, types.RegionUSEast)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)

	got, err := r.Resolve(ctx, 999, "5.6.7.8")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != types.RegionEU {
		t.Fatalf("want EU, got %s", got)
	}
}

func TestResolver_FallsBackToDefault(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionDev)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)

	got, err := r.Resolve(ctx, 7, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != types.RegionDev {
		t.Fatalf("want dev-local, got %s", got)
	}
}

func TestComplianceGate_StrictLocalBlocksCrossRegion(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	ctx := context.Background()

	if err := r.ReloadBindings(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	repo.bindings[1] = &types.TenantRegionBinding{
		TenantID:        1,
		PrimaryRegion:   types.RegionEU,
		ResidencyPolicy: types.ResidencyStrictLocal,
	}
	if err := r.ReloadBindings(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}

	err := r.ComplianceGate(ctx, 1, types.RegionEU, types.RegionUSEast)
	if !errors.Is(err, ErrResidencyViolation) {
		t.Fatalf("want ErrResidencyViolation, got %v", err)
	}
	// Same-region is allowed.
	if err := r.ComplianceGate(ctx, 1, types.RegionEU, types.RegionEU); err != nil {
		t.Fatalf("same-region should pass: %v", err)
	}
}

func TestComplianceGate_EUOnlyBlocksAPAC(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionEU)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)
	repo.bindings[2] = &types.TenantRegionBinding{
		TenantID:        2,
		PrimaryRegion:   types.RegionEU,
		ResidencyPolicy: types.ResidencyEUOnly,
	}
	_ = r.ReloadBindings(ctx)

	if err := r.ComplianceGate(ctx, 2, types.RegionEU, types.RegionAPAC); !errors.Is(err, ErrResidencyViolation) {
		t.Fatalf("EU→APAC should be blocked: %v", err)
	}
	if err := r.ComplianceGate(ctx, 2, types.RegionEU, types.RegionEU); err != nil {
		t.Fatalf("EU→EU should pass: %v", err)
	}
}

func TestComplianceGate_GlobalAllowsAll(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)
	repo.bindings[3] = &types.TenantRegionBinding{
		TenantID:        3,
		PrimaryRegion:   types.RegionUSEast,
		ResidencyPolicy: types.ResidencyGlobal,
	}
	_ = r.ReloadBindings(ctx)

	for _, dst := range []types.Region{types.RegionEU, types.RegionAPAC, types.RegionUSWest} {
		if err := r.ComplianceGate(ctx, 3, types.RegionUSEast, dst); err != nil {
			t.Fatalf("global should allow EU→%s: %v", dst, err)
		}
	}
}

func TestComplianceGate_NoBindingAllows(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)

	// Tenant with no binding — open posture.
	if err := r.ComplianceGate(ctx, 999, types.RegionUSEast, types.RegionEU); err != nil {
		t.Fatalf("no-binding should allow: %v", err)
	}
}

func TestService_BindTenantRejectsInvalidRegion(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	s := NewService(repo, r)
	ctx := context.Background()

	err := s.BindTenant(ctx, &types.TenantRegionBinding{
		TenantID:      100,
		PrimaryRegion: types.Region("atlantis"),
	})
	if !errors.Is(err, ErrInvalidRegion) {
		t.Fatalf("want ErrInvalidRegion, got %v", err)
	}
}

func TestService_BindTenantRejectsInvalidPolicy(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	s := NewService(repo, r)
	ctx := context.Background()

	err := s.BindTenant(ctx, &types.TenantRegionBinding{
		TenantID:        100,
		PrimaryRegion:   types.RegionEU,
		ResidencyPolicy: types.DataResidencyPolicy("freedom"),
	})
	if !errors.Is(err, ErrInvalidRegion) {
		t.Fatalf("want ErrInvalidRegion, got %v", err)
	}
}

func TestService_BindThenUnbindReloadsCache(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	s := NewService(repo, r)
	ctx := context.Background()
	_ = r.ReloadBindings(ctx)

	// Initially tenant 9 has no binding → default region.
	got, _ := r.Resolve(ctx, 9, "")
	if got != types.RegionUSEast {
		t.Fatalf("expected default, got %s", got)
	}

	if err := s.BindTenant(ctx, &types.TenantRegionBinding{
		TenantID:      9,
		PrimaryRegion: types.RegionAPAC,
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	got, _ = r.Resolve(ctx, 9, "")
	if got != types.RegionAPAC {
		t.Fatalf("expected APAC after bind, got %s", got)
	}

	if err := s.UnbindTenant(ctx, 9); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	got, _ = r.Resolve(ctx, 9, "")
	if got != types.RegionUSEast {
		t.Fatalf("expected default after unbind, got %s", got)
	}
}

func TestService_AuditCrossRegionPersistsLog(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	s := NewService(repo, r)
	ctx := context.Background()

	if err := s.AuditCrossRegion(ctx, &types.CrossRegionAuditLog{
		SourceRegion: types.RegionEU,
		TargetRegion: types.RegionUSEast,
		TenantID:     1,
		UserID:       "u1",
		Action:       types.CrossRegionActionRead,
		Allowed:      true,
	}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(repo.audits) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(repo.audits))
	}
	if repo.audits[0].Timestamp.IsZero() {
		t.Fatalf("timestamp should be auto-populated")
	}
}

func TestService_AuditCrossRegionAutoStampsTimestamp(t *testing.T) {
	repo := newFakeRegionRepo()
	r := NewResolver(repo, nil, types.RegionUSEast)
	s := NewService(repo, r)
	s.SetNow(func() time.Time { return time.Unix(1234567890, 0).UTC() })
	ctx := context.Background()

	if err := s.AuditCrossRegion(ctx, &types.CrossRegionAuditLog{
		SourceRegion: types.RegionEU,
		TargetRegion: types.RegionUSEast,
		TenantID:     1,
		Allowed:      false,
	}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !repo.audits[0].Timestamp.Equal(time.Unix(1234567890, 0).UTC()) {
		t.Fatalf("expected frozen time, got %v", repo.audits[0].Timestamp)
	}
}

func TestRegion_IsValid(t *testing.T) {
	if !types.RegionEU.IsValid() {
		t.Fatal("EU must be valid")
	}
	if types.Region("atlantis").IsValid() {
		t.Fatal("atlantis must be invalid")
	}
}

func TestPolicy_IsValid(t *testing.T) {
	if !types.ResidencyStrictLocal.IsValid() {
		t.Fatal("strict_local must be valid")
	}
	if types.DataResidencyPolicy("freedom").IsValid() {
		t.Fatal("freedom must be invalid")
	}
}
