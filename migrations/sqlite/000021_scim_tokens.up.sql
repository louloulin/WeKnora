CREATE TABLE scim_tokens (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     INTEGER NOT NULL,
    name          TEXT    NOT NULL,
    token_hash    TEXT    NOT NULL,
    token_prefix  TEXT    NOT NULL,
    created_by    TEXT    NOT NULL,
    last_used_at  DATETIME,
    expires_at    DATETIME,
    revoked       INTEGER NOT NULL DEFAULT 0,
    revoked_at    DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME,
    deleted_at    DATETIME
);

CREATE UNIQUE INDEX uniq_scim_token_tenant ON scim_tokens (tenant_id);
CREATE INDEX idx_scim_tokens_hash ON scim_tokens (token_hash);
CREATE INDEX idx_scim_tokens_active ON scim_tokens (tenant_id) WHERE revoked = 0 AND deleted_at IS NULL;
