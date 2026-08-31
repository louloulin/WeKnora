-- v0.7.31 Build #37 — AI Workflow Builder foundation.
CREATE TABLE workflows (
    id          VARCHAR(36) PRIMARY KEY,
    tenant_id   VARCHAR(36) NOT NULL,
    kb_id       VARCHAR(36) NOT NULL,
    name        VARCHAR(256) NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    enabled     BOOLEAN NOT NULL DEFAULT false,
    node_blob   JSONB NOT NULL DEFAULT '[]'::jsonb,
    edge_blob   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workflow_runs (
    id            VARCHAR(36) PRIMARY KEY,
    workflow_id   VARCHAR(36) NOT NULL,
    tenant_id     VARCHAR(36) NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'queued',
    triggered_by  VARCHAR(64) NOT NULL DEFAULT 'manual',
    input         JSONB NOT NULL DEFAULT '{}'::jsonb,
    output        JSONB NOT NULL DEFAULT '{}'::jsonb,
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workflow_node_runs (
    id            VARCHAR(36) PRIMARY KEY,
    run_id        VARCHAR(36) NOT NULL,
    node_id       VARCHAR(36) NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'queued',
    input         JSONB NOT NULL DEFAULT '{}'::jsonb,
    output        JSONB NOT NULL DEFAULT '{}'::jsonb,
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflows_tenant_kb ON workflows(tenant_id, kb_id);
CREATE INDEX idx_workflow_runs_workflow ON workflow_runs(workflow_id);
CREATE INDEX idx_workflow_runs_tenant ON workflow_runs(tenant_id);
CREATE INDEX idx_workflow_node_runs_run ON workflow_node_runs(run_id);
