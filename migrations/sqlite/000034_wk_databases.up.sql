-- SQLite variant for the lite profile.
CREATE TABLE IF NOT EXISTS wk_databases (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    schema          TEXT NOT NULL DEFAULT '[]',
    created_by      VARCHAR(36) NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_wk_databases_tenant
    ON wk_databases (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS wk_database_rows (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       VARCHAR(36) NOT NULL,
    database_id     INTEGER NOT NULL REFERENCES wk_databases(id) ON DELETE CASCADE,
    values          TEXT NOT NULL DEFAULT '{}',
    created_by      VARCHAR(36) NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_wk_database_rows_db
    ON wk_database_rows (database_id, created_at DESC);
