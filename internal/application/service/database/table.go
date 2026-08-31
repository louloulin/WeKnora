// Package database implements the v0.7.23 WeKnora Base — Database /
// 多维表 surface. A WKDatabase is a tenant-scoped schema + row store
// backed by JSONB-style values. The schema describes the columns
// (text / number / select / checkbox / date) and the row Values map
// stores per-column data.
//
// Validation is the service's primary responsibility: a write with a
// field name not in the schema, or a value of the wrong type, is
// rejected before the repo layer ever sees it.
package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Common errors.
var (
	ErrEmptyDatabaseName = errors.New("database: name required")
	ErrInvalidSchema     = errors.New("database: invalid schema")
	ErrDatabaseNotFound  = errors.New("database: not found")
	ErrRowNotFound       = errors.New("database: row not found")
	ErrInvalidRowValue   = errors.New("database: row value fails schema validation")
)

// Service is the public façade.
type Service struct {
	repo interfaces.WKDatabaseRepository
}

// NewService wires the service to the repo.
func NewService(repo interfaces.WKDatabaseRepository) *Service {
	return &Service{repo: repo}
}

// Create persists a new database. Validates that Name is set and the
// schema is non-empty + well-formed.
func (s *Service) Create(ctx context.Context, db *types.WKDatabase) error {
	db.Name = strings.TrimSpace(db.Name)
	if db.Name == "" {
		return ErrEmptyDatabaseName
	}
	if err := validateSchema(db.Schema); err != nil {
		return err
	}
	return s.repo.Create(ctx, db)
}

// Update mutates an existing database schema / name.
func (s *Service) Update(ctx context.Context, db *types.WKDatabase) error {
	if db.ID == 0 {
		return ErrDatabaseNotFound
	}
	db.Name = strings.TrimSpace(db.Name)
	if db.Name == "" {
		return ErrEmptyDatabaseName
	}
	if err := validateSchema(db.Schema); err != nil {
		return err
	}
	return s.repo.Update(ctx, db)
}

// Get returns one database.
func (s *Service) Get(ctx context.Context, tenantID string, id uint64) (*types.WKDatabase, error) {
	return s.repo.Get(ctx, tenantID, id)
}

// List returns the tenant's databases with pagination.
func (s *Service) List(ctx context.Context, tenantID string, limit, offset int) ([]*types.WKDatabase, int, error) {
	return s.repo.List(ctx, tenantID, limit, offset)
}

// Delete soft-deletes a database.
func (s *Service) Delete(ctx context.Context, tenantID string, id uint64) error {
	return s.repo.DeleteDatabase(ctx, tenantID, id)
}

// InsertRow validates row values against the schema and persists.
func (s *Service) InsertRow(ctx context.Context, row *types.WKDatabaseRow) error {
	db, err := s.repo.Get(ctx, row.TenantID, row.DatabaseID)
	if err != nil {
		return err
	}
	if db == nil {
		return ErrDatabaseNotFound
	}
	if err := validateRowValues(db.Schema, row.Values, false); err != nil {
		return err
	}
	return s.repo.InsertRow(ctx, row)
}

// UpdateRow mutates a row's values, re-validating against the schema.
func (s *Service) UpdateRow(ctx context.Context, row *types.WKDatabaseRow) error {
	existing, err := s.repo.GetRow(ctx, row.TenantID, row.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrRowNotFound
	}
	if existing.DatabaseID != row.DatabaseID {
		return fmt.Errorf("database: row %d belongs to database %d, not %d",
			row.ID, existing.DatabaseID, row.DatabaseID)
	}
	db, err := s.repo.Get(ctx, row.TenantID, existing.DatabaseID)
	if err != nil {
		return err
	}
	if db == nil {
		return ErrDatabaseNotFound
	}
	if err := validateRowValues(db.Schema, row.Values, true); err != nil {
		return err
	}
	return s.repo.UpdateRow(ctx, row)
}

// GetRow returns one row.
func (s *Service) GetRow(ctx context.Context, tenantID string, id uint64) (*types.WKDatabaseRow, error) {
	return s.repo.GetRow(ctx, tenantID, id)
}

// ListRows returns rows for one database with pagination.
func (s *Service) ListRows(ctx context.Context, tenantID string, databaseID uint64, limit, offset int) ([]*types.WKDatabaseRow, int, error) {
	return s.repo.ListRows(ctx, tenantID, databaseID, limit, offset)
}

// DeleteRow soft-deletes a row.
func (s *Service) DeleteRow(ctx context.Context, tenantID string, id uint64) error {
	return s.repo.DeleteRow(ctx, tenantID, id)
}

// --- validation ---

func validateSchema(schema []types.DatabaseField) error {
	if len(schema) == 0 {
		return fmt.Errorf("%w: empty", ErrInvalidSchema)
	}
	names := map[string]struct{}{}
	for _, f := range schema {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			return fmt.Errorf("%w: empty field name", ErrInvalidSchema)
		}
		if _, dup := names[name]; dup {
			return fmt.Errorf("%w: duplicate field %q", ErrInvalidSchema, name)
		}
		names[name] = struct{}{}
		switch f.Type {
		case types.DBFieldText, types.DBFieldNumber, types.DBFieldCheckbox, types.DBFieldDate:
			// ok
		case types.DBFieldSelect:
			if len(f.Options) == 0 {
				return fmt.Errorf("%w: select field %q has no options",
					ErrInvalidSchema, name)
			}
		default:
			return fmt.Errorf("%w: unknown type %q for field %q",
				ErrInvalidSchema, f.Type, name)
		}
	}
	return nil
}

// validateRowValues rejects writes where (a) the row carries a field
// not in the schema, (b) a value's Go type doesn't match the
// declared field type, or (c) a select value isn't in Options.
// updateMode allows missing fields (partial update).
func validateRowValues(schema []types.DatabaseField, values map[string]any, updateMode bool) error {
	if values == nil {
		return nil
	}
	allowed := map[string]types.DatabaseField{}
	for _, f := range schema {
		allowed[f.Name] = f
	}
	for k, v := range values {
		f, ok := allowed[k]
		if !ok {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidRowValue, k)
		}
		if v == nil {
			if !updateMode && f.Required {
				return fmt.Errorf("%w: required field %q is nil",
					ErrInvalidRowValue, k)
			}
			continue
		}
		switch f.Type {
		case types.DBFieldText:
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%w: field %q expects string", ErrInvalidRowValue, k)
			}
		case types.DBFieldNumber:
			switch v.(type) {
			case float64, float32, int, int32, int64, json.Number:
				// ok
			default:
				return fmt.Errorf("%w: field %q expects number", ErrInvalidRowValue, k)
			}
		case types.DBFieldCheckbox:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%w: field %q expects bool", ErrInvalidRowValue, k)
			}
		case types.DBFieldDate:
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%w: field %q expects ISO date string", ErrInvalidRowValue, k)
			}
		case types.DBFieldSelect:
			s, isStr := v.(string)
			if !isStr {
				return fmt.Errorf("%w: field %q expects string", ErrInvalidRowValue, k)
			}
			inOpts := false
			for _, opt := range f.Options {
				if opt == s {
					inOpts = true
					break
				}
			}
			if !inOpts {
				return fmt.Errorf("%w: select field %q value %q not in options",
					ErrInvalidRowValue, k, s)
			}
		}
	}
	return nil
}
