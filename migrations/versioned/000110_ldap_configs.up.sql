-- 000110 — LDAP directory server configs (per tenant)
--
-- Mirrors saml_idp_configs (migration 107): one row per tenant,
-- service holds the unique invariant. Bind password is stored
-- encrypted at rest by the service layer; the column type is
-- TEXT to fit envelope-encrypted payloads.
CREATE TABLE IF NOT EXISTS ldap_configs (
    id                  BIGSERIAL PRIMARY KEY,
    tenant_id           BIGINT       NOT NULL,
    name                VARCHAR(128) NOT NULL,
    host                VARCHAR(255) NOT NULL,
    port                INTEGER      NOT NULL DEFAULT 389,
    use_tls             BOOLEAN      NOT NULL DEFAULT FALSE,
    skip_verify         BOOLEAN      NOT NULL DEFAULT FALSE,
    bind_dn             VARCHAR(512) NOT NULL,
    bind_password       TEXT         NOT NULL,
    base_dn             VARCHAR(1024) NOT NULL,
    user_filter         VARCHAR(512),
    username_attr       VARCHAR(64),
    email_attr          VARCHAR(64),
    display_name_attr   VARCHAR(64),
    group_attr          VARCHAR(64),
    group_search_base_dn VARCHAR(1024),
    group_filter        VARCHAR(512),
    vendor              VARCHAR(32),
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP,
    deleted_at          TIMESTAMP
);

-- One directory per tenant (mirrors saml_idp_configs).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_ldap_tenant
    ON ldap_configs (tenant_id);

-- Tenant-scoped lookups for the admin diagnostics view.
CREATE INDEX IF NOT EXISTS idx_ldap_configs_enabled
    ON ldap_configs (enabled) WHERE deleted_at IS NULL;
