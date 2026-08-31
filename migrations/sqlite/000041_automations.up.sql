-- v0.7.27 Build #33 — Automation / Button engine.
CREATE TABLE  automations (
    id              VARCHAR(64)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    knowledge_base_id VARCHAR(64) NOT NULL,
    database_id     VARCHAR(64)  NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    trigger_type    VARCHAR(32)  NOT NULL,
    trigger_config  JSON        NOT NULL DEFAULT '{}',
    enabled         INTEGER      NOT NULL DEFAULT TRUE,
    steps           JSON        NOT NULL DEFAULT '[]',
    created_by      BIGINT       NOT NULL DEFAULT 0,
    created_at      TIMESTAMP  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP  NOT NULL DEFAULT NOW(),
    last_fired_at   TIMESTAMP,
    last_fire_status VARCHAR(32) NOT NULL DEFAULT ''
);

CREATE INDEX  idx_automations_tenant_db ON automations(tenant_id, database_id);
CREATE INDEX  idx_automations_trigger ON automations(trigger_type) WHERE enabled = TRUE;

CREATE TABLE  automation_runs (
    id              VARCHAR(64)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    automation_id   VARCHAR(64)  NOT NULL,
    trigger         VARCHAR(32)  NOT NULL,
    status          VARCHAR(32)  NOT NULL,
    started_at      TIMESTAMP  NOT NULL,
    finished_at     TIMESTAMP,
    step_runs       JSON        NOT NULL DEFAULT '[]',
    error           TEXT         NOT NULL DEFAULT '',
    retried_count   INT          NOT NULL DEFAULT 0
);

CREATE INDEX  idx_automation_runs_automation ON automation_runs(automation_id, started_at DESC);
CREATE INDEX  idx_automation_runs_status ON automation_runs(status, started_at DESC);
