-- v0.7.32 Build #34.x — sqlite mirror of 000139.
CREATE TABLE plugin_vendors (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    public_key      TEXT NOT NULL,
    contact_email   TEXT NOT NULL DEFAULT '',
    verified        INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE plugins (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id       TEXT NOT NULL,
    version         TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    author          TEXT NOT NULL,
    homepage        TEXT NOT NULL DEFAULT '',
    permissions     TEXT NOT NULL DEFAULT '[]',
    entry_point     TEXT NOT NULL DEFAULT '',
    artifact_url    TEXT NOT NULL DEFAULT '',
    artifact_sha256 TEXT NOT NULL DEFAULT '',
    icon_url        TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'submitted',
    trust_tier      TEXT NOT NULL DEFAULT 'basic',
    downloads       INTEGER NOT NULL DEFAULT 0,
    submitted_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at     DATETIME,
    reviewer_note   TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (plugin_id, version)
);
CREATE INDEX idx_plugins_status ON plugins(status);
CREATE INDEX idx_plugins_trust_tier ON plugins(trust_tier);
CREATE INDEX idx_plugins_author ON plugins(author);
CREATE INDEX idx_plugins_updated_at ON plugins(updated_at DESC);

CREATE TABLE tenant_plugins (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     INTEGER NOT NULL,
    plugin_id     TEXT NOT NULL,
    version       TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    permissions   TEXT NOT NULL DEFAULT '[]',
    installed_by  TEXT NOT NULL DEFAULT '',
    installed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, plugin_id)
);
CREATE INDEX idx_tenant_plugins_tenant ON tenant_plugins(tenant_id);

CREATE TABLE plugin_audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER NOT NULL DEFAULT 0,
    actor_id    TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    plugin_id   TEXT NOT NULL DEFAULT '',
    version     TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_plugin_audit_tenant_time ON plugin_audit_log(tenant_id, timestamp DESC);
CREATE INDEX idx_plugin_audit_action_time ON plugin_audit_log(action, timestamp DESC);
CREATE INDEX idx_plugin_audit_plugin ON plugin_audit_log(plugin_id, timestamp DESC);
