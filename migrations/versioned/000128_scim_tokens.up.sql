-- 000112 — SCIM 2.0 bearer tokens (per tenant)
--
-- One row per tenant. Token is stored as SHA-256 hex so a database
-- leak cannot replay the credential against /scim/v2/*. The IdP is
-- expected to refresh the credential out-of-band; we expose
-- Revoked + ExpiresAt for revocation flow but do not auto-expire.
CREATE TABLE IF NOT EXISTS scim_tokens (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT       NOT NULL,
    name          VARCHAR(128) NOT NULL,
    token_hash    VARCHAR(128) NOT NULL,
    token_prefix  VARCHAR(16)  NOT NULL,
    created_by    VARCHAR(36)  NOT NULL,
    last_used_at  TIMESTAMP,
    expires_at    TIMESTAMP,
    revoked       BOOLEAN      NOT NULL DEFAULT FALSE,
    revoked_at    TIMESTAMP,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP,
    deleted_at    TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_scim_token_tenant
    ON scim_tokens (tenant_id);

-- The hot-path lookup index for bearer-token auth.
CREATE INDEX IF NOT EXISTS idx_scim_tokens_hash
    ON scim_tokens (token_hash);

-- Tenant diagnostics view: "which token last did what".
CREATE INDEX IF NOT EXISTS idx_scim_tokens_active
    ON scim_tokens (tenant_id) WHERE revoked = FALSE AND deleted_at IS NULL;
