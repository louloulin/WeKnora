-- v0.7.31 Build #36 — sqlite mirror of 000137.
CREATE TABLE regions (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL,
    location      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    capacity_pct  INTEGER NOT NULL DEFAULT 0,
    latency_ms    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tenant_region_bindings (
    tenant_id        INTEGER PRIMARY KEY,
    primary_region   TEXT NOT NULL REFERENCES regions(id),
    residency_policy TEXT NOT NULL DEFAULT 'strict_local',
    replica_regions  TEXT NOT NULL DEFAULT '[]',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tenant_region_bindings_region ON tenant_region_bindings(primary_region);

CREATE TABLE cross_region_audit_log (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source_region  TEXT NOT NULL,
    target_region  TEXT NOT NULL,
    tenant_id      INTEGER NOT NULL,
    user_id        TEXT NOT NULL DEFAULT '',
    action         TEXT NOT NULL,
    resource_type  TEXT NOT NULL DEFAULT '',
    resource_id    TEXT NOT NULL DEFAULT '',
    allowed        INTEGER NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cross_region_audit_tenant_time ON cross_region_audit_log(tenant_id, timestamp DESC);
CREATE INDEX idx_cross_region_audit_denied_time ON cross_region_audit_log(allowed, timestamp DESC);
CREATE INDEX idx_cross_region_audit_action_time ON cross_region_audit_log(action, timestamp DESC);

INSERT OR IGNORE INTO regions (id, display_name, location) VALUES
    ('us-east-1', 'US East (N. Virginia)', 'United States'),
    ('us-west-2', 'US West (Oregon)', 'United States'),
    ('eu-central-1', 'EU Central (Frankfurt)', 'European Union'),
    ('ap-northeast-1', 'APAC Northeast (Tokyo)', 'Asia Pacific'),
    ('dev-local', 'Local Dev', 'Local');
