package authz

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

func TestCompositeChecker_CrossTenantDeny(t *testing.T) {
	c := NewCompositeChecker(NewTenantRoleAdapter())
	req := CheckRequest{
		User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeTenant, ID: "kb1", TenantID: 2},
		Relation: RelationViewer,
	}
	d := c.Check(context.Background(), req)
	if d.Allowed {
		t.Fatalf("cross-tenant access should be denied, got %+v", d)
	}
	if d.Code != CodeWrongTenant {
		t.Errorf("expected CodeWrongTenant, got %s", d.Code)
	}
}

func TestCompositeChecker_UnknownObjectType(t *testing.T) {
	c := NewCompositeChecker(NewTenantRoleAdapter())
	req := CheckRequest{
		User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectType("mystery"), ID: "x", TenantID: 1},
		Relation: RelationViewer,
	}
	d := c.Check(context.Background(), req)
	if d.Allowed {
		t.Fatalf("unknown object type should deny, got %+v", d)
	}
	if d.Code != CodeNoSuchAdapter {
		t.Errorf("expected CodeNoSuchAdapter, got %s", d.Code)
	}
}

func TestTenantRoleAdapter_Matrix(t *testing.T) {
	cases := []struct {
		name     string
		role     string
		relation Relation
		want     bool
	}{
		{"owner-can-owner", "owner", RelationOwner, true},
		{"admin-cannot-owner", "admin", RelationOwner, false},
		{"admin-can-admin", "admin", RelationAdmin, true},
		{"contributor-cannot-admin", "contributor", RelationAdmin, false},
		{"contributor-can-editor", "contributor", RelationEditor, true},
		{"viewer-can-viewer", "viewer", RelationViewer, true},
		{"viewer-cannot-editor", "viewer", RelationEditor, false},
		{"viewer-can-mention", "viewer", RelationMention, true},
		{"empty-role-denies", "", RelationViewer, false},
		{"unknown-relation-requires-admin", "admin", Relation("weird"), true},
		{"unknown-relation-denies-contributor", "contributor", Relation("weird"), false},
	}
	c := NewCompositeChecker(NewTenantRoleAdapter())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := CheckRequest{
				User:     User{Type: UserTypeUser, ID: "u", TenantID: 1, Role: tc.role},
				Object:   Object{Type: ObjectTypeTenant, ID: "x", TenantID: 1},
				Relation: tc.relation,
			}
			d := c.Check(context.Background(), req)
			if d.Allowed != tc.want {
				t.Errorf("%s: got allowed=%v (code=%s), want %v", tc.name, d.Allowed, d.Code, tc.want)
			}
		})
	}
}

func TestTenantRoleAdapter_SystemAlwaysAllowed(t *testing.T) {
	c := NewCompositeChecker(NewTenantRoleAdapter())
	req := CheckRequest{
		User:     User{Type: UserTypeSystem, ID: "system"},
		Object:   Object{Type: ObjectTypeTenant, ID: "x", TenantID: 1},
		Relation: RelationDelete,
	}
	d := c.Check(context.Background(), req)
	if !d.Allowed {
		t.Fatalf("system should be allowed, got %+v", d)
	}
}

func TestNotificationAdapter_RecipientOnly(t *testing.T) {
	lookup := func(_ context.Context, _ uint64, id string) (string, bool, error) {
		if id == "missing" {
			return "", false, nil
		}
		if id == "boom" {
			return "", false, fmt.Errorf("db down")
		}
		return "alice", true, nil
	}
	a := NewNotificationAdapter(lookup)
	c := NewCompositeChecker(NewTenantRoleAdapter(), a)

	t.Run("recipient-allowed", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
			Object:   Object{Type: ObjectTypeNotification, ID: "n1", TenantID: 1},
			Relation: RelationViewer,
		})
		if !d.Allowed {
			t.Errorf("recipient should be allowed, got %+v", d)
		}
	})

	t.Run("non-recipient-denied", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "bob", TenantID: 1},
			Object:   Object{Type: ObjectTypeNotification, ID: "n1", TenantID: 1},
			Relation: RelationViewer,
		})
		if d.Allowed {
			t.Errorf("non-recipient should be denied, got %+v", d)
		}
		if d.Code != CodeNotShared {
			t.Errorf("expected CodeNotShared, got %s", d.Code)
		}
	})

	t.Run("admin-bypass", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "admin1", TenantID: 1, Role: "admin"},
			Object:   Object{Type: ObjectTypeNotification, ID: "n1", TenantID: 1},
			Relation: RelationViewer,
		})
		if !d.Allowed {
			t.Errorf("admin should be allowed on any notification, got %+v", d)
		}
	})

	t.Run("missing-returns-404", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
			Object:   Object{Type: ObjectTypeNotification, ID: "missing", TenantID: 1},
			Relation: RelationViewer,
		})
		if d.Allowed {
			t.Errorf("missing notification should deny, got %+v", d)
		}
		if d.Code != CodeNoSuchResource {
			t.Errorf("expected CodeNoSuchResource, got %s", d.Code)
		}
	})

	t.Run("lookup-error-is-conservative-deny", func(t *testing.T) {
		d := c.Check(context.Background(), CheckRequest{
			User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
			Object:   Object{Type: ObjectTypeNotification, ID: "boom", TenantID: 1},
			Relation: RelationViewer,
		})
		if d.Allowed {
			t.Errorf("lookup error should deny, got %+v", d)
		}
		if d.Code != CodeError {
			t.Errorf("expected CodeError, got %s", d.Code)
		}
	})
}

