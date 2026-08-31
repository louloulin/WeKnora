-- Migration 107: SAML 2.0 per-tenant IdP configs (P0 enterprise IAM).
--
-- One row per tenant. The (tenant_id) unique index enforces the
-- invariant that a tenant has at most one IdP at a time; rotating
-- to a new IdP requires deleting or disabling the old one first
-- (handled by the service layer with a clear audit trail).
--
-- Certificate is stored base64-encoded. In production this column
-- is encrypted at rest by the repository wrapper (envelope
-- encryption via tenant secret KMS); the migration only stores
-- the raw value so the schema stays portable across deployments
-- without KMS.

CREATE TABLE IF NOT EXISTS saml_idp_configs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(128) NOT NULL,
    entity_id VARCHAR(512) NOT NULL,
    sso_url VARCHAR(1024) NOT NULL,
    slo_url VARCHAR(1024) NOT NULL DEFAULT '',
    certificate TEXT NOT NULL,
    name_id_format VARCHAR(32) NOT NULL DEFAULT 'email',
    attribute_map JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uniq_saml_idp_tenant UNIQUE (tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_saml_idp_enabled ON saml_idp_configs (enabled) WHERE deleted_at IS NULL;
