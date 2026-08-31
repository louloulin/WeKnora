-- AI Connector framework (Build v0.7.24). Each tenant can register
-- external sources (Slack / Email / Webhook / RSS / Confluence /
-- Notion / Jira / Salesforce / HubSpot / ...) that flow messages
-- into the KB on demand or on a schedule. The "config" column
-- holds the per-source credentials and parameters — the service
-- layer is responsible for encrypting it before insert, so this
-- migration stores the JSON as-is and trusts the application to
-- handle secrecy.
--
-- The connector kind enum is intentionally permissive: new
-- connectors can be added in code without a schema change.
CREATE TABLE IF NOT EXISTS ingest_connectors (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    kind            VARCHAR(64) NOT NULL,
    config          JSONB NOT NULL DEFAULT '{}'::jsonb,
    knowledge_base_id VARCHAR(36) NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_at    TIMESTAMP,
    last_error      TEXT NOT NULL DEFAULT '',
    created_by      VARCHAR(36) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ingest_connectors_tenant
    ON ingest_connectors (tenant_id, kind, created_at DESC)
    WHERE deleted_at IS NULL;

-- ingest_jobs records one sync attempt. One row per run, regardless
-- of how many items the connector fetched. The result_count column
-- tells operators at a glance whether a job did real work or
-- returned zero new items (a useful signal for connector health).
CREATE TABLE IF NOT EXISTS ingest_jobs (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    connector_id    BIGINT NOT NULL REFERENCES ingest_connectors(id) ON DELETE CASCADE,
    status          VARCHAR(32) NOT NULL DEFAULT 'queued',
    result_count    INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMP,
    finished_at     TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_connector
    ON ingest_jobs (connector_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ingest_jobs_tenant
    ON ingest_jobs (tenant_id, created_at DESC);
