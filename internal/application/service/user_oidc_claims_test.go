package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestOIDCTenantRoleUsesExplicitPermissionsOnly(t *testing.T) {
	cfg := &config.OIDCAuthConfig{
		PlatformAdminPermission:       "weknora.platform.admin",
		WorkspaceReadPermission:       "weknora.workspace.read",
		WorkspaceContributePermission: "weknora.workspace.contribute",
		WorkspaceAdminPermission:      "weknora.workspace.admin",
		WorkspaceOwnerPermission:      "weknora.workspace.owner",
	}

	if got := oidcTenantRole(map[string]struct{}{"isadmin": {}}, cfg); got != "" {
		t.Fatalf("isAdmin claim must not grant a tenant role, got %q", got)
	}
	if got := oidcTenantRole(map[string]struct{}{"evil/weknora.workspace.read": {}}, cfg); got != "" {
		t.Fatalf("lookalike permission must not grant a tenant role, got %q", got)
	}
	if got := oidcTenantRole(map[string]struct{}{cfg.WorkspaceReadPermission: {}}, cfg); got != types.TenantRoleViewer {
		t.Fatalf("read permission mapped to %q, want viewer", got)
	}
	if got := oidcTenantRole(map[string]struct{}{cfg.WorkspaceOwnerPermission: {}, cfg.WorkspaceAdminPermission: {}}, cfg); got != types.TenantRoleOwner {
		t.Fatalf("highest explicit permission mapped to %q, want owner", got)
	}
}

func TestOIDCStringClaims(t *testing.T) {
	got := oidcStringClaims([]interface{}{"org-a", "org-b,org-c"})
	if len(got) != 3 || got[0] != "org-a" || got[2] != "org-c" {
		t.Fatalf("unexpected claim values: %#v", got)
	}
}
