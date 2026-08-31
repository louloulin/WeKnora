package database

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// stubDBRepo is an in-memory implementation of WKDatabaseRepository.
type stubDBRepo struct {
	dbs   map[uint64]*types.WKDatabase
	rows  map[uint64]*types.WKDatabaseRow
	seqDB uint64
	seqRW uint64
}

func newStubDBRepo() *stubDBRepo {
	return &stubDBRepo{
		dbs:  map[uint64]*types.WKDatabase{},
		rows: map[uint64]*types.WKDatabaseRow{},
	}
}

func (s *stubDBRepo) Create(_ context.Context, db *types.WKDatabase) error {
	s.seqDB++
	db.ID = s.seqDB
	s.dbs[db.ID] = db
	return nil
}
func (s *stubDBRepo) Update(_ context.Context, db *types.WKDatabase) error {
	s.dbs[db.ID] = db
	return nil
}
func (s *stubDBRepo) Get(_ context.Context, tenantID string, id uint64) (*types.WKDatabase, error) {
	if v, ok := s.dbs[id]; ok && v.TenantID == tenantID {
		return v, nil
	}
	return nil, nil
}
func (s *stubDBRepo) List(_ context.Context, tenantID string, limit, offset int) ([]*types.WKDatabase, int, error) {
	out := []*types.WKDatabase{}
	for _, v := range s.dbs {
		if v.TenantID == tenantID {
			out = append(out, v)
		}
	}
	return out, len(out), nil
}
func (s *stubDBRepo) DeleteDatabase(_ context.Context, tenantID string, id uint64) error {
	if v, ok := s.dbs[id]; ok && v.TenantID == tenantID {
		delete(s.dbs, id)
		return nil
	}
	return errors.New("not found")
}
func (s *stubDBRepo) InsertRow(_ context.Context, row *types.WKDatabaseRow) error {
	s.seqRW++
	row.ID = s.seqRW
	s.rows[row.ID] = row
	return nil
}
func (s *stubDBRepo) UpdateRow(_ context.Context, row *types.WKDatabaseRow) error {
	s.rows[row.ID] = row
	return nil
}
func (s *stubDBRepo) GetRow(_ context.Context, tenantID string, id uint64) (*types.WKDatabaseRow, error) {
	if v, ok := s.rows[id]; ok && v.TenantID == tenantID {
		return v, nil
	}
	return nil, nil
}
func (s *stubDBRepo) ListRows(_ context.Context, tenantID string, databaseID uint64, limit, offset int) ([]*types.WKDatabaseRow, int, error) {
	out := []*types.WKDatabaseRow{}
	for _, v := range s.rows {
		if v.TenantID == tenantID && v.DatabaseID == databaseID {
			out = append(out, v)
		}
	}
	return out, len(out), nil
}
func (s *stubDBRepo) DeleteRow(_ context.Context, tenantID string, id uint64) error {
	if v, ok := s.rows[id]; ok && v.TenantID == tenantID {
		delete(s.rows, id)
		return nil
	}
	return errors.New("not found")
}

// --- tests ---

