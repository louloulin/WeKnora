-- v0.7.32 Build #34.x — Plugin Signing + Marketplace foundation.
CREATE TABLE plugin_vendors (
    id              BIGSERIAL PRIMARY KEY,
    slug            VARCHAR(64) NOT NULL UNIQUE,
    name            VARCHAR(128) NOT NULL,
    public_key      TEXT NOT NULL,
    contact_email   VARCHAR(128) NOT NULL DEFAULT '',
    verified        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE plugins (
    id              BIGSERIAL PRIMARY KEY,
    plugin_id       VARCHAR(64) NOT NULL,
    version         VARCHAR(32) NOT NULL,
    name            VARCHAR(128) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    author          VARCHAR(64) NOT NULL,
    homepage        VARCHAR(255) NOT NULL DEFAULT '',
    permissions     JSONB NOT NULL DEFAULT '[]'::jsonb,
    entry_point     VARCHAR(255) NOT NULL DEFAULT '',
    artifact_url    VARCHAR(255) NOT NULL DEFAULT '',
    artifact_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    icon_url        VARCHAR(255) NOT NULL DEFAULT '',
    status          VARCHAR(16) NOT NULL DEFAULT 'submitted',
    trust_tier      VARCHAR(16) NOT NULL DEFAULT 'basic',
    downloads       INTEGER NOT NULL DEFAULT 0,
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at     TIMESTAMPTZ,
    reviewer_note   TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (plugin_id, version)
);
CREATE INDEX idx_plugins_status ON plugins(status);
CREATE INDEX idx_plugins_trust_tier ON plugins(trust_tier);
CREATE INDEX idx_plugins_author ON plugins(author);
CREATE INDEX idx_plugins_updated_at ON plugins(updated_at DESC);

CREATE TABLE tenant_plugins (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT NOT NULL,
    plugin_id     VARCHAR(64) NOT NULL,
    version       VARCHAR(32) NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT true,
    permissions   JSONB NOT NULL DEFAULT '[]'::jsonb,
    installed_by  VARCHAR(64) NOT NULL DEFAULT '',
    installed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, plugin_id)
);
CREATE INDEX idx_tenant_plugins_tenant ON tenant_plugins(tenant_id);

CREATE TABLE plugin_audit_log (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL DEFAULT 0,
    actor_id    VARCHAR(64) NOT NULL DEFAULT '',
    action      VARCHAR(16) NOT NULL,
    plugin_id   VARCHAR(64) NOT NULL DEFAULT '',
    version     VARCHAR(32) NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_plugin_audit_tenant_time ON plugin_audit_log(tenant_id, "timestamp" DESC);
CREATE INDEX idx_plugin_audit_action_time ON plugin_audit_log(action, "timestamp" DESC);
CREATE INDEX idx_plugin_audit_plugin ON plugin_audit_log(plugin_id, "timestamp" DESC);
