-- v0.7.25 — Multi-view Database (Build #26, G06)
-- SQLite variant of 000125_databases. The shape matches; SQLite doesn't
-- support adding foreign keys in a portable way across all column types,
-- so we lean on the service layer for referential integrity.
CREATE TABLE IF NOT EXISTS databases (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       INTEGER NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    icon            TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_databases_tenant_kb
    ON databases(tenant_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_databases_kb_updated
    ON databases(knowledge_base_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS database_fields (
    id              VARCHAR(36) PRIMARY KEY,
    database_id     VARCHAR(36) NOT NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL,
    options         TEXT NOT NULL DEFAULT '{}',
    width           INTEGER NOT NULL DEFAULT 160,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    is_primary      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_database_fields_db_order
    ON database_fields(database_id, sort_order);

CREATE TABLE IF NOT EXISTS database_rows (
    id              VARCHAR(36) PRIMARY KEY,
    database_id     VARCHAR(36) NOT NULL,
    data            TEXT NOT NULL DEFAULT '{}',
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_database_rows_db_order
    ON database_rows(database_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_database_rows_db_updated
    ON database_rows(database_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS database_views (
    id              VARCHAR(36) PRIMARY KEY,
    database_id     VARCHAR(36) NOT NULL,
    type            TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    config          TEXT NOT NULL DEFAULT '{}',
    sort_order      INTEGER NOT NULL DEFAULT 0,
    is_default      INTEGER NOT NULL DEFAULT 0,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_database_views_db_order
    ON database_views(database_id, sort_order);
