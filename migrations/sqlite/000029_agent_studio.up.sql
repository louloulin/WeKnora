-- SQLite mirror of migrations/versioned 000114.
CREATE TABLE IF NOT EXISTS agent_triggers (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    trigger_type    VARCHAR(32)  NOT NULL,
    name            VARCHAR(128) NOT NULL,
    trigger_config  TEXT         NOT NULL DEFAULT '{}',
    payload_template TEXT        NOT NULL DEFAULT '',
    status          VARCHAR(32)  NOT NULL DEFAULT 'active',
    last_fired_at   DATETIME,
    last_fire_status VARCHAR(32),
    next_fire_at    DATETIME,
    created_by      INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, agent_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agent_triggers_tenant ON agent_triggers (tenant_id, status);

CREATE TABLE IF NOT EXISTS agent_runs (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    trigger_id      INTEGER,
    triggered_by    VARCHAR(32)  NOT NULL,
    triggered_user  INTEGER,
    status          VARCHAR(32)  NOT NULL DEFAULT 'queued',
    input_payload   TEXT         NOT NULL DEFAULT '{}',
    output_payload  TEXT         NOT NULL DEFAULT '{}',
    error_message   TEXT         NOT NULL DEFAULT '',
    steps_count     INTEGER      NOT NULL DEFAULT 0,
    tokens_used     INTEGER      NOT NULL DEFAULT 0,
    cost_micros     INTEGER      NOT NULL DEFAULT 0,
    started_at      DATETIME,
    finished_at     DATETIME,
    duration_ms     INTEGER      NOT NULL DEFAULT 0,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_agent_runs_tenant ON agent_runs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_agent  ON agent_runs (agent_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_credentials (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    name            VARCHAR(128) NOT NULL,
    credential_type VARCHAR(32)  NOT NULL,
    ciphertext      BLOB         NOT NULL,
    nonce           BLOB         NOT NULL,
    auth_tag        BLOB         NOT NULL,
    enc_meta        TEXT         NOT NULL DEFAULT '{}',
    created_by      INTEGER      NOT NULL,
    last_used_at    DATETIME,
    expires_at      DATETIME,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS agent_credit_ledger (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    run_id          INTEGER,
    operation       VARCHAR(32)  NOT NULL,
    unit            VARCHAR(32)  NOT NULL,
    quantity        INTEGER      NOT NULL,
    balance_after   INTEGER      NOT NULL,
    policy_version  INTEGER      NOT NULL DEFAULT 1,
    notes           TEXT         NOT NULL DEFAULT '',
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_credit_ledger_tenant ON agent_credit_ledger (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_quota_policies (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    tenant_id       INTEGER      NOT NULL,
    name            VARCHAR(128) NOT NULL,
    version         INTEGER      NOT NULL DEFAULT 1,
    monthly_tokens  INTEGER      NOT NULL DEFAULT 0,
    daily_invocations INTEGER    NOT NULL DEFAULT 0,
    per_run_cost_cap_micros INTEGER NOT NULL DEFAULT 0,
    per_agent_concurrency INTEGER NOT NULL DEFAULT 0,
    is_active       BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by      INTEGER      NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, name, version)
);
