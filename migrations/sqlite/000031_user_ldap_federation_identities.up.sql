CREATE TABLE user_ldap_federation_identities (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER NOT NULL,
    ldap_config_id  INTEGER NOT NULL,
    entry_dn        TEXT    NOT NULL,
    entry_uuid      TEXT,
    user_id         INTEGER NOT NULL,
    username        TEXT    NOT NULL,
    email           TEXT,
    revoked         INTEGER NOT NULL DEFAULT 0,
    last_login_at   DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME,
    deleted_at      DATETIME
);

CREATE UNIQUE INDEX uniq_ldap_fed
    ON user_ldap_federation_identities (tenant_id, ldap_config_id, entry_dn);
CREATE INDEX idx_ldap_fed_user
    ON user_ldap_federation_identities (user_id);
CREATE INDEX idx_ldap_fed_uuid
    ON user_ldap_federation_identities (entry_uuid);
