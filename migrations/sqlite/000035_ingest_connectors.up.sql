-- SQLite variant of the v0.7.24 ingest connector tables.
CREATE TABLE IF NOT EXISTS ingest_connectors (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    kind            VARCHAR(64) NOT NULL,
    config          TEXT NOT NULL DEFAULT '{}',
    knowledge_base_id VARCHAR(36) NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_sync_at    DATETIME,
    last_error      TEXT NOT NULL DEFAULT '',
    created_by      VARCHAR(36) NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_ingest_connectors_tenant
    ON ingest_connectors (tenant_id, kind, created_at DESC);

CREATE TABLE IF NOT EXISTS ingest_jobs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    connector_id    INTEGER NOT NULL REFERENCES ingest_connectors(id) ON DELETE CASCADE,
    status          VARCHAR(32) NOT NULL DEFAULT 'queued',
    result_count    INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    started_at      DATETIME,
    finished_at     DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_connector
    ON ingest_jobs (connector_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_tenant
    ON ingest_jobs (tenant_id, created_at DESC);
