// Package database implements the v0.7.25 Build #26 (G06) multi-view
// database. It is the parity layer with Notion / 飞书 Base / Tana's
// supertag-driven databases. The service owns CRUD for databases,
// fields, rows, and views; the view-specific renderers live in the
// frontend (vue components per DatabaseViewType).
package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors.
var (
	ErrDatabaseNotFound = errors.New("database: not found")
	ErrDatabaseName     = errors.New("database: name required")
	ErrUnknownFieldType = errors.New("database: unknown field type")
	ErrUnknownViewType  = errors.New("database: unknown view type")
	ErrRowNotFound      = errors.New("database: row not found")
	ErrViewNotFound     = errors.New("database: view not found")
)

// Service is the runtime façade.
type Service struct {
	repo interfaces.DatabaseRepository
}

// NewService wires the service.
func NewService(repo interfaces.DatabaseRepository) *Service {
	return &Service{repo: repo}
}

// Create creates a database + a default Table view + the standard "Name"
// primary text field in one call. The default view makes the database
// immediately usable; users can clone it into Board / Gallery / etc.
func (s *Service) Create(ctx context.Context, db *types.Database) error {
	if strings.TrimSpace(db.Name) == "" {
		return ErrDatabaseName
	}
	if db.ID == "" {
		db.ID = uuid.NewString()
	}
	db.CreatedAt = time.Now().UTC()
	db.UpdatedAt = db.CreatedAt
	if err := s.repo.CreateDatabase(ctx, db); err != nil {
		return fmt.Errorf("database: create: %w", err)
	}

	// Seed the primary Name field. Field IDs are stable so subsequent
	// row inserts can target the title cell.
	nameField := &types.DatabaseField{
		ID:         uuid.NewString(),
		DatabaseID: db.ID,
		Name:       "Name",
		Type:       types.DatabaseFieldText,
		Options:    types.JSON(`{}`),
		Width:      240,
		SortOrder:  0,
		IsPrimary:  true,
	}
	if err := s.repo.CreateField(ctx, nameField); err != nil {
		return fmt.Errorf("database: seed name field: %w", err)
	}

	// Default Table view with empty config.
	defaultView := &types.DatabaseView{
		ID:         uuid.NewString(),
		DatabaseID: db.ID,
		Type:       types.DatabaseViewTable,
		Name:       "All rows",
		Config:     types.JSON(`{}`),
		SortOrder:  0,
		IsDefault:  true,
		CreatedBy:  db.CreatedBy,
	}
	if err := s.repo.CreateView(ctx, defaultView); err != nil {
		return fmt.Errorf("database: seed default view: %w", err)
	}

	logger.Infof(ctx, "database: created id=%s kb=%s by=%s", db.ID, db.KnowledgeBaseID, db.CreatedBy)
	return nil
}

// Update mutates the database's metadata (name/description/icon).
func (s *Service) Update(ctx context.Context, db *types.Database) error {
	if _, err := s.repo.GetDatabase(ctx, db.TenantID, db.ID); err != nil {
		return err
	}
	db.UpdatedAt = time.Now().UTC()
	return s.repo.UpdateDatabase(ctx, db)
}

// Delete soft-deletes a database.
func (s *Service) Delete(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.SoftDeleteDatabase(ctx, tenantID, id)
}

// DatabaseDetail is the GET response shape: database + fields + views.
type DatabaseDetail struct {
	Database *types.Database        `json:"database"`
	Fields   []*types.DatabaseField `json:"fields"`
	Views    []*types.DatabaseView  `json:"views"`
}

func (s *Service) GetDetail(ctx context.Context, tenantID uint64, id string) (*DatabaseDetail, error) {
	db, err := s.repo.GetDatabase(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, ErrDatabaseNotFound
	}
	fields, err := s.repo.ListFields(ctx, id)
	if err != nil {
		return nil, err
	}
	views, err := s.repo.ListViews(ctx, id)
	if err != nil {
		return nil, err
	}
	return &DatabaseDetail{Database: db, Fields: fields, Views: views}, nil
}

