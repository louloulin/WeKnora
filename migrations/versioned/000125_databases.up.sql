-- v0.7.25 — Multi-view Database (Build #26, G06)
-- Adds the schema for the multi-view database (Notion / Feishu Base / Tana
-- parity). A database belongs to a knowledge base; fields are typed columns;
-- rows are JSONB blobs that satisfy the field schema; views persist per-user
-- filters / sorts / groups / hidden columns for the 6 supported view types.
-- Migration is idempotent so re-runs on a partially-applied DB are safe.
CREATE TABLE IF NOT EXISTS databases (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    icon            VARCHAR(64) NOT NULL DEFAULT '',
    created_by      VARCHAR(64) NOT NULL,
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
    -- Field type: text | number | select | multi_select | date | person |
    -- checkbox | url | email | phone | formula | relation | rollup
    type            VARCHAR(32) NOT NULL,
    -- Type-specific options (select choices, formula expr, relation target).
    options         JSONB NOT NULL DEFAULT '{}'::jsonb,
    width           INT NOT NULL DEFAULT 160,
    sort_order      INT NOT NULL DEFAULT 0,
    is_primary      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_database_fields_db
        FOREIGN KEY (database_id) REFERENCES databases(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_database_fields_db_order
    ON database_fields(database_id, sort_order);

CREATE TABLE IF NOT EXISTS database_rows (
    id              VARCHAR(36) PRIMARY KEY,
    database_id     VARCHAR(36) NOT NULL,
    -- Row values as JSONB keyed by field_id. The shape is unconstrained at
    -- the DB level (any field type may add new keys); the service layer is
    -- the source of truth for value validation.
    data            JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order      INT NOT NULL DEFAULT 0,
    created_by      VARCHAR(64) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP,
    CONSTRAINT fk_database_rows_db
        FOREIGN KEY (database_id) REFERENCES databases(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_database_rows_db_order
    ON database_rows(database_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_database_rows_db_updated
    ON database_rows(database_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS database_views (
    id              VARCHAR(36) PRIMARY KEY,
    database_id     VARCHAR(36) NOT NULL,
    -- View type: table | board | gallery | calendar | timeline | list
    type            VARCHAR(32) NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    -- JSON config: {filters, sorts, groups, hiddenFields, boardGroupField,
    --   calendarDateField, timelineStartField, timelineEndField, ...}
    config          JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order      INT NOT NULL DEFAULT 0,
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    created_by      VARCHAR(64) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_database_views_db
        FOREIGN KEY (database_id) REFERENCES databases(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_database_views_db_order
    ON database_views(database_id, sort_order);
