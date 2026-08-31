CREATE TABLE IF NOT EXISTS audit_exports (
    id              TEXT PRIMARY KEY,
    tenant_id       INTEGER NOT NULL,
    requested_by    TEXT NOT NULL,
    format          TEXT NOT NULL,
    filter_json     TEXT NOT NULL DEFAULT '{}',
    row_count       INTEGER NOT NULL DEFAULT 0,
    byte_size       INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    error           TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at     DATETIME,
    expires_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_created ON audit_exports(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_status ON audit_exports(tenant_id, status);
