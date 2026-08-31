-- v0.7.25 — Audit exports (Build #24, G04)
-- Tracks on-demand audit log exports + monthly compliance reports. The
-- row itself is a snapshot of the (tenant, filter) tuple + metadata; the
-- actual CSV/JSON is generated on demand from audit_logs.
CREATE TABLE IF NOT EXISTS audit_exports (
    id              VARCHAR(36) PRIMARY KEY,
    tenant_id       BIGINT NOT NULL,
    requested_by    VARCHAR(64) NOT NULL,
    format          VARCHAR(16) NOT NULL,
    filter_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    row_count       BIGINT NOT NULL DEFAULT 0,
    byte_size       BIGINT NOT NULL DEFAULT 0,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at     TIMESTAMP,
    expires_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_created
    ON audit_exports(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_status
    ON audit_exports(tenant_id, status);
