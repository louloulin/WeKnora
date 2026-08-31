package marketplace

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// --- fakes ---

type fakeRepo struct {
	vendors        map[string]*types.PluginVendor
	plugins        map[string]*types.PluginRecord // key = plugin_id + "|" + version
	tenantPlugins  map[uint64]map[string]*types.TenantPlugin
	audits         []*types.PluginAuditLog
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		vendors:       make(map[string]*types.PluginVendor),
		plugins:       make(map[string]*types.PluginRecord),
		tenantPlugins: make(map[uint64]map[string]*types.TenantPlugin),
	}
}

func keyFor(pluginID, version string) string { return pluginID + "|" + version }
func tKey(tenantID uint64, pluginID string) string { return pluginID }

func (f *fakeRepo) UpsertVendor(ctx context.Context, v *types.PluginVendor) error {
	if v.ID == 0 {
		f.vendors[v.Slug] = v
	} else {
		f.vendors[v.Slug] = v
	}
	return nil
}
func (f *fakeRepo) GetVendorBySlug(ctx context.Context, slug string) (*types.PluginVendor, error) {
	if v, ok := f.vendors[slug]; ok {
		return v, nil
	}
	return nil, nil
}
func (f *fakeRepo) GetVendorByPublicKey(ctx context.Context, publicKey string) (*types.PluginVendor, error) {
	for _, v := range f.vendors {
		if v.PublicKey == publicKey {
			return v, nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) ListVendors(ctx context.Context) ([]*types.PluginVendor, error) {
	out := []*types.PluginVendor{}
	for _, v := range f.vendors {
		out = append(out, v)
	}
	return out, nil
}

func (f *fakeRepo) UpsertPlugin(ctx context.Context, p *types.PluginRecord) error {
	if p.ID == 0 {
		f.plugins[keyFor(p.PluginID, p.Version)] = p
	} else {
		f.plugins[keyFor(p.PluginID, p.Version)] = p
	}
	return nil
}
func (f *fakeRepo) GetPlugin(ctx context.Context, pluginID, version string) (*types.PluginRecord, error) {
	if p, ok := f.plugins[keyFor(pluginID, version)]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeRepo) ListPlugins(ctx context.Context, status types.PluginReviewStatus, limit int) ([]*types.PluginRecord, error) {
	out := []*types.PluginRecord{}
	for _, p := range f.plugins {
		if status == "" || p.Status == status {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListVersionsByPlugin(ctx context.Context, pluginID string) ([]*types.PluginRecord, error) {
	out := []*types.PluginRecord{}
	for _, p := range f.plugins {
		if p.PluginID == pluginID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeRepo) UpdatePluginStatus(ctx context.Context, pluginID, version string, status types.PluginReviewStatus, reviewerNote string) error {
	p, ok := f.plugins[keyFor(pluginID, version)]
	if !ok {
		return errors.New("not found")
	}
	p.Status = status
	p.ReviewerNote = reviewerNote
	return nil
}
func (f *fakeRepo) IncrementDownloads(ctx context.Context, pluginID, version string) error {
	p, ok := f.plugins[keyFor(pluginID, version)]
	if !ok {
		return errors.New("not found")
	}
	p.Downloads++
	return nil
}

func (f *fakeRepo) UpsertTenantPlugin(ctx context.Context, t *types.TenantPlugin) error {
	if f.tenantPlugins[t.TenantID] == nil {
		f.tenantPlugins[t.TenantID] = make(map[string]*types.TenantPlugin)
	}
	f.tenantPlugins[t.TenantID][tKey(t.TenantID, t.PluginID)] = t
	return nil
}
func (f *fakeRepo) DeleteTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) error {
	if f.tenantPlugins[tenantID] == nil {
		return errors.New("not found")
	}
	delete(f.tenantPlugins[tenantID], pluginID)
	return nil
}
func (f *fakeRepo) GetTenantPlugin(ctx context.Context, tenantID uint64, pluginID string) (*types.TenantPlugin, error) {
	if f.tenantPlugins[tenantID] == nil {
		return nil, nil
	}
	return f.tenantPlugins[tenantID][pluginID], nil
}
func (f *fakeRepo) ListTenantPlugins(ctx context.Context, tenantID uint64) ([]*types.TenantPlugin, error) {
	out := []*types.TenantPlugin{}
	for _, p := range f.tenantPlugins[tenantID] {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRepo) AppendPluginAudit(ctx context.Context, a *types.PluginAuditLog) error {
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) ListPluginAudit(ctx context.Context, tenantID uint64, limit int) ([]*types.PluginAuditLog, error) {
	out := []*types.PluginAuditLog{}
	for _, a := range f.audits {
		if a.TenantID == tenantID {
			out = append(out, a)
		}
	}
	return out, nil
}

var _ interfaces.MarketplaceRepository = (*fakeRepo)(nil)

// fakeStorage records uploads so tests can assert the artifact URL.
type fakeStorage struct{ uploaded map[string][]byte }

func (s *fakeStorage) Upload(ctx context.Context, key string, data []byte) (string, error) {
	if s.uploaded == nil {
		s.uploaded = map[string][]byte{}
	}
	s.uploaded[key] = data
	return "s3://" + key, nil
}

// --- crypto helpers ---

func newKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
	return key, pemStr
}

func resign(t *testing.T, m *types.PluginManifest, key *rsa.PrivateKey) {
	t.Helper()
	sig, err := Signer(m, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	m.Signature = sig
}

func newManifest(t *testing.T, key *rsa.PrivateKey, pubPEM, slug string) *types.PluginManifest {
	t.Helper()
	m := &types.PluginManifest{
		ID:               "weknora-slack-bridge",
		Name:             "Slack Bridge",
		Version:          "1.0.0",
		Description:      "Sync Slack threads into the KB",
		Author:           slug,
		AuthorPublicKey:  pubPEM,
		Permissions:      []string{"kb:read", "kb:write"},
		MinWeKnoraVersion: "0.7.34",
		EntryPoint:       "dist/index.js",
		ArtifactSHA256:   "deadbeef",
	}
	sig, err := Signer(m, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	m.Signature = sig
	return m
}

// --- tests ---

func newService(t *testing.T) (*Service, *fakeRepo, *fakeStorage) {
	t.Helper()
	repo := newFakeRepo()
	store := &fakeStorage{}
	return NewService(repo, store), repo, store
}

func registerVendor(t *testing.T, s *Service, slug, pubPEM string) {
	t.Helper()
	if err := s.RegisterVendor(context.Background(), &types.PluginVendor{
		Slug: slug, Name: slug, PublicKey: pubPEM,
	}); err != nil {
		t.Fatalf("register vendor: %v", err)
	}
}

func TestService_RegisterVendor_RejectsMissingFields(t *testing.T) {
	s, _, _ := newService(t)
	if err := s.RegisterVendor(context.Background(), &types.PluginVendor{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("want ErrInvalidManifest, got %v", err)
	}
}

func TestService_Publish_RejectsUnknownVendor(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	m := newManifest(t, key, pub, "ghost-vendor")
	if _, err := s.Publish(context.Background(), m, nil); !errors.Is(err, ErrVendorNotFound) {
		t.Fatalf("want ErrVendorNotFound, got %v", err)
	}
}

func TestService_Publish_RejectsKeyMismatch(t *testing.T) {
	s, _, _ := newService(t)
	key1, pub1 := newKey(t)
	_, pub2 := newKey(t) // different key
	registerVendor(t, s, "acme", pub1)
	m := newManifest(t, key1, pub2, "acme") // wrong key embedded
	if _, err := s.Publish(context.Background(), m, nil); !errors.Is(err, ErrUntrustedPlugin) {
		t.Fatalf("want ErrUntrustedPlugin, got %v", err)
	}
}

func TestService_Publish_RejectsTamperedManifest(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	// Tamper with the name AFTER signing.
	m.Name = "Evil Plugin"
	if _, err := s.Publish(context.Background(), m, nil); !errors.Is(err, ErrUntrustedPlugin) {
		t.Fatalf("want ErrUntrustedPlugin, got %v", err)
	}
}

func TestService_Publish_PersistsSubmittedRecord(t *testing.T) {
	s, repo, store := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	artifact := []byte("binary-blob")
	rec, err := s.Publish(context.Background(), m, artifact)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if rec.Status != types.PluginReviewSubmitted {
		t.Fatalf("status = %s, want submitted", rec.Status)
	}
	if _, ok := store.uploaded[m.ID+"/"+m.Version+".tar.gz"]; !ok {
		t.Fatal("artifact should have been uploaded")
	}
	if len(repo.audits) != 1 || repo.audits[0].Action != types.PluginAuditPublish {
		t.Fatal("expected publish audit entry")
	}
}

func TestService_Publish_SkipsUploadWhenArtifactEmpty(t *testing.T) {
	s, _, store := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	if _, err := s.Publish(context.Background(), m, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(store.uploaded) != 0 {
		t.Fatalf("artifact should not have been uploaded")
	}
}

func TestService_ReviewPlugin_UpdatesStatus(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	if err := s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin"); err != nil {
		t.Fatalf("review: %v", err)
	}
	got, _ := s.ListVersions(context.Background(), rec.PluginID)
	if got[0].Status != types.PluginReviewPublished {
		t.Fatalf("status not updated: %s", got[0].Status)
	}
}

func TestService_Install_RejectsUnpublishedPlugin(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	// Plugin is in submitted status, not published.
	_, err := s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})
	if !errors.Is(err, ErrPluginNotPublic) {
		t.Fatalf("want ErrPluginNotPublic, got %v", err)
	}
}

func TestService_Install_AcceptsPublishedPlugin(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	tp, err := s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read", "kb:write"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !tp.Enabled {
		t.Fatal("install should be enabled")
	}
	if len(tp.Permissions) != 2 {
		t.Fatalf("expected 2 permissions granted, got %d", len(tp.Permissions))
	}
}

func TestService_Install_NarrowsPermissionsByGrant(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	m.Permissions = []string{"kb:read", "kb:write", "kb:admin"}
	resign(t, m, key)
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	tp, _ := s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})
	if len(tp.Permissions) != 1 || tp.Permissions[0] != "kb:read" {
		t.Fatalf("permissions not narrowed: %v", tp.Permissions)
	}
}

func TestService_Install_DeniesWhenNoGrants(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	tp, _ := s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{})
	if len(tp.Permissions) != 0 {
		t.Fatalf("expected 0 permissions when grant is empty, got %d", len(tp.Permissions))
	}
}

func TestService_Install_RejectsDuplicate(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	_, _ = s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})
	_, err := s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("want ErrAlreadyInstalled, got %v", err)
	}
}

