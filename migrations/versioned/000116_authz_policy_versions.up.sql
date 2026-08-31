-- v0.7.22 — AuthZ policy version history (immutable)
--
-- Lays the storage foundation for the v0.7.22 AuthZ Admin UI. Each edit
-- to a policy (e.g. "wiki_page:read") creates a new immutable version
-- row; the live policy is the highest-version row. Old versions stay
-- queryable so the admin UI can diff v1 vs v2 and roll back atomically.
--
-- Combined with dlp_policies, this brings the policy-control surface
-- to parity with OPA Playground + Auth0 FGA + Styra DAS.
--
-- Rollback: 000116_authz_policy_versions.down.sql drops the table.
CREATE TABLE IF NOT EXISTS authz_policy_versions (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    -- policy key, e.g. "wiki_page:read", "kb:export", "custom_agent:run"
    policy_key      VARCHAR(128) NOT NULL,
    version         BIGINT       NOT NULL,
    -- DSL expression (OPA-like subset):
    --   allow if actor.role == "owner" and resource.tenant_id == actor.tenant_id
    --   deny  if actor.has_tag("contractor")
    -- We store the raw expression text; evaluation is handled by
    -- internal/authz/expr.go (Add #34).
    expression      TEXT         NOT NULL,
    -- decision: allow | deny | conditional
    decision        VARCHAR(32)  NOT NULL,
    -- free-form metadata: author, commit, ticket, reason
    metadata        TEXT         NOT NULL DEFAULT '{}',
    created_by      BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, policy_key, version)
);
CREATE INDEX IF NOT EXISTS idx_authz_versions_tenant ON authz_policy_versions (tenant_id, policy_key);
CREATE INDEX IF NOT EXISTS idx_authz_versions_latest  ON authz_policy_versions (tenant_id, policy_key, version DESC);
