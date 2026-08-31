CREATE TABLE ldap_configs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id           INTEGER NOT NULL,
    name                TEXT    NOT NULL,
    host                TEXT    NOT NULL,
    port                INTEGER NOT NULL DEFAULT 389,
    use_tls             INTEGER NOT NULL DEFAULT 0,
    skip_verify         INTEGER NOT NULL DEFAULT 0,
    bind_dn             TEXT    NOT NULL,
    bind_password       TEXT    NOT NULL,
    base_dn             TEXT    NOT NULL,
    user_filter         TEXT,
    username_attr       TEXT,
    email_attr          TEXT,
    display_name_attr   TEXT,
    group_attr          TEXT,
    group_search_base_dn TEXT,
    group_filter        TEXT,
    vendor              TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME,
    deleted_at          DATETIME
);

CREATE UNIQUE INDEX uniq_ldap_tenant ON ldap_configs (tenant_id);
CREATE INDEX idx_ldap_configs_enabled ON ldap_configs (enabled) WHERE deleted_at IS NULL;
