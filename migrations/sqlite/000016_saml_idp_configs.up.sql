-- SQLite equivalent of migration 107 for local development.
CREATE TABLE IF NOT EXISTS saml_idp_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    sso_url TEXT NOT NULL,
    slo_url TEXT NOT NULL DEFAULT '',
    certificate TEXT NOT NULL,
    name_id_format TEXT NOT NULL DEFAULT 'email',
    attribute_map TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    CONSTRAINT uniq_saml_idp_tenant UNIQUE (tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_saml_idp_enabled ON saml_idp_configs (enabled);
