-- WeKnora Base — Database / 多维表 (Build v0.7.23). A wk_database is a
-- tenant-scoped schema + row store that any wiki page or knowledge
-- chunk can attach to. Modeled after Notion databases and Feishu
-- 多维表格, but designed to live in the same tenant as wiki / KB so
-- we can use the existing authz + audit infrastructure.
--
-- schema is JSONB describing the field set: name, type, options,
-- validation. The schema is the source of truth for what rows can
-- carry — if a row carries an unknown field it is rejected at write
-- time.
CREATE TABLE IF NOT EXISTS wk_databases (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    schema          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_by      VARCHAR(36) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_wk_databases_tenant
    ON wk_databases (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- row values are JSONB so we can store arbitrary shapes matching the
-- schema. A real database would normalise fields; here we trade a bit
-- of write efficiency for the ability to evolve schema without ALTER
-- TABLE on hot paths.
CREATE TABLE IF NOT EXISTS wk_database_rows (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    database_id     BIGINT NOT NULL REFERENCES wk_databases(id) ON DELETE CASCADE,
    values          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by      VARCHAR(36) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_wk_database_rows_db
    ON wk_database_rows (database_id, created_at DESC)
    WHERE deleted_at IS NULL;
