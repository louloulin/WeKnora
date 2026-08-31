-- v0.7.31 Build #36 — Multi-region + Data Residency foundation.
CREATE TABLE regions (
    id            VARCHAR(32) PRIMARY KEY,
    display_name  VARCHAR(64) NOT NULL,
    location      VARCHAR(64) NOT NULL DEFAULT '',
    status        VARCHAR(16) NOT NULL DEFAULT 'active',
    capacity_pct  INTEGER NOT NULL DEFAULT 0,
    latency_ms    INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_region_bindings (
    tenant_id        BIGINT PRIMARY KEY,
    primary_region   VARCHAR(32) NOT NULL REFERENCES regions(id),
    residency_policy VARCHAR(32) NOT NULL DEFAULT 'strict_local',
    replica_regions  JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tenant_region_bindings_region ON tenant_region_bindings(primary_region);

CREATE TABLE cross_region_audit_log (
    id             BIGSERIAL PRIMARY KEY,
    source_region  VARCHAR(32) NOT NULL,
    target_region  VARCHAR(32) NOT NULL,
    tenant_id      BIGINT NOT NULL,
    user_id        VARCHAR(64) NOT NULL DEFAULT '',
    action         VARCHAR(16) NOT NULL,
    resource_type  VARCHAR(64) NOT NULL DEFAULT '',
    resource_id    VARCHAR(128) NOT NULL DEFAULT '',
    allowed        BOOLEAN NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    "timestamp"    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cross_region_audit_tenant_time ON cross_region_audit_log(tenant_id, "timestamp" DESC);
CREATE INDEX idx_cross_region_audit_denied_time ON cross_region_audit_log(allowed, "timestamp" DESC) WHERE allowed = false;
CREATE INDEX idx_cross_region_audit_action_time ON cross_region_audit_log(action, "timestamp" DESC);

-- Seed catalog with the five supported regions so /regions returns
-- useful data out of the box.
INSERT INTO regions (id, display_name, location) VALUES
    ('us-east-1', 'US East (N. Virginia)', 'United States'),
    ('us-west-2', 'US West (Oregon)', 'United States'),
    ('eu-central-1', 'EU Central (Frankfurt)', 'European Union'),
    ('ap-northeast-1', 'APAC Northeast (Tokyo)', 'Asia Pacific'),
    ('dev-local', 'Local Dev', 'Local')
ON CONFLICT (id) DO NOTHING;
