-- SQLite mirror of migrations/versioned 000115.
CREATE TABLE IF NOT EXISTS dlp_policies (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    name            VARCHAR(128) NOT NULL,
    version         INTEGER      NOT NULL DEFAULT 1,
    resource_scope  VARCHAR(64)  NOT NULL DEFAULT '*',
    severity        VARCHAR(32)  NOT NULL DEFAULT 'medium',
    action          VARCHAR(32)  NOT NULL DEFAULT 'log',
    is_active       BOOLEAN      NOT NULL DEFAULT FALSE,
    description     TEXT         NOT NULL DEFAULT '',
    created_by      INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX IF NOT EXISTS idx_dlp_policies_tenant ON dlp_policies (tenant_id, is_active);

CREATE TABLE IF NOT EXISTS dlp_rules (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    policy_id       INTEGER      NOT NULL,
    tenant_id       INTEGER      NOT NULL,
    pattern_type    VARCHAR(32)  NOT NULL,
    pattern_value   TEXT         NOT NULL,
    severity        VARCHAR(32)  NOT NULL DEFAULT 'medium',
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    description     TEXT         NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dlp_rules_policy ON dlp_rules (policy_id);

CREATE TABLE IF NOT EXISTS dlp_violations (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    policy_id       INTEGER      NOT NULL,
    rule_id         INTEGER,
    resource        VARCHAR(64)  NOT NULL,
    resource_id     VARCHAR(36)  NOT NULL,
    actor_id        INTEGER      NOT NULL,
    matched_value   TEXT         NOT NULL,
    context         TEXT         NOT NULL DEFAULT '',
    matched_pattern VARCHAR(128) NOT NULL,
    action_taken    VARCHAR(32)  NOT NULL,
    severity        VARCHAR(32)  NOT NULL,
    audit_log_id    INTEGER,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dlp_violations_tenant ON dlp_violations (tenant_id, created_at DESC);
