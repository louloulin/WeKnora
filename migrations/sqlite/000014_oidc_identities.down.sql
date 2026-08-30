DROP TABLE IF EXISTS user_oidc_identities;
ALTER TABLE tenant_members DROP COLUMN role_source;
ALTER TABLE users DROP COLUMN oidc_managed_system_admin;
