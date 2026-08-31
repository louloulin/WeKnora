CREATE TABLE IF NOT EXISTS user_saml_federation_identities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    tenant_id INTEGER NOT NULL,
    idp_entity_id TEXT NOT NULL,
    name_id TEXT NOT NULL,
    name_id_format TEXT NOT NULL DEFAULT 'emailAddress',
    session_index TEXT NOT NULL DEFAULT '',
    email_at_last_login TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    last_login_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at DATETIME,
    CONSTRAINT uniq_saml_fed UNIQUE (idp_entity_id, name_id)
);

CREATE INDEX IF NOT EXISTS idx_saml_fed_user ON user_saml_federation_identities (user_id, tenant_id);
