-- 000113 — SCIM sync audit log
--
-- One row per SCIM request. Append-only; retention is handled by
-- the existing audit_log_retention cron (90 days default).
CREATE TABLE IF NOT EXISTS scim_sync_logs (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT       NOT NULL,
    token_id      BIGINT       NOT NULL,
    method        VARCHAR(10)  NOT NULL,
    path          VARCHAR(255) NOT NULL,
    resource_type VARCHAR(32),
    status        INTEGER      NOT NULL,
    subject       VARCHAR(255),
    detail        TEXT,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scim_logs_tenant
    ON scim_sync_logs (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_scim_logs_token
    ON scim_sync_logs (token_id, created_at DESC);
