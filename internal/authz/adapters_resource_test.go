package authz

import (
	"context"
	"errors"
	"testing"
)

// KB adapter tests --------------------------------------------------------

func TestKBAdapter_OwnerAlwaysAllowed(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 1, "alice", true, nil
		},
		nil,
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationDelete,
	})
	if !d.Allowed {
		t.Errorf("creator should be allowed delete, got %+v", d)
	}
}

func TestKBAdapter_SameTenantRoleMatrix(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 1, "alice", true, nil
		},
		nil,
	))
	cases := []struct {
		role     string
		relation Relation
		want     bool
	}{
		{"viewer", RelationViewer, true},
		{"viewer", RelationEditor, false},
		{"contributor", RelationEditor, true},
		{"admin", RelationAdmin, true},
		{"contributor", RelationOwner, false},
	}
	for _, tc := range cases {
		req := CheckRequest{
			User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: tc.role},
			Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
			Relation: tc.relation,
		}
		d := c.Check(context.Background(), req)
		if d.Allowed != tc.want {
			t.Errorf("role=%s rel=%s: got %v want %v (code=%s)", tc.role, tc.relation, d.Allowed, tc.want, d.Code)
		}
	}
}

func TestKBAdapter_CrossTenantShare(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 1, "alice", true, nil
		},
		func(_ context.Context, _ string, _ uint64, role string) (string, bool, error) {
			if role == "viewer" {
				return "viewer", true, nil
			}
			return "editor", true, nil
		},
	))
	// Caller in tenant 2 with viewer role should be denied editor.
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 2, Role: "viewer"},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationEditor,
	})
	if d.Allowed {
		t.Errorf("viewer-share should not allow editor, got %+v", d)
	}
	if d.Code != CodeRoleTooLow {
		t.Errorf("expected CodeRoleTooLow, got %s", d.Code)
	}
	d = c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 2, Role: "viewer"},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationViewer,
	})
	if !d.Allowed {
		t.Errorf("viewer-share should allow viewer, got %+v", d)
	}
}

func TestKBAdapter_NoShareMeansDeny(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 1, "alice", true, nil
		},
		func(_ context.Context, _ string, _ uint64, _ string) (string, bool, error) {
			return "", false, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 2, Role: "owner"},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationViewer,
	})
	if d.Allowed {
		t.Errorf("no share should deny, got %+v", d)
	}
	if d.Code != CodeNotShared {
		t.Errorf("expected CodeNotShared, got %s", d.Code)
	}
}

func TestKBAdapter_MissingReturns404(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 0, "", false, nil
		},
		nil,
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeKB, ID: "missing", TenantID: 1},
		Relation: RelationViewer,
	})
	if d.Code != CodeNoSuchResource {
		t.Errorf("expected CodeNoSuchResource, got %s", d.Code)
	}
}

func TestKBAdapter_LookupErrorIsConservativeDeny(t *testing.T) {
	c := NewCompositeChecker(NewKBAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 0, "", false, errors.New("db down")
		},
		nil,
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationViewer,
	})
	if d.Allowed {
		t.Errorf("lookup error should deny, got %+v", d)
	}
	if d.Code != CodeError {
		t.Errorf("expected CodeError, got %s", d.Code)
	}
}

// Wiki page adapter tests -------------------------------------------------

func TestWikiPageAdapter_OwnerShortCircuit(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		nil, // ResolveLookup not hit when owner matches
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationViewer,
	})
	if !d.Allowed {
		t.Errorf("owner should be allowed, got %+v", d)
	}
}

func TestWikiPageAdapter_AdminBypass(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		nil,
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "admin1", TenantID: 1, Role: "admin"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationDelete,
	})
	if !d.Allowed {
		t.Errorf("admin should bypass ACL, got %+v", d)
	}
}

func TestWikiPageAdapter_ACLAllowListReject(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		func(_ context.Context, _, _, _ string) (string, bool, error) {
			return "deny_allow_list", true, nil
		},
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "contributor"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationViewer,
	})
	if d.Allowed {
		t.Errorf("deny_allow_list should deny, got %+v", d)
	}
	if d.Code != CodeNotShared {
		t.Errorf("expected CodeNotShared, got %s", d.Code)
	}
}

