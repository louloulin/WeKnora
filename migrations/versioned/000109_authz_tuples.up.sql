-- Migration 109: AuthZ phase-3 — persistent tuple store + datasource creator
--
-- Adds:
--   1. datasources.creator_id — records who created the datasource so
--      the authz.DatasourceAdapter can short-circuit on the creator
--      like KB / Agent / WikiPage do. Existing rows get '' (legacy).
--   2. authz_tuples — the persistent store for explicit per-object
--      relations (e.g. "user:abc#viewer@kb:42" or "group:eng#editor@agent:7").
--      Composite indexes make (object_type, object_id) the lookup key
--      for "who has any relation on this object?" and (subject_type,
--      subject_id) the lookup key for "what does this user have access
--      to?".
--
-- Tuple keys follow the OpenFGA-style "object#relation@subject"
-- convention so future expansion to graph queries / inherited
-- relations is straightforward.

ALTER TABLE datasources ADD COLUMN IF NOT EXISTS creator_id VARCHAR(36) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_datasource_creator ON datasources (creator_id);

CREATE TABLE IF NOT EXISTS authz_tuples (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    object_type VARCHAR(64) NOT NULL,
    object_id VARCHAR(64) NOT NULL,
    relation VARCHAR(32) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id VARCHAR(64) NOT NULL,
    subject_relation VARCHAR(32) NOT NULL DEFAULT '',
    granted_by VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT uniq_authz_tuple UNIQUE (object_type, object_id, relation, subject_type, subject_id, subject_relation)
);

CREATE INDEX IF NOT EXISTS idx_authz_tuple_object ON authz_tuples (object_type, object_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_authz_tuple_subject ON authz_tuples (subject_type, subject_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_authz_tuple_tenant ON authz_tuples (tenant_id, object_type);
CREATE INDEX IF NOT EXISTS idx_authz_tuple_expires ON authz_tuples (expires_at) WHERE expires_at IS NOT NULL;
