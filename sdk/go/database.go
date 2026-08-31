package weknora

import (
	"context"

)

// DatabaseService exposes CRUD over multi-dim tables, columns, views, and rows.
type DatabaseService struct{ c *Client }

// NewDatabaseService constructs a DatabaseService.
func NewDatabaseService(c *Client) *DatabaseService { return &DatabaseService{c: c} }

// Create inserts a new database.
func (s *DatabaseService) Create(ctx context.Context, kbID string, in  DatabaseInput) (* Database, error) {
	var out  Database
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/databases", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the databases in a KB.
func (s *DatabaseService) List(ctx context.Context, kbID string) ([] Database, error) {
	var out [] Database
	if err := s.c.Do(ctx, "GET", "/knowledge-bases/"+kbID+"/databases", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InsertRows appends new rows to a database.
func (s *DatabaseService) InsertRows(ctx context.Context, kbID, databaseID string, rows [] RowInput) ([] Row, error) {
	var out [] Row
	body := map[string]any{"rows": rows}
	if err := s.c.Do(ctx, "POST", "/knowledge-bases/"+kbID+"/databases/"+databaseID+"/rows", nil, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// QueryRows fetches rows that match a filter expression.
func (s *DatabaseService) QueryRows(ctx context.Context, kbID, databaseID, filter string) ([] Row, error) {
	var out [] Row
	q := map[string][]string{"filter": {filter}}
	if err := s.c.DoQuery(ctx, "GET", "/knowledge-bases/"+kbID+"/databases/"+databaseID+"/rows", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}
