CREATE TABLE IF NOT EXISTS conditional_access_policies (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   VARCHAR(36)  NOT NULL,
    name        VARCHAR(128) NOT NULL,
    description TEXT,
    conditions   TEXT        NOT NULL DEFAULT,
    action      VARCHAR(32)  NOT NULL DEFAULT,
    priority    INT          NOT NULL DEFAULT 100,
    enabled     BOOLEAN      NOT NULL DEFAULT 1,
    created_by  VARCHAR(36)  NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_cond_acc_tenant_enabled_priority
    ON conditional_access_policies (tenant_id, enabled, priority);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cond_acc_tenant_name
    ON conditional_access_policies (tenant_id, name);