func (s *Service) ListByKB(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.Database, int, error) {
	return s.repo.ListDatabasesByKB(ctx, tenantID, kbID, limit, offset)
}

// --- Fields ---

func (s *Service) AddField(ctx context.Context, f *types.DatabaseField) error {
	if !validFieldType(f.Type) {
		return ErrUnknownFieldType
	}
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.Options == nil {
		f.Options = types.JSON(`{}`)
	}
	if err := s.repo.CreateField(ctx, f); err != nil {
		return fmt.Errorf("database: add field: %w", err)
	}
	return nil
}

func (s *Service) UpdateField(ctx context.Context, f *types.DatabaseField) error {
	if !validFieldType(f.Type) {
		return ErrUnknownFieldType
	}
	return s.repo.UpdateField(ctx, f)
}

func (s *Service) DeleteField(ctx context.Context, databaseID, fieldID string) error {
	return s.repo.DeleteField(ctx, databaseID, fieldID)
}

// --- Rows ---

func (s *Service) AddRow(ctx context.Context, row *types.DatabaseRow) error {
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if row.Data == nil {
		row.Data = json.RawMessage(`{}`)
	}
	row.CreatedAt = time.Now().UTC()
	row.UpdatedAt = row.CreatedAt
	if err := s.repo.CreateRow(ctx, row); err != nil {
		return fmt.Errorf("database: add row: %w", err)
	}
	return nil
}

func (s *Service) UpdateRow(ctx context.Context, row *types.DatabaseRow) error {
	row.UpdatedAt = time.Now().UTC()
	return s.repo.UpdateRow(ctx, row)
}

func (s *Service) GetRow(ctx context.Context, tenantID uint64, id string) (*types.DatabaseRow, error) {
	row, err := s.repo.GetRow(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrRowNotFound
	}
	return row, nil
}

func (s *Service) ListRows(ctx context.Context, databaseID string, limit, offset int) ([]*types.DatabaseRow, int, error) {
	return s.repo.ListRows(ctx, databaseID, limit, offset)
}

// ReorderRows bulk-updates sort_order to match the new id sequence.
func (s *Service) ReorderRows(ctx context.Context, ids []string) error {
	return s.repo.BulkUpdateRowOrder(ctx, ids)
}

func (s *Service) DeleteRow(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.SoftDeleteRow(ctx, tenantID, id)
}

// --- Views ---

func (s *Service) AddView(ctx context.Context, v *types.DatabaseView) error {
	if !validViewType(v.Type) {
		return ErrUnknownViewType
	}
	if v.ID == "" {
		v.ID = uuid.NewString()
	}
	if v.Config == nil {
		v.Config = types.JSON(`{}`)
	}
	if err := s.repo.CreateView(ctx, v); err != nil {
		return fmt.Errorf("database: add view: %w", err)
	}
	return nil
}

func (s *Service) UpdateView(ctx context.Context, v *types.DatabaseView) error {
	if !validViewType(v.Type) {
		return ErrUnknownViewType
	}
	return s.repo.UpdateView(ctx, v)
}

func (s *Service) ListViews(ctx context.Context, databaseID string) ([]*types.DatabaseView, error) {
	return s.repo.ListViews(ctx, databaseID)
}

func (s *Service) DeleteView(ctx context.Context, databaseID, viewID string) error {
	return s.repo.DeleteView(ctx, databaseID, viewID)
}

// --- helpers ---

func validFieldType(t types.DatabaseFieldType) bool {
	for _, v := range types.AllDatabaseFieldTypes {
		if v == t {
			return true
		}
	}
	return false
}

func validViewType(t types.DatabaseViewType) bool {
	for _, v := range types.AllDatabaseViewTypes {
		if v == t {
			return true
		}
	}
	return false
}
