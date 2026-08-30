DROP TABLE IF EXISTS user_oidc_identities;
ALTER TABLE tenant_members DROP COLUMN IF EXISTS role_source;
ALTER TABLE users DROP COLUMN IF EXISTS oidc_managed_system_admin;
ALTER TABLE tenant_members DROP COLUMN IF EXISTS role_source;
ALTER TABLE users DROP COLUMN IF EXISTS oidc_managed_system_admin;