func TestService_Uninstall_RemovesRecord(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	_, _ = s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})
	if err := s.Uninstall(context.Background(), 1, rec.PluginID, "u1"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := s.GetInstalled(context.Background(), 1, rec.PluginID); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("want ErrNotInstalled, got %v", err)
	}
}

func TestService_HasPermission(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	m.Permissions = []string{"kb:read", "kb:write"}
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	_, _ = s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})

	ok, err := s.HasPermission(context.Background(), 1, rec.PluginID, "kb:read")
	if err != nil || !ok {
		t.Fatalf("kb:read should be granted, got ok=%v err=%v", ok, err)
	}
	ok, _ = s.HasPermission(context.Background(), 1, rec.PluginID, "kb:write")
	if ok {
		t.Fatal("kb:write should NOT be granted")
	}
	ok, _ = s.HasPermission(context.Background(), 1, rec.PluginID, "kb:admin")
	if ok {
		t.Fatal("kb:admin should NOT be granted")
	}
}

func TestService_HasPermission_NotInstalledReturnsFalse(t *testing.T) {
	s, _, _ := newService(t)
	ok, err := s.HasPermission(context.Background(), 999, "anything", "kb:read")
	if err != nil || ok {
		t.Fatalf("not-installed tenant should have no perms, got ok=%v err=%v", ok, err)
	}
}

