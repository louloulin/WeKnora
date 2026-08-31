package authz

import (
	"context"
	"errors"
	"testing"
)

// fakeDSCreator returns a fixed (tenant, creator, kbID, found)
// tuple. Mirrors the production DatasourceCreatorLookup shape.
type fakeDSCreator struct {
	TenantID  uint64
	CreatorID string
	KBID      string
	Found     bool
	Err       error
	CallCount int
}

func (f *fakeDSCreator) Lookup(_ context.Context, _ string) (uint64, string, string, bool, error) {
	f.CallCount++
	return f.TenantID, f.CreatorID, f.KBID, f.Found, f.Err
}

// fakeDSShare mimics DataSourceShareLookup.
type fakeDSShare struct {
	Role   string
	Shared bool
	Err    error
	Calls  int
}

func (f *fakeDSShare) Lookup(_ context.Context, _ string, _ uint64, _ string) (string, bool, error) {
	f.Calls++
	return f.Role, f.Shared, f.Err
}

// fakeKBAccess mimics KBShareLookup (re-used for the parent KB
// access lookup that drives the datasource inheritance rule).
type fakeKBAccess struct {
	Role   string
	Shared bool
	Err    error
	Calls  int
}

func (f *fakeKBAccess) Lookup(_ context.Context, _ string, _ uint64, _ string) (string, bool, error) {
	f.Calls++
	return f.Role, f.Shared, f.Err
}

func TestDataSourceAdapter_CreatorShortCircuit(t *testing.T) {
	creator := &fakeDSCreator{TenantID: 1, CreatorID: "u-1", KBID: "kb-1", Found: true}
	a := NewDataSourceAdapter(creator.Lookup, nil, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-1", TenantID: 1, Role: "viewer"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationAdmin,
	})
	if !d.Allowed || d.Source != "datasource" {
		t.Fatalf("creator short-circuit expected, got %+v", d)
	}
}

func TestDataSourceAdapter_NoSuchResource(t *testing.T) {
	creator := &fakeDSCreator{Found: false}
	a := NewDataSourceAdapter(creator.Lookup, nil, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-1", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-missing"},
		Relation: RelationViewer,
	})
	if d.Allowed || d.Code != CodeNoSuchResource {
		t.Fatalf("expected no_such_resource deny, got %+v", d)
	}
}

func TestDataSourceAdapter_CreatorLookupError(t *testing.T) {
	creator := &fakeDSCreator{Err: errors.New("db down")}
	a := NewDataSourceAdapter(creator.Lookup, nil, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-1", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationViewer,
	})
	if d.Allowed || d.Code != CodeError {
		t.Fatalf("expected CodeError deny, got %+v", d)
	}
}

func TestDataSourceAdapter_CrossTenantShareAllows(t *testing.T) {
	creator := &fakeDSCreator{TenantID: 1, CreatorID: "u-1", KBID: "kb-1", Found: true}
	share := &fakeDSShare{Role: "viewer", Shared: true}
	a := NewDataSourceAdapter(creator.Lookup, share.Lookup, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-2", TenantID: 2, Role: "viewer"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationViewer,
	})
	if !d.Allowed || d.Source != "datasource_share" {
		t.Fatalf("expected datasource_share allow, got %+v", d)
	}
}

func TestDataSourceAdapter_CrossTenantShareRoleTooLow(t *testing.T) {
	creator := &fakeDSCreator{TenantID: 1, CreatorID: "u-1", KBID: "kb-1", Found: true}
	share := &fakeDSShare{Role: "viewer", Shared: true}
	a := NewDataSourceAdapter(creator.Lookup, share.Lookup, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-2", TenantID: 2, Role: "viewer"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationAdmin,
	})
	if d.Allowed || d.Code != CodeRoleTooLow {
		t.Fatalf("expected CodeRoleTooLow deny, got %+v", d)
	}
}

func TestDataSourceAdapter_SameTenantRoleDenies(t *testing.T) {
	creator := &fakeDSCreator{TenantID: 1, CreatorID: "u-1", KBID: "kb-1", Found: true}
	a := NewDataSourceAdapter(creator.Lookup, nil, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-2", TenantID: 1, Role: "viewer"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationAdmin,
	})
	if d.Allowed || d.Code != CodeRoleTooLow {
		t.Fatalf("viewer should not satisfy admin, got %+v", d)
	}
}

func TestDataSourceAdapter_KBInheritanceGrantsViewer(t *testing.T) {
	creator := &fakeDSCreator{TenantID: 1, CreatorID: "u-1", KBID: "kb-1", Found: true}
	kb := &fakeKBAccess{Role: "viewer", Shared: true}
	a := NewDataSourceAdapter(creator.Lookup, nil, kb.Lookup)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-3", TenantID: 1, Role: "viewer"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationViewer,
	})
	if !d.Allowed || d.Source != "datasource" {
		t.Fatalf("KB inheritance should grant viewer, got %+v", d)
	}
}

func TestDataSourceAdapter_NilLookup(t *testing.T) {
	a := NewDataSourceAdapter(nil, nil, nil)
	d := a.Check(context.Background(), CheckRequest{
		User:     User{Type: UserTypeUser, ID: "u-1", TenantID: 1, Role: "owner"},
		Object:   Object{Type: ObjectTypeDatasource, ID: "ds-1"},
		Relation: RelationViewer,
	})
	if d.Allowed || d.Code != CodeError {
		t.Fatalf("nil lookup must fail-closed with CodeError, got %+v", d)
	}
}

func TestDataSourceAdapter_ObjectType(t *testing.T) {
	a := NewDataSourceAdapter(nil, nil, nil)
	if a.ObjectType() != ObjectTypeDatasource {
		t.Fatalf("ObjectType() = %q, want %q", a.ObjectType(), ObjectTypeDatasource)
	}
}

func TestDataSourceAdapter_InvalidateNoop(t *testing.T) {
	a := NewDataSourceAdapter(nil, nil, nil)
	if err := a.Invalidate(context.Background(), Object{}); err != nil {
		t.Fatalf("Invalidate must be a no-op, got %v", err)
	}
}
