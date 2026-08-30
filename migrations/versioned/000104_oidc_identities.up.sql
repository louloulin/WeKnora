-- Stable external OIDC identity bindings. Email is intentionally not the
-- identity key because it can change and may collide across organizations.
CREATE TABLE IF NOT EXISTS user_oidc_identities (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    issuer VARCHAR(512) NOT NULL,
    subject VARCHAR(512) NOT NULL,
    provider VARCHAR(64) NOT NULL DEFAULT 'oidc',
    email_at_last_login VARCHAR(255) NOT NULL DEFAULT '',
    last_login_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_user_oidc_identities_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_user_oidc_identity_issuer_subject UNIQUE (issuer, subject)
);

CREATE INDEX IF NOT EXISTS idx_user_oidc_identities_user_id ON user_oidc_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_oidc_identities_revoked_at ON user_oidc_identities(revoked_at);

ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_managed_system_admin BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_users_oidc_managed_system_admin ON users(oidc_managed_system_admin);
ALTER TABLE tenant_members ADD COLUMN IF NOT EXISTS role_source VARCHAR(20) NOT NULL DEFAULT 'local';
CREATE INDEX IF NOT EXISTS idx_tenant_members_role_source ON tenant_members(role_source);
