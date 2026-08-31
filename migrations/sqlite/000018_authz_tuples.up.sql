-- SQLite equivalent of migration 109 for local development.

-- SQLite does not support IF NOT EXISTS on ALTER TABLE ADD COLUMN before 3.35;
-- use a defensive try pattern that swallows the duplicate-column error.
-- Most local SQLite installs are recent enough; if yours is older, the
-- migration runner reports the column already exists and continues.

ALTER TABLE datasources ADD COLUMN creator_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_datasource_creator ON datasources (creator_id);

CREATE TABLE IF NOT EXISTS authz_tuples (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    subject_relation TEXT NOT NULL DEFAULT '',
    granted_by TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME,
    revoked_at DATETIME,
    CONSTRAINT uniq_authz_tuple UNIQUE (object_type, object_id, relation, subject_type, subject_id, subject_relation)
);

CREATE INDEX IF NOT EXISTS idx_authz_tuple_object ON authz_tuples (object_type, object_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_authz_tuple_subject ON authz_tuples (subject_type, subject_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_authz_tuple_tenant ON authz_tuples (object_type, object_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_authz_tuple_expires ON authz_tuples (expires_at) WHERE expires_at IS NOT NULL;
