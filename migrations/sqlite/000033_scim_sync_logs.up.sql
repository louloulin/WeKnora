CREATE TABLE scim_sync_logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     INTEGER NOT NULL,
    token_id      INTEGER NOT NULL,
    method        TEXT    NOT NULL,
    path          TEXT    NOT NULL,
    resource_type TEXT,
    status        INTEGER NOT NULL,
    subject       TEXT,
    detail        TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    DATETIME
);

CREATE INDEX idx_scim_logs_tenant ON scim_sync_logs (tenant_id, created_at DESC);
CREATE INDEX idx_scim_logs_token  ON scim_sync_logs (token_id, created_at DESC);