func TestWikiPageAdapter_ACLPrivateDeny(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		func(_ context.Context, _, _, _ string) (string, bool, error) {
			return "deny_private", true, nil
		},
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "contributor"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationViewer,
	})
	if d.Allowed {
		t.Errorf("deny_private should deny, got %+v", d)
	}
}

func TestWikiPageAdapter_NoACLRowHidesExistence(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		func(_ context.Context, _, _, _ string) (string, bool, error) {
			return "", false, nil // page missing or no ACL row
		},
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "contributor"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationViewer,
	})
	if d.Code != CodeNoSuchResource {
		t.Errorf("no ACL + not owner/admin should 404, got %s", d.Code)
	}
}

func TestWikiPageAdapter_ACLAllow(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		func(_ context.Context, _, _, _ string) (string, bool, error) {
			return "allow", true, nil
		},
		func(_ context.Context, _, slug string) (string, uint64, bool, error) {
			return "alice", 1, true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "viewer"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "kb1/slug"},
		Relation: RelationViewer,
	})
	if !d.Allowed {
		t.Errorf("ACL allow should permit, got %+v", d)
	}
}

func TestWikiPageAdapter_BadObjectIDIsError(t *testing.T) {
	c := NewCompositeChecker(NewWikiPageAdapter(
		nil,
		nil,
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "contributor"},
		Object:   Object{Type: ObjectTypeWikiPage, ID: "no-slash"},
		Relation: RelationViewer,
	})
	if d.Code != CodeError {
		t.Errorf("malformed id should be CodeError, got %s", d.Code)
	}
}

// Agent adapter tests -----------------------------------------------------

func TestAgentAdapter_OwnerAndRole(t *testing.T) {
	c := NewCompositeChecker(NewAgentAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 7, "alice", true, nil
		},
		nil,
	))
	t.Run("owner", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "alice", TenantID: 7},
			Object:   Object{Type: ObjectTypeAgent, ID: "ag1", TenantID: 7},
			Relation: RelationDelete,
		})
		if !d.Allowed {
			t.Errorf("owner should allow, got %+v", d)
		}
	})
	t.Run("non-owner-contributor", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "bob", TenantID: 7, Role: "contributor"},
			Object:   Object{Type: ObjectTypeAgent, ID: "ag1", TenantID: 7},
			Relation: RelationEditor,
		})
		if !d.Allowed {
			t.Errorf("contributor should allow editor, got %+v", d)
		}
	})
}

func TestAgentAdapter_CrossTenantShare(t *testing.T) {
	c := NewCompositeChecker(NewAgentAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 7, "alice", true, nil
		},
		func(_ context.Context, _ string, _ uint64, role string) (string, bool, error) {
			if role == "viewer" {
				return "viewer", true, nil
			}
			return "editor", true, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 9, Role: "viewer"},
		Object:   Object{Type: ObjectTypeAgent, ID: "ag1", TenantID: 7},
		Relation: RelationEditor,
	})
	if d.Allowed {
		t.Errorf("viewer-share should not allow editor, got %+v", d)
	}
	if d.Code != CodeRoleTooLow {
		t.Errorf("expected CodeRoleTooLow, got %s", d.Code)
	}
}

func TestAgentAdapter_NoShareMeansDeny(t *testing.T) {
	c := NewCompositeChecker(NewAgentAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 7, "alice", true, nil
		},
		func(_ context.Context, _ string, _ uint64, _ string) (string, bool, error) {
			return "", false, nil
		},
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 9, Role: "owner"},
		Object:   Object{Type: ObjectTypeAgent, ID: "ag1", TenantID: 7},
		Relation: RelationViewer,
	})
	if d.Code != CodeNotShared {
		t.Errorf("expected CodeNotShared, got %s", d.Code)
	}
}

func TestAgentAdapter_MissingReturns404(t *testing.T) {
	c := NewCompositeChecker(NewAgentAdapter(
		func(_ context.Context, id string) (uint64, string, bool, error) {
			return 0, "", false, nil
		},
		nil,
	))
	d := c.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeAgent, ID: "missing"},
		Relation: RelationViewer,
	})
	if d.Code != CodeNoSuchResource {
		t.Errorf("expected CodeNoSuchResource, got %s", d.Code)
	}
}
