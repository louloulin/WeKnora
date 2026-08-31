package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// fakeRepo implements interfaces.DatabaseRepository in-memory for
// unit tests. It is deliberately minimal — it tracks the same shapes
// the GORM-backed repo tracks, and lets us assert service-level
// invariants (seed field, default view, error mapping) without DB.
type fakeRepo struct {
	databases map[string]*types.Database
	fields    map[string]*types.DatabaseField // keyed by id
	rows      map[string]*types.DatabaseRow
	views     map[string]*types.DatabaseView
	dbSeq     int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		databases: map[string]*types.Database{},
		fields:    map[string]*types.DatabaseField{},
		rows:      map[string]*types.DatabaseRow{},
		views:     map[string]*types.DatabaseView{},
	}
}

func (r *fakeRepo) CreateDatabase(_ context.Context, d *types.Database) error {
	if _, ok := r.databases[d.ID]; ok {
		return nil
	}
	r.databases[d.ID] = d
	return nil
}
func (r *fakeRepo) UpdateDatabase(_ context.Context, d *types.Database) error {
	r.databases[d.ID] = d
	return nil
}
func (r *fakeRepo) GetDatabase(_ context.Context, _ uint64, id string) (*types.Database, error) {
	d, ok := r.databases[id]
	if !ok {
		return nil, nil
	}
	return d, nil
}
func (r *fakeRepo) ListDatabasesByKB(_ context.Context, _ uint64, kbID string, _, _ int) ([]*types.Database, int, error) {
	out := []*types.Database{}
	for _, d := range r.databases {
		if d.KnowledgeBaseID == kbID && d.DeletedAt == nil {
			out = append(out, d)
		}
	}
	return out, len(out), nil
}
func (r *fakeRepo) SoftDeleteDatabase(_ context.Context, _ uint64, id string) error {
	if d, ok := r.databases[id]; ok {
		d.DeletedAt = nil
		delete(r.databases, id)
	}
	return nil
}

func (r *fakeRepo) CreateField(_ context.Context, f *types.DatabaseField) error {
	r.fields[f.ID] = f
	return nil
}
func (r *fakeRepo) UpdateField(_ context.Context, f *types.DatabaseField) error {
	r.fields[f.ID] = f
	return nil
}
func (r *fakeRepo) ListFields(_ context.Context, dbID string) ([]*types.DatabaseField, error) {
	out := []*types.DatabaseField{}
	for _, f := range r.fields {
		if f.DatabaseID == dbID {
			out = append(out, f)
		}
	}
	return out, nil
}
func (r *fakeRepo) DeleteField(_ context.Context, _, fieldID string) error {
	delete(r.fields, fieldID)
	return nil
}

func (r *fakeRepo) CreateRow(_ context.Context, row *types.DatabaseRow) error {
	r.rows[row.ID] = row
	return nil
}
func (r *fakeRepo) UpdateRow(_ context.Context, row *types.DatabaseRow) error {
	r.rows[row.ID] = row
	return nil
}
func (r *fakeRepo) GetRow(_ context.Context, _ uint64, id string) (*types.DatabaseRow, error) {
	row, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	return row, nil
}
func (r *fakeRepo) ListRows(_ context.Context, dbID string, _, _ int) ([]*types.DatabaseRow, int, error) {
	out := []*types.DatabaseRow{}
	for _, row := range r.rows {
		if row.DatabaseID == dbID && row.DeletedAt == nil {
			out = append(out, row)
		}
	}
	return out, len(out), nil
}
func (r *fakeRepo) BulkUpdateRowOrder(_ context.Context, ids []string) error {
	for i, id := range ids {
		if row, ok := r.rows[id]; ok {
			row.SortOrder = i
		}
	}
	return nil
}
func (r *fakeRepo) SoftDeleteRow(_ context.Context, _ uint64, id string) error {
	if row, ok := r.rows[id]; ok {
		row.DeletedAt = nil
		delete(r.rows, id)
	}
	return nil
}

func (r *fakeRepo) CreateView(_ context.Context, v *types.DatabaseView) error {
	r.views[v.ID] = v
	return nil
}
func (r *fakeRepo) UpdateView(_ context.Context, v *types.DatabaseView) error {
	r.views[v.ID] = v
	return nil
}
func (r *fakeRepo) ListViews(_ context.Context, dbID string) ([]*types.DatabaseView, error) {
	out := []*types.DatabaseView{}
	for _, v := range r.views {
		if v.DatabaseID == dbID {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *fakeRepo) DeleteView(_ context.Context, _, viewID string) error {
	delete(r.views, viewID)
	return nil
}

// --- tests ---

func TestCreateSeedsNameFieldAndDefaultView(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	db := &types.Database{
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Name:            "Tasks",
		CreatedBy:       "u-1",
	}
	if err := svc.Create(ctx, db); err != nil {
		t.Fatalf("create: %v", err)
	}
	detail, err := svc.GetDetail(ctx, 1, db.ID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if len(detail.Fields) != 1 || detail.Fields[0].Name != "Name" || !detail.Fields[0].IsPrimary {
		t.Errorf("expected one primary Name field, got %+v", detail.Fields)
	}
	if len(detail.Views) != 1 || detail.Views[0].Type != types.DatabaseViewTable || !detail.Views[0].IsDefault {
		t.Errorf("expected default Table view, got %+v", detail.Views)
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	if err := svc.Create(context.Background(), &types.Database{KnowledgeBaseID: "kb-1"}); err == nil {
		t.Fatal("expected error on empty name")
	}
}

func TestAddFieldRejectsUnknownType(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	db := &types.Database{TenantID: 1, KnowledgeBaseID: "kb-1", Name: "x", CreatedBy: "u-1"}
	if err := svc.Create(ctx, db); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.AddField(ctx, &types.DatabaseField{DatabaseID: db.ID, Name: "x", Type: "bad-type"}); err == nil {
		t.Fatal("expected error on unknown type")
	}
}

func TestAddRowAndReorder(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	db := &types.Database{TenantID: 1, KnowledgeBaseID: "kb-1", Name: "x", CreatedBy: "u-1"}
	if err := svc.Create(ctx, db); err != nil {
		t.Fatalf("create: %v", err)
	}
	r1 := &types.DatabaseRow{DatabaseID: db.ID, Data: json.RawMessage(`{"x":1}`)}
	r2 := &types.DatabaseRow{DatabaseID: db.ID, Data: json.RawMessage(`{"x":2}`)}
	if err := svc.AddRow(ctx, r1); err != nil {
		t.Fatalf("add r1: %v", err)
	}
	if err := svc.AddRow(ctx, r2); err != nil {
		t.Fatalf("add r2: %v", err)
	}
	if err := svc.ReorderRows(ctx, []string{r2.ID, r1.ID}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if r1.SortOrder != 1 || r2.SortOrder != 0 {
		t.Errorf("reorder failed: r1=%d r2=%d", r1.SortOrder, r2.SortOrder)
	}
}

func TestAddViewRejectsUnknownType(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	db := &types.Database{TenantID: 1, KnowledgeBaseID: "kb-1", Name: "x", CreatedBy: "u-1"}
	if err := svc.Create(ctx, db); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.AddView(ctx, &types.DatabaseView{DatabaseID: db.ID, Type: "bad-view", CreatedBy: "u-1"}); err == nil {
		t.Fatal("expected error on unknown view type")
	}
}

func TestGetDetailReturns404ForMissing(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	if _, err := svc.GetDetail(context.Background(), 1, "missing"); err != ErrDatabaseNotFound {
		t.Errorf("err = %v, want %v", err, ErrDatabaseNotFound)
	}
}
