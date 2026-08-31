package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// databaseRepository implements DatabaseRepository with cross-dialect
// raw SQL (Postgres + SQLite). The shape mirrors the schema in
// migrations/{versioned/000125,sqlite/000038}_databases.up.sql.
type databaseRepository struct {
	db *gorm.DB
}

// NewDatabaseRepository wires the repo.
func NewDatabaseRepository(db *gorm.DB) *databaseRepository {
	return &databaseRepository{db: db}
}

func (r *databaseRepository) dialect() string {
	if r.db.Dialector != nil && strings.Contains(r.db.Dialector.Name(), "postgres") {
		return "postgres"
	}
	return "sqlite"
}

// --- Database ---

func (r *databaseRepository) CreateDatabase(ctx context.Context, d *types.Database) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO databases (id, tenant_id, knowledge_base_id, name, description, icon, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
RETURNING created_at, updated_at`,
			d.ID, d.TenantID, d.KnowledgeBaseID, d.Name, d.Description, d.Icon, d.CreatedBy,
		).Scan(d).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO databases (id, tenant_id, knowledge_base_id, name, description, icon, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING created_at, updated_at`,
		d.ID, d.TenantID, d.KnowledgeBaseID, d.Name, d.Description, d.Icon, d.CreatedBy,
	).Scan(d).Error
}

func (r *databaseRepository) UpdateDatabase(ctx context.Context, d *types.Database) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE databases SET name = ?, description = ?, icon = ?, updated_at = NOW()
WHERE id = ? AND tenant_id = ?`,
			d.Name, d.Description, d.Icon, d.ID, d.TenantID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE databases SET name = ?, description = ?, icon = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND tenant_id = ?`,
		d.Name, d.Description, d.Icon, d.ID, d.TenantID,
	).Error
}

func (r *databaseRepository) GetDatabase(ctx context.Context, tenantID uint64, id string) (*types.Database, error) {
	var d types.Database
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, knowledge_base_id, name, description, icon, created_by, created_at, updated_at, deleted_at
FROM databases WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (r *databaseRepository) ListDatabasesByKB(ctx context.Context, tenantID uint64, kbID string, limit, offset int) ([]*types.Database, int, error) {
	var rows []*types.Database
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, knowledge_base_id, name, description, icon, created_by, created_at, updated_at, deleted_at
FROM databases
WHERE tenant_id = ? AND knowledge_base_id = ? AND deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT ? OFFSET ?`,
		tenantID, kbID, limit, offset,
	).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM databases
WHERE tenant_id = ? AND knowledge_base_id = ? AND deleted_at IS NULL`,
		tenantID, kbID,
	).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *databaseRepository) SoftDeleteDatabase(ctx context.Context, tenantID uint64, id string) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE databases SET deleted_at = NOW() WHERE id = ? AND tenant_id = ?`,
			id, tenantID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE databases SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND tenant_id = ?`,
		id, tenantID,
	).Error
}

// --- Field ---

func (r *databaseRepository) CreateField(ctx context.Context, f *types.DatabaseField) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO database_fields (id, database_id, name, type, options, width, sort_order, is_primary, created_at)
VALUES (?, ?, ?, ?, ?::jsonb, ?, ?, ?, NOW())
RETURNING created_at`,
			f.ID, f.DatabaseID, f.Name, string(f.Type), []byte(f.Options), f.Width, f.SortOrder, f.IsPrimary,
		).Scan(f).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO database_fields (id, database_id, name, type, options, width, sort_order, is_primary, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING created_at`,
		f.ID, f.DatabaseID, f.Name, string(f.Type), string(f.Options), f.Width, f.SortOrder, f.IsPrimary,
	).Scan(f).Error
}

func (r *databaseRepository) UpdateField(ctx context.Context, f *types.DatabaseField) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE database_fields SET name = ?, type = ?, options = ?::jsonb, width = ?, sort_order = ?, is_primary = ?
WHERE id = ? AND database_id = ?`,
			f.Name, string(f.Type), []byte(f.Options), f.Width, f.SortOrder, f.IsPrimary, f.ID, f.DatabaseID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE database_fields SET name = ?, type = ?, options = ?, width = ?, sort_order = ?, is_primary = ?
WHERE id = ? AND database_id = ?`,
		f.Name, string(f.Type), string(f.Options), f.Width, f.SortOrder, f.IsPrimary, f.ID, f.DatabaseID,
	).Error
}

