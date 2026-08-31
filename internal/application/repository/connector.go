package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// connectorRepository implements IngestConnectorRepository and
// IngestJobRepository using cross-dialect raw SQL.
type connectorRepository struct {
	db *gorm.DB
}

// NewConnectorRepository wires the repo.
func NewConnectorRepository(db *gorm.DB) *connectorRepository {
	return &connectorRepository{db: db}
}

func (r *connectorRepository) dialect() string {
	if r.db.Dialector != nil && strings.Contains(r.db.Dialector.Name(), "postgres") {
		return "postgres"
	}
	return "sqlite"
}

// --- IngestConnector ---

func (r *connectorRepository) Create(ctx context.Context, c *types.IngestConnector) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Raw(`
INSERT INTO ingest_connectors (tenant_id, name, kind, config, knowledge_base_id, enabled, last_error, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, ?, NOW(), NOW())
RETURNING id, created_at, updated_at`,
			c.TenantID, c.Name, string(c.Kind), c.Config, c.KnowledgeBaseID, c.Enabled,
			c.LastError, c.CreatedBy,
		).Scan(c).Error
	}
	return r.db.WithContext(ctx).Raw(`
INSERT INTO ingest_connectors (tenant_id, name, kind, config, knowledge_base_id, enabled, last_error, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id, created_at, updated_at`,
		c.TenantID, c.Name, string(c.Kind), c.Config, c.KnowledgeBaseID, c.Enabled,
		c.LastError, c.CreatedBy,
	).Scan(c).Error
}

func (r *connectorRepository) Update(ctx context.Context, c *types.IngestConnector) error {
	if r.dialect() == "postgres" {
		return r.db.WithContext(ctx).Exec(`
UPDATE ingest_connectors
SET name = ?, kind = ?, config = ?::jsonb, knowledge_base_id = ?, enabled = ?, last_error = ?, updated_at = NOW()
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
			c.Name, string(c.Kind), c.Config, c.KnowledgeBaseID, c.Enabled, c.LastError,
			c.TenantID, c.ID,
		).Error
	}
	return r.db.WithContext(ctx).Exec(`
UPDATE ingest_connectors
SET name = ?, kind = ?, config = ?, knowledge_base_id = ?, enabled = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL`,
		c.Name, string(c.Kind), c.Config, c.KnowledgeBaseID, c.Enabled, c.LastError,
		c.TenantID, c.ID,
	).Error
}

func (r *connectorRepository) Get(ctx context.Context, tenantID string, id uint64) (*types.IngestConnector, error) {
	var c types.IngestConnector
	err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, name, kind, config, knowledge_base_id, enabled, last_sync_at, last_error,
       created_by, created_at, updated_at
FROM ingest_connectors
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL LIMIT 1`,
		tenantID, id).Scan(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.ID == 0 {
		return nil, nil
	}
	return &c, nil
}

func (r *connectorRepository) List(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestConnector, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.IngestConnector
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, name, kind, config, knowledge_base_id, enabled, last_sync_at, last_error,
       created_by, created_at, updated_at
FROM ingest_connectors
WHERE tenant_id = ? AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, tenantID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM ingest_connectors WHERE tenant_id = ? AND deleted_at IS NULL`,
		tenantID).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *connectorRepository) Delete(ctx context.Context, tenantID string, id uint64) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE ingest_connectors SET deleted_at = CURRENT_TIMESTAMP
WHERE tenant_id = ? AND id = ?`, tenantID, id).Error
}

func (r *connectorRepository) TouchSync(ctx context.Context, id uint64, lastSyncAt time.Time, lastErr string) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE ingest_connectors SET last_sync_at = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, lastSyncAt, lastErr, id).Error
}

// --- IngestJob ---

func (r *connectorRepository) CreateJob(ctx context.Context, job *types.IngestJob) error {
	return r.db.WithContext(ctx).Raw(`
INSERT INTO ingest_jobs (tenant_id, connector_id, status, result_count, error, started_at, finished_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING id, created_at`,
		job.TenantID, job.ConnectorID, string(job.Status), job.ResultCount,
		job.Error, job.StartedAt, job.FinishedAt,
	).Scan(job).Error
}

func (r *connectorRepository) UpdateJob(ctx context.Context, job *types.IngestJob) error {
	return r.db.WithContext(ctx).Exec(`
UPDATE ingest_jobs
SET status = ?, result_count = ?, error = ?, started_at = ?, finished_at = ?
WHERE id = ?`,
		string(job.Status), job.ResultCount, job.Error, job.StartedAt, job.FinishedAt, job.ID,
	).Error
}

func (r *connectorRepository) GetJob(ctx context.Context, id uint64) (*types.IngestJob, error) {
	var job types.IngestJob
	err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, connector_id, status, result_count, error, started_at, finished_at, created_at
FROM ingest_jobs WHERE id = ? LIMIT 1`, id).Scan(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if job.ID == 0 {
		return nil, nil
	}
	return &job, nil
}

func (r *connectorRepository) ListJobsByConnector(ctx context.Context, tenantID string, connectorID uint64, limit, offset int) ([]*types.IngestJob, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.IngestJob
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, connector_id, status, result_count, error, started_at, finished_at, created_at
FROM ingest_jobs
WHERE tenant_id = ? AND connector_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, tenantID, connectorID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	_ = r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM ingest_jobs WHERE tenant_id = ? AND connector_id = ?`,
		tenantID, connectorID).Scan(&total).Error
	return rows, total, nil
}

func (r *connectorRepository) ListJobsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*types.IngestJob, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var rows []*types.IngestJob
	if err := r.db.WithContext(ctx).Raw(`
SELECT id, tenant_id, connector_id, status, result_count, error, started_at, finished_at, created_at
FROM ingest_jobs
WHERE tenant_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, tenantID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int
	_ = r.db.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM ingest_jobs WHERE tenant_id = ?`, tenantID).Scan(&total).Error
	return rows, total, nil
}
