package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// DatabaseRepository is the persistence surface for the multi-view
// database (Build #26). It is dialect-portable via raw SQL so the same
// code runs against Postgres + SQLite.
type DatabaseRepository interface {
	CreateDatabase(ctx context.Context, d *types.Database) error
	UpdateDatabase(ctx context.Context, d *types.Database) error
	GetDatabase(ctx context.Context, tenantID uint64, id string) (*types.Database, error)
	ListDatabasesByKB(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.Database, int, error)
	SoftDeleteDatabase(ctx context.Context, tenantID uint64, id string) error

	CreateField(ctx context.Context, f *types.DatabaseField) error
	UpdateField(ctx context.Context, f *types.DatabaseField) error
	ListFields(ctx context.Context, databaseID string) ([]*types.DatabaseField, error)
	DeleteField(ctx context.Context, databaseID, fieldID string) error

	CreateRow(ctx context.Context, r *types.DatabaseRow) error
	UpdateRow(ctx context.Context, r *types.DatabaseRow) error
	GetRow(ctx context.Context, tenantID uint64, id string) (*types.DatabaseRow, error)
	ListRows(ctx context.Context, databaseID string, limit, offset int) ([]*types.DatabaseRow, int, error)
	BulkUpdateRowOrder(ctx context.Context, ids []string) error
	SoftDeleteRow(ctx context.Context, tenantID uint64, id string) error

	CreateView(ctx context.Context, v *types.DatabaseView) error
	UpdateView(ctx context.Context, v *types.DatabaseView) error
	ListViews(ctx context.Context, databaseID string) ([]*types.DatabaseView, error)
	DeleteView(ctx context.Context, databaseID, viewID string) error
}
