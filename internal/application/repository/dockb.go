package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/types"
)

// dockbRepository implements interfaces.DocKBSummaryRepository and
// WKDatabaseRepository using cross-dialect raw SQL. The v0.7.23
// tables were added after the initial auto-migrate phase, so we keep
// the same ON CONFLICT / INSERT OR REPLACE pattern as the dlp_authz
// repo.
type dockbRepository struct {
	db *gorm.DB
}

// NewDockbRepository wires the repo to the shared *gorm.DB.
func NewDockbRepository(db *gorm.DB) *dockbRepository {
	return &dockbRepository{db: db}
}

func (r *dockbRepository) dialect() string {
	if r.db.Dialector != nil {
		name := r.db.Dialector.Name()
		if strings.Contains(name, "postgres") {
			return "postgres"
		}
	}
	return "sqlite"
}

// --- DocKBSummary ---

func (r *dockbRepository) Upsert(ctx context.Context, s *types.DocKBSummary) error {
	kpJSON, _ := json.Marshal(s.Keyphrases)
	tagsJSON, _ := json.Marshal(s.AutoTags)
	if r.dialect() == "postgres" {
		const q = `
INSERT INTO doc_kb_summaries
  (tenant_id, knowledge_id, chunk_id, summary, keyphrases, auto_tags,
   model_name, confidence, created_at, updated_at)
VALUES (?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, NOW(), NOW())
ON CONFLICT (tenant_id, knowledge_id, chunk_id) WHERE deleted_at IS NULL
DO UPDATE SET
  summary = EXCLUDED.summary,
  keyphrases = EXCLUDED.keyphrases,
  auto_tags = EXCLUDED.auto_tags,
  model_name = EXCLUDED.model_name,
  confidence = EXCLUDED.confidence,
  updated_at = NOW()
RETURNING id`
		return r.db.WithContext(ctx).Raw(q,
			s.TenantID, s.KnowledgeID, s.ChunkID, s.Summary,
			string(kpJSON), string(tagsJSON), s.ModelName, s.Confidence,
		).Scan(s).Error
	}
	// SQLite path: try insert, fall back to update on conflict.
	res := r.db.WithContext(ctx).Exec(`
INSERT INTO doc_kb_summaries
  (tenant_id, knowledge_id, chunk_id, summary, keyphrases, auto_tags,
   model_name, confidence, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		s.TenantID, s.KnowledgeID, s.ChunkID, s.Summary,
		string(kpJSON), string(tagsJSON), s.ModelName, s.Confidence,
	)
	if res.Error == nil {
		// fetch generated id back
		return r.db.WithContext(ctx).Raw(`
SELECT id, created_at, updated_at FROM doc_kb_summaries
WHERE tenant_id = ? AND knowledge_id = ? AND chunk_id = ?
ORDER BY id DESC LIMIT 1`,
			s.TenantID, s.KnowledgeID, s.ChunkID,
		).Scan(s).Error
	}
	if !strings.Contains(res.Error.Error(), "UNIQUE") {
		return res.Error
	}
	return r.db.WithContext(ctx).Exec(`
UPDATE doc_kb_summaries SET
  summary = ?, keyphrases = ?, auto_tags = ?,
  model_name = ?, confidence = ?, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND knowledge_id = ? AND chunk_id = ? AND deleted_at IS NULL`,
		s.Summary, string(kpJSON), string(tagsJSON),
		s.ModelName, s.Confidence,
		s.TenantID, s.KnowledgeID, s.ChunkID,
	).Error
}

func (r *dockbRepository) GetByChunk(ctx context.Context, tenantID, knowledgeID, chunkID string) (*types.DocKBSummary, error) {
	var s types.DocKBSummary
	err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, knowledge_id, chunk_id, summary, keyphrases,
       auto_tags, model_name, confidence, created_at, updated_at
FROM doc_kb_summaries
WHERE tenant_id = ? AND knowledge_id = ? AND chunk_id = ? AND deleted_at IS NULL
LIMIT 1`, tenantID, knowledgeID, chunkID).Scan(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *dockbRepository) ListByKnowledge(ctx context.Context, tenantID, knowledgeID string) ([]*types.DocKBSummary, error) {
	var rows []*types.DocKBSummary
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, knowledge_id, chunk_id, summary, keyphrases,
       auto_tags, model_name, confidence, created_at, updated_at
FROM doc_kb_summaries
WHERE tenant_id = ? AND knowledge_id = ? AND deleted_at IS NULL
ORDER BY created_at ASC`, tenantID, knowledgeID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *dockbRepository) DeleteSummary(ctx context.Context, tenantID string, id uint64) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE doc_kb_summaries SET deleted_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ?`, tenantID, id).Error
}

// --- WKDatabase ---

