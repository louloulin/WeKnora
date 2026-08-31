-- 000111 — LDAP federation identities (mirrors
-- user_saml_federation_identities, migration 108)
--
-- Each row says "this directory entry on this server is this local
-- user". The (tenant_id, ldap_config_id, entry_dn) triple is the
-- lookup key on every login.
CREATE TABLE IF NOT EXISTS user_ldap_federation_identities (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    ldap_config_id  BIGINT       NOT NULL,
    entry_dn        VARCHAR(1024) NOT NULL,
    entry_uuid      VARCHAR(128),
    user_id         BIGINT       NOT NULL,
    username        VARCHAR(256) NOT NULL,
    email           VARCHAR(255),
    revoked         BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMP,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP,
    deleted_at      TIMESTAMP
);

-- The lookup key for "find the local user behind this directory
-- entry".
CREATE UNIQUE INDEX IF NOT EXISTS uniq_ldap_fed
    ON user_ldap_federation_identities (tenant_id, ldap_config_id, entry_dn);

-- Reverse lookup for the admin UI ("Alice is bound to ...").
CREATE INDEX IF NOT EXISTS idx_ldap_fed_user
    ON user_ldap_federation_identities (user_id);

-- Stable across renames when the directory publishes a UUID.
CREATE INDEX IF NOT EXISTS idx_ldap_fed_uuid
    ON user_ldap_federation_identities (entry_uuid);
