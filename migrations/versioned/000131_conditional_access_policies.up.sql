-- Conditional Access policies (Build v0.7.14). Each row is one rule
-- that the login flow evaluates against the request context (IP,
-- time, device, geo) and the authenticated principal (user / role).
--
-- Conditions are stored as JSONB so the schema stays small and new
-- condition types can (be added without a migration. The evaluator
-- knows how to walk each shape; unknown keys are ignored.
--
-- Priority breaks ties: when multiple policies match, the highest
-- priority (lowest number) wins. Ties at the same priority default
-- to deny-on-ambiguity so the audit log records why.
--
-- enabled=false is the soft-disable switch — useful for staging a
-- rule before turning it on for real. The handler still returns the
-- row in list queries so admins can see disabled policies.
--
-- tenant_id is denormalised so a per-tenant list query does not
-- have to join through anywhere. The composite index on
-- (tenant_id, enabled, priority) supports the hot read path:
--   SELECT ... FROM conditional_access_policies
--   WHERE tenant_id = ? AND enabled = TRUE
--   ORDER BY priority ASC
CREATE TABLE IF NOT EXISTS conditional_access_policies (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   VARCHAR(36)  NOT NULL,
    name        VARCHAR(128) NOT NULL,
    description TEXT,
    conditions   JSONB NOT NULL DEFAULT '{}'::jsonb,
    action      VARCHAR(32)  NOT NULL DEFAULT,
    priority    INT          NOT NULL DEFAULT 100,
    enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by  VARCHAR(36)  NOT NULL DEFAULT '',
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cond_acc_tenant_enabled_priority
    ON conditional_access_policies (tenant_id, enabled, priority)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_cond_acc_tenant_name
    ON conditional_access_policies (tenant_id, name)
    WHERE deleted_at IS NULL;