func (r *databaseRepository) ListFields(ctx context.Context, databaseID string) ([]*types.DatabaseField, error) {
	var rows []*types.DatabaseField
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, database_id, name, type, options, width, sort_order, is_primary, created_at
FROM database_fields WHERE database_id = ? ORDER BY sort_order ASC, created_at ASC`,
		databaseID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *databaseRepository) DeleteField(ctx context.Context, databaseID, fieldID string) error {
	return r.db.WithContext(ctx).Raw(`
DELETE FROM database_fields WHERE id = ? AND database_id = ?`,
		fieldID, databaseID,
	).Error
}

// --- Row ---

func (r *databaseRepository) CreateRow(ctx context.Context, row *types.DatabaseRow) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO database_rows (id, database_id, data, sort_order, created_by, created_at, updated_at)
VALUES (?, ?, ?::jsonb, ?, ?, NOW(), NOW())
RETURNING created_at, updated_at`,
			row.ID, row.DatabaseID, []byte(row.Data), row.SortOrder, row.CreatedBy,
		).Scan(row).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO database_rows (id, database_id, data, sort_order, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING created_at, updated_at`,
		row.ID, row.DatabaseID, string(row.Data), row.SortOrder, row.CreatedBy,
	).Scan(row).Error
}

func (r *databaseRepository) UpdateRow(ctx context.Context, row *types.DatabaseRow) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE database_rows SET data = ?::jsonb, sort_order = ?, updated_at = NOW()
WHERE id = ? AND database_id = ?`,
			[]byte(row.Data), row.SortOrder, row.ID, row.DatabaseID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE database_rows SET data = ?, sort_order = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND database_id = ?`,
		string(row.Data), row.SortOrder, row.ID, row.DatabaseID,
	).Error
}

func (r *databaseRepository) GetRow(ctx context.Context, tenantID uint64, id string) (*types.DatabaseRow, error) {
	var row types.DatabaseRow
	if err := r.db.WithContext(ctx).Raw(`
SELECT r.id, r.database_id, r.data, r.sort_order, r.created_by, r.created_at, r.updated_at, r.deleted_at
FROM database_rows r
JOIN databases d ON d.id = r.database_id
WHERE r.id = ? AND d.tenant_id = ? AND r.deleted_at IS NULL`,
		id, tenantID,
	).Scan(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *databaseRepository) ListRows(ctx context.Context, databaseID string, limit, offset int) ([]*types.DatabaseRow, int, error) {
	var rows []*types.DatabaseRow
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, database_id, data, sort_order, created_by, created_at, updated_at, deleted_at
FROM database_rows
WHERE database_id = ? AND deleted_at IS NULL
ORDER BY sort_order ASC, created_at ASC
LIMIT ? OFFSET ?`,
		databaseID, limit, offset,
	).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM database_rows WHERE database_id = ? AND deleted_at IS NULL`,
		databaseID,
	).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *databaseRepository) BulkUpdateRowOrder(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	// One UPDATE per id keeps the SQL portable. The number of rows in a
	// single drag-reorder is bounded by UX (typically ≤ 100), so this is
	// not a hot path concern.
	if r.dialect() == "postgres" {
		for i, id := range ids {
			if err := r.db.WithContext(ctx).Raw(`
UPDATE database_rows SET sort_order = ?, updated_at = NOW() WHERE id = ?`,
				i, id,
			).Error; err != nil {
				return err
			}
		}
		return nil
	}
	for i, id := range ids {
		if err := r.db.WithContext(ctx).Raw(`
UPDATE database_rows SET sort_order = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			i, id,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *databaseRepository) SoftDeleteRow(ctx context.Context, tenantID uint64, id string) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE database_rows r SET deleted_at = NOW()
FROM databases d
WHERE r.id = ? AND d.id = r.database_id AND d.tenant_id = ?`,
			id, tenantID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE database_rows SET deleted_at = CURRENT_TIMESTAMP
WHERE id = ? AND database_id IN (SELECT id FROM databases WHERE tenant_id = ?)`,
		id, tenantID,
	).Error
}

// --- View ---

func (r *databaseRepository) CreateView(ctx context.Context, v *types.DatabaseView) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO database_views (id, database_id, type, name, config, sort_order, is_default, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?::jsonb, ?, ?, ?, NOW(), NOW())
RETURNING created_at, updated_at`,
			v.ID, v.DatabaseID, string(v.Type), v.Name, []byte(v.Config), v.SortOrder, v.IsDefault, v.CreatedBy,
		).Scan(v).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO database_views (id, database_id, type, name, config, sort_order, is_default, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING created_at, updated_at`,
		v.ID, v.DatabaseID, string(v.Type), v.Name, string(v.Config), v.SortOrder, v.IsDefault, v.CreatedBy,
	).Scan(v).Error
}

func (r *databaseRepository) UpdateView(ctx context.Context, v *types.DatabaseView) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
UPDATE database_views SET type = ?, name = ?, config = ?::jsonb, sort_order = ?, is_default = ?, updated_at = NOW()
WHERE id = ? AND database_id = ?`,
			string(v.Type), v.Name, []byte(v.Config), v.SortOrder, v.IsDefault, v.ID, v.DatabaseID,
		).Error
	}
	return r.db.WithContext(ctx).Raw(`
UPDATE database_views SET type = ?, name = ?, config = ?, sort_order = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND database_id = ?`,
		string(v.Type), v.Name, string(v.Config), v.SortOrder, v.IsDefault, v.ID, v.DatabaseID,
	).Error
}

func (r *databaseRepository) ListViews(ctx context.Context, databaseID string) ([]*types.DatabaseView, error) {
	var rows []*types.DatabaseView
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, database_id, type, name, config, sort_order, is_default, created_by, created_at, updated_at
FROM database_views WHERE database_id = ? ORDER BY sort_order ASC, created_at ASC`,
		databaseID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *databaseRepository) DeleteView(ctx context.Context, databaseID, viewID string) error {
	return r.db.WithContext(ctx).Raw(`
DELETE FROM database_views WHERE id = ? AND database_id = ?`,
		viewID, databaseID,
	).Error
}
