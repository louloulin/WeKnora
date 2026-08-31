-- v0.7.31 Build #37 — sqlite mirror of 000136.
CREATE TABLE workflows (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    kb_id       TEXT NOT NULL,
    name        TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    enabled     INTEGER NOT NULL DEFAULT 0,
    node_blob   TEXT NOT NULL DEFAULT '[]',
    edge_blob   TEXT NOT NULL DEFAULT '[]',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workflow_runs (
    id            TEXT PRIMARY KEY,
    workflow_id   TEXT NOT NULL,
    tenant_id     TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'queued',
    triggered_by  TEXT NOT NULL DEFAULT 'manual',
    input         TEXT NOT NULL DEFAULT '{}',
    output        TEXT NOT NULL DEFAULT '{}',
    error         TEXT NOT NULL DEFAULT '',
    started_at    DATETIME,
    finished_at   DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workflow_node_runs (
    id            TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL,
    node_id       TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'queued',
    input         TEXT NOT NULL DEFAULT '{}',
    output        TEXT NOT NULL DEFAULT '{}',
    error         TEXT NOT NULL DEFAULT '',
    started_at    DATETIME,
    finished_at   DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_workflows_tenant_kb ON workflows(tenant_id, kb_id);
CREATE INDEX idx_workflow_runs_workflow ON workflow_runs(workflow_id);
CREATE INDEX idx_workflow_runs_tenant ON workflow_runs(tenant_id);
CREATE INDEX idx_workflow_node_runs_run ON workflow_node_runs(run_id);