func TestService_ListCatalog_OnlyPublished(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m1 := newManifest(t, key, pub, "acme")
	m1.Version = "1.0.0"
	m1.Signature = ""
	sig1, _ := Signer(m1, key)
	m1.Signature = sig1
	_, _ = s.Publish(context.Background(), m1, nil)
	// Second plugin, stays in submitted.
	m2 := newManifest(t, key, pub, "acme")
	m2.ID = "other-plugin"
	m2.Version = "1.0.0"
	_, _ = s.Publish(context.Background(), m2, nil)
	// Publish the first one.
	_ = s.ReviewPlugin(context.Background(), "weknora-slack-bridge", "1.0.0", types.PluginReviewPublished, "ok", "admin")

	cat, err := s.ListCatalog(context.Background(), 100)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(cat) != 1 {
		t.Fatalf("expected 1 published plugin, got %d", len(cat))
	}
}

func TestService_ListAudit_ByTenant(t *testing.T) {
	s, _, _ := newService(t)
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, _ := s.Publish(context.Background(), m, nil)
	_ = s.ReviewPlugin(context.Background(), rec.PluginID, rec.Version, types.PluginReviewPublished, "ok", "admin")
	_, _ = s.Install(context.Background(), 1, rec.PluginID, rec.Version, "u1", []string{"kb:read"})

	out, err := s.ListAudit(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	// Should have install + uninstall? we did install only.
	if len(out) < 1 {
		t.Fatal("expected at least 1 audit entry")
	}
	for _, a := range out {
		if a.TenantID != 1 {
			t.Fatalf("foreign audit: %+v", a)
		}
	}
}

func TestService_TimestampFrozenOnPublish(t *testing.T) {
	s, _, _ := newService(t)
	s.SetNow(func() time.Time { return time.Unix(1234567890, 0).UTC() })
	key, pub := newKey(t)
	registerVendor(t, s, "acme", pub)
	m := newManifest(t, key, pub, "acme")
	rec, err := s.Publish(context.Background(), m, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !rec.SubmittedAt.Equal(time.Unix(1234567890, 0).UTC()) {
		t.Fatalf("submitted_at = %v, want frozen", rec.SubmittedAt)
	}
}

func TestVerifySignature_RejectsEmpty(t *testing.T) {
	m := &types.PluginManifest{}
	if err := VerifySignature(m); err == nil {
		t.Fatal("expected error for empty signature")
	}
}

func TestVerifySignature_RejectsNonPEM(t *testing.T) {
	m := &types.PluginManifest{
		AuthorPublicKey: "not a pem block",
		Signature:       "AAAA",
	}
	if err := VerifySignature(m); err == nil {
		t.Fatal("expected error for non-PEM key")
	}
}

func TestPluginManifest_CanonicalBytesStable(t *testing.T) {
	m1 := &types.PluginManifest{ID: "x", Version: "1", Name: "n"}
	m2 := &types.PluginManifest{ID: "x", Version: "1", Name: "n"}
	m1.Signature = "AAA"
	m2.Signature = "BBB"
	b1, _ := m1.CanonicalBytes()
	b2, _ := m2.CanonicalBytes()
	if string(b1) != string(b2) {
		t.Fatalf("canonical bytes depend on signature")
	}
}

func TestFilterGranted(t *testing.T) {
	out := filterGranted([]string{"a", "b", "c"}, []string{"a", "c"})
	if len(out) != 2 || out[0] != "a" || out[1] != "c" {
		t.Fatalf("unexpected: %v", out)
	}
	if filterGranted([]string{"a"}, []string{}) != nil {
		t.Fatal("empty grant should return nil")
	}
}

func TestKeysEqual(t *testing.T) {
	if !keysEqual("  abc\n", "abc") {
		t.Fatal("trimmed keys should be equal")
	}
	if keysEqual("abc", "def") {
		t.Fatal("different keys should NOT be equal")
	}
}