func (r *dockbRepository) Create(ctx context.Context, db *types.WKDatabase) error {
	schemaJSON, err := json.Marshal(db.Schema)
	if err != nil {
		return fmt.Errorf("dockb: marshal schema: %w", err)
	}
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO wk_databases (tenant_id, name, description, schema, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?::jsonb, ?, NOW(), NOW())
RETURNING id, created_at, updated_at`,
			db.TenantID, db.Name, db.Description, string(schemaJSON), db.CreatedBy,
		).Scan(db).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO wk_databases (tenant_id, name, description, schema, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id, created_at, updated_at`,
		db.TenantID, db.Name, db.Description, string(schemaJSON), db.CreatedBy,
	).Scan(db).Error
}

func (r *dockbRepository) Update(ctx context.Context, db *types.WKDatabase) error {
	schemaJSON, err := json.Marshal(db.Schema)
	if err != nil {
		return fmt.Errorf("dockb: marshal schema: %w", err)
	}
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
UPDATE wk_databases SET name = ?, description = ?, schema = ?::jsonb, updated_at = NOW()
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
			db.Name, db.Description, string(schemaJSON), db.TenantID, db.ID,
		).Error
	}
	return r.db.WithContext(ctx).Exec(`
UPDATE wk_databases SET name = ?, description = ?, schema = ?, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
		db.Name, db.Description, string(schemaJSON), db.TenantID, db.ID,
	).Error
}

func (r *dockbRepository) Get(ctx context.Context, tenantID string, id uint64) (*types.WKDatabase, error) {
	var db types.WKDatabase
	err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, name, description, schema, created_by, created_at, updated_at
FROM wk_databases
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL LIMIT 1`,
		tenantID, id).Scan(&db).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if db.ID == 0 {
		return nil, nil
	}
	return &db, nil
}

func (r *dockbRepository) List(ctx context.Context, tenantID string, limit, offset int) ([]*types.WKDatabase, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.WKDatabase
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, name, description, schema, created_by, created_at, updated_at
FROM wk_databases
WHERE tenant_id = ? AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, tenantID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM wk_databases WHERE tenant_id = ? AND deleted_at IS NULL`,
		tenantID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *dockbRepository) DeleteDatabase(ctx context.Context, tenantID string, id uint64) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE wk_databases SET deleted_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ?`, tenantID, id).Error
}

// --- WKDatabaseRow ---

func (r *dockbRepository) InsertRow(ctx context.Context, row *types.WKDatabaseRow) error {
	valuesJSON, err := json.Marshal(row.Values)
	if err != nil {
		return fmt.Errorf("dockb: marshal values: %w", err)
	}
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO wk_database_rows (tenant_id, database_id, values, created_by, created_at, updated_at)
VALUES (?, ?, ?::jsonb, ?, NOW(), NOW())
RETURNING id, created_at, updated_at`,
			row.TenantID, row.DatabaseID, string(valuesJSON), row.CreatedBy,
		).Scan(row).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO wk_database_rows (tenant_id, database_id, values, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id, created_at, updated_at`,
		row.TenantID, row.DatabaseID, string(valuesJSON), row.CreatedBy,
	).Scan(row).Error
}

func (r *dockbRepository) UpdateRow(ctx context.Context, row *types.WKDatabaseRow) error {
	valuesJSON, err := json.Marshal(row.Values)
	if err != nil {
		return fmt.Errorf("dockb: marshal values: %w", err)
	}
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
UPDATE wk_database_rows SET values = ?::jsonb, updated_at = NOW()
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
			string(valuesJSON), row.TenantID, row.ID,
		).Error
	}
	return r.db.WithContext(ctx).Exec(`
UPDATE wk_database_rows SET values = ?, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
		string(valuesJSON), row.TenantID, row.ID,
	).Error
}

func (r *dockbRepository) GetRow(ctx context.Context, tenantID string, id uint64) (*types.WKDatabaseRow, error) {
	var row types.WKDatabaseRow
	err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, database_id, values, created_by, created_at, updated_at
FROM wk_database_rows WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL LIMIT 1`,
		tenantID, id).Scan(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *dockbRepository) ListRows(ctx context.Context, tenantID string, databaseID uint64, limit, offset int) ([]*types.WKDatabaseRow, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.WKDatabaseRow
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, database_id, values, created_by, created_at, updated_at
FROM wk_database_rows
WHERE tenant_id = ? AND database_id = ? AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT ? OFFSET ?`, tenantID, databaseID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM wk_database_rows WHERE tenant_id = ? AND database_id = ? AND deleted_at IS NULL`,
		tenantID, databaseID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *dockbRepository) DeleteRow(ctx context.Context, tenantID string, id uint64) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE wk_database_rows SET deleted_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ?`, tenantID, id).Error
}

// satisfy unused import warnings for cases where build flags strip db
var _ = sql.ErrNoRows
var _ = clause.OnConflict{}
