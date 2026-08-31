-- SQLite mirror of migrations/versioned 000116.
CREATE TABLE IF NOT EXISTS authz_policy_versions (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    policy_key      VARCHAR(128) NOT NULL,
    version         INTEGER      NOT NULL,
    expression      TEXT         NOT NULL,
    decision        VARCHAR(32)  NOT NULL,
    metadata        TEXT         NOT NULL DEFAULT '{}',
    created_by      INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, policy_key, version)
);
CREATE INDEX IF NOT EXISTS idx_authz_versions_tenant ON authz_policy_versions (tenant_id, policy_key);