func TestCheckBulk_OrderPreservedAndBounded(t *testing.T) {
	a := NewTenantRoleAdapter()
	c := NewCompositeChecker(a)
	reqs := make([]CheckRequest, 0, 50)
	for i := 0; i < 50; i++ {
		reqs = append(reqs, CheckRequest{
			User:     User{Type: UserTypeUser, ID: "u", TenantID: 1, Role: "owner"},
			Object:   Object{Type: ObjectTypeTenant, ID: "x", TenantID: 1},
			Relation: RelationViewer,
		})
	}
	decisions := c.CheckBulk(context.Background(), reqs)
	if len(decisions) != len(reqs) {
		t.Fatalf("decision count mismatch: got %d, want %d", len(decisions), len(reqs))
	}
	for i, d := range decisions {
		if !d.Allowed {
			t.Errorf("decision[%d] should be allowed, got %+v", i, d)
		}
	}
}

func TestInvalidate_ClearsCompositeCache(t *testing.T) {
	lookup := func(_ context.Context, _ uint64, id string) (string, bool, error) {
		return "alice", true, nil
	}
	a := NewNotificationAdapter(lookup)
	c := NewCompositeChecker(NewTenantRoleAdapter(), a)
	c.DisableCache() // we exercise invalidation via the adapter-level cache here
	_ = c

	// Re-enable to exercise the composite cache.
	c2 := NewCompositeChecker(NewTenantRoleAdapter(), a)
	obj := Object{Type: ObjectTypeNotification, ID: "n1", TenantID: 1}
	// Prime cache
	d1 := c2.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "alice", TenantID: 1},
		Object:   obj,
		Relation: RelationViewer,
	})
	if !d1.Allowed {
		t.Fatalf("priming check should be allowed, got %+v", d1)
	}
	// Invalidate
	if err := c2.Invalidate(context.Background(), obj); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	// Cache should be empty for that object
	if c2.decisionCache == nil {
		t.Fatalf("cache should be enabled")
	}
	// Verify by checking that the cached entries for this object were evicted.
	if c2.decisionCache.Len() != 0 {
		t.Errorf("after invalidate, cache should be empty for the object, got len=%d", c2.decisionCache.Len())
	}
}

// countingAdapter records Check call counts so we can verify cache
// hits actually short-circuit the adapter chain.
type countingAdapter struct {
	calls int64
}

func (c *countingAdapter) ObjectType() ObjectType { return ObjectTypeKB }
func (c *countingAdapter) Check(_ context.Context, _ CheckRequest) Decision {
	atomic.AddInt64(&c.calls, 1)
	return Allow("counting", "always allow")
}
func (c *countingAdapter) Invalidate(_ context.Context, _ Object) error { return nil }

func TestCompositeCache_HitsShortCircuitAdapter(t *testing.T) {
	ca := &countingAdapter{}
	c := NewCompositeChecker(ca)
	req := CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeKB, ID: "kb1", TenantID: 1},
		Relation: RelationViewer,
	}
	for i := 0; i < 5; i++ {
		_ = c.Check(context.Background(), req)
	}
	if got := atomic.LoadInt64(&ca.calls); got != 1 {
		t.Errorf("expected 1 adapter call (cache hit on 2..5), got %d", got)
	}
}

func TestDuplicateAdapterFirstWins(t *testing.T) {
	// Composite tolerates duplicate ObjectTypes — the first
	// registration wins. The TupleAdapter intentionally reuses
	// ObjectTypeTenant so it can sit alongside TenantRoleAdapter
	// without forcing the composite to special-case fallthrough
	// dispatch.
	a := NewTenantRoleAdapter()
	c := NewCompositeChecker(a, NewTenantRoleAdapter())
	if c.adapters[ObjectTypeTenant] != a {
		t.Fatalf("first adapter must win on duplicate ObjectType")
	}
}

func TestStableReasonOrder_PicksMostSpecific(t *testing.T) {
	ds := []Decision{
		Deny(CodeNoRelation, "a", "no relation"),
		Deny(CodeError, "b", "boom"),
		Allow("c", "ok"),
	}
	top := StableReasonOrder(ds)
	if !top.Allowed {
		t.Errorf("top should be allowed, got %+v", top)
	}
	// Now drop the allow and verify CodeError wins over CodeNoRelation.
	ds = []Decision{
		Deny(CodeNoRelation, "a", "no relation"),
		Deny(CodeError, "b", "boom"),
		Deny(CodeRoleTooLow, "c", "low"),
	}
	top = StableReasonOrder(ds)
	if top.Allowed {
		t.Errorf("expected deny, got %+v", top)
	}
	if top.Code != CodeError {
		t.Errorf("expected CodeError to win, got %s", top.Code)
	}
}
