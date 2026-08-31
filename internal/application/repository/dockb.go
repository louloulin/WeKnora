package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

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