func TestService_Create_HappyPath(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db := &types.WKDatabase{
		TenantID: "t1",
		Name:     "Tasks",
		Schema: []types.DatabaseField{
			{Name: "title", Type: types.DBFieldText},
			{Name: "priority", Type: types.DBFieldSelect, Options: []string{"low", "med", "high"}},
		},
		CreatedBy: "u1",
	}
	if err := svc.Create(context.Background(), db); err != nil {
		t.Fatalf("create: %v", err)
	}
	if db.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestService_Create_RejectsEmptyName(t *testing.T) {
	svc := NewService(newStubDBRepo())
	err := svc.Create(context.Background(), &types.WKDatabase{
		TenantID: "t1",
		Schema:   []types.DatabaseField{{Name: "x", Type: types.DBFieldText}},
	})
	if !errors.Is(err, ErrEmptyDatabaseName) {
		t.Fatalf("expected ErrEmptyDatabaseName, got %v", err)
	}
}

func TestService_Create_RejectsEmptySchema(t *testing.T) {
	svc := NewService(newStubDBRepo())
	err := svc.Create(context.Background(), &types.WKDatabase{
		TenantID: "t1", Name: "X",
	})
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestService_Create_RejectsSelectWithoutOptions(t *testing.T) {
	svc := NewService(newStubDBRepo())
	err := svc.Create(context.Background(), &types.WKDatabase{
		TenantID: "t1", Name: "X",
		Schema: []types.DatabaseField{
			{Name: "status", Type: types.DBFieldSelect /* no options */},
		},
	})
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestService_Create_RejectsDuplicateFieldNames(t *testing.T) {
	svc := NewService(newStubDBRepo())
	err := svc.Create(context.Background(), &types.WKDatabase{
		TenantID: "t1", Name: "X",
		Schema: []types.DatabaseField{
			{Name: "x", Type: types.DBFieldText},
			{Name: "x", Type: types.DBFieldNumber},
		},
	})
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestService_InsertRow_RejectsUnknownField(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db := &types.WKDatabase{
		TenantID: "t1", Name: "X", CreatedBy: "u1",
		Schema: []types.DatabaseField{{Name: "title", Type: types.DBFieldText}},
	}
	_ = svc.Create(context.Background(), db)
	err := svc.InsertRow(context.Background(), &types.WKDatabaseRow{
		TenantID: "t1", DatabaseID: db.ID, CreatedBy: "u1",
		Values: map[string]any{"unknown_field": "value"},
	})
	if !errors.Is(err, ErrInvalidRowValue) {
		t.Fatalf("expected ErrInvalidRowValue, got %v", err)
	}
}

func TestService_InsertRow_RejectsWrongType(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db := &types.WKDatabase{
		TenantID: "t1", Name: "X", CreatedBy: "u1",
		Schema: []types.DatabaseField{
			{Name: "score", Type: types.DBFieldNumber},
		},
	}
	_ = svc.Create(context.Background(), db)
	err := svc.InsertRow(context.Background(), &types.WKDatabaseRow{
		TenantID: "t1", DatabaseID: db.ID, CreatedBy: "u1",
		Values: map[string]any{"score": "not a number"},
	})
	if !errors.Is(err, ErrInvalidRowValue) {
		t.Fatalf("expected ErrInvalidRowValue, got %v", err)
	}
}

func TestService_InsertRow_RejectsSelectValueNotInOptions(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db := &types.WKDatabase{
		TenantID: "t1", Name: "X", CreatedBy: "u1",
		Schema: []types.DatabaseField{
			{Name: "priority", Type: types.DBFieldSelect, Options: []string{"low", "med"}},
		},
	}
	_ = svc.Create(context.Background(), db)
	err := svc.InsertRow(context.Background(), &types.WKDatabaseRow{
		TenantID: "t1", DatabaseID: db.ID, CreatedBy: "u1",
		Values: map[string]any{"priority": "urgent"}, // not in options
	})
	if !errors.Is(err, ErrInvalidRowValue) {
		t.Fatalf("expected ErrInvalidRowValue, got %v", err)
	}
}

func TestService_InsertRow_HappyPath_AllTypes(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db := &types.WKDatabase{
		TenantID: "t1", Name: "AllTypes", CreatedBy: "u1",
		Schema: []types.DatabaseField{
			{Name: "title", Type: types.DBFieldText},
			{Name: "score", Type: types.DBFieldNumber},
			{Name: "done", Type: types.DBFieldCheckbox},
			{Name: "due", Type: types.DBFieldDate},
			{Name: "status", Type: types.DBFieldSelect, Options: []string{"open", "closed"}},
		},
	}
	_ = svc.Create(context.Background(), db)
	row := &types.WKDatabaseRow{
		TenantID: "t1", DatabaseID: db.ID, CreatedBy: "u1",
		Values: map[string]any{
			"title":  "test",
			"score":  42.0,
			"done":   true,
			"due":    "2026-09-01",
			"status": "open",
		},
	}
	if err := svc.InsertRow(context.Background(), row); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if row.ID == 0 {
		t.Error("expected non-zero row ID")
	}
}

func TestService_UpdateRow_RejectsCrossDatabase(t *testing.T) {
	repo := newStubDBRepo()
	svc := NewService(repo)
	db1 := &types.WKDatabase{TenantID: "t1", Name: "A", CreatedBy: "u1",
		Schema: []types.DatabaseField{{Name: "x", Type: types.DBFieldText}}}
	_ = svc.Create(context.Background(), db1)
	db2 := &types.WKDatabase{TenantID: "t1", Name: "B", CreatedBy: "u1",
		Schema: []types.DatabaseField{{Name: "x", Type: types.DBFieldText}}}
	_ = svc.Create(context.Background(), db2)

	row := &types.WKDatabaseRow{TenantID: "t1", DatabaseID: db1.ID, CreatedBy: "u1",
		Values: map[string]any{"x": "v"}}
	_ = svc.InsertRow(context.Background(), row)

	err := svc.UpdateRow(context.Background(), &types.WKDatabaseRow{
		ID: row.ID, TenantID: "t1", DatabaseID: db2.ID,
		Values: map[string]any{"x": "v"},
	})
	if err == nil {
		t.Fatal("expected error when updating row with mismatched database_id")
	}
}

// interface compliance
var _ interfaces.WKDatabaseRepository = (*stubDBRepo)(nil)
