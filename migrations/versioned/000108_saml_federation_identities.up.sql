-- Migration 108: per-user SAML federation bindings
--
-- One row per (IdP, NameID) tuple. The unique index on
-- (idp_entity_id, name_id) is the lookup surface for the SAML ACS
-- handler: the IdP POSTs the assertion, we look up the binding by
-- (IdP entity id + NameID) and load the corresponding WeKnora user.
--
-- tenant_id is denormalised so the ACS path can answer
-- 'is this user a member of the tenant this IdP is configured
-- for?' without a join to the saml_idp_configs table.
--
-- (user_id, tenant_id) supports the inverse lookup used by the
-- admin UI: 'which IdPs is this user federated with?'.

CREATE TABLE IF NOT EXISTS user_saml_federation_identities (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    tenant_id BIGINT NOT NULL,
    idp_entity_id VARCHAR(512) NOT NULL,
    name_id VARCHAR(512) NOT NULL,
    name_id_format VARCHAR(64) NOT NULL DEFAULT 'emailAddress',
    session_index VARCHAR(128) NOT NULL DEFAULT '',
    email_at_last_login VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT uniq_saml_fed UNIQUE (idp_entity_id, name_id)
);

CREATE INDEX IF NOT EXISTS idx_saml_fed_user ON user_saml_federation_identities (user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_saml_fed_revoked ON user_saml_federation_identities (revoked_at) WHERE revoked_at IS NOT NULL;
