-- v0.7.21 — Custom Agent Studio (飞书妙搭 / Notion Custom Agents parity)
--
-- Adds the execution, trigger, credential, and quota layers on top of the
-- existing custom_agents table (migration 000006). The five new tables:
--
--   agent_triggers        — scheduled / event / webhook triggers per agent
--   agent_runs            — execution history (audit + observability)
--   agent_credentials     — encrypted vault for tool credentials
--   agent_credit_ledger   — append-only quota ledger (tokens / invocations / cost)
--   agent_quota_policies  — versioned quota policies (immutable history)
--
-- Rollback: 000114_agent_studio.down.sql drops all five tables.
CREATE TABLE IF NOT EXISTS agent_triggers (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    -- trigger types: cron | event | webhook | manual
    trigger_type    VARCHAR(32)  NOT NULL,
    -- trigger name (unique per agent)
    name            VARCHAR(128) NOT NULL,
    -- type-specific config: cron expr / event filter / webhook path
    trigger_config  TEXT         NOT NULL DEFAULT '{}',
    -- payload template (Go template) for the agent input
    payload_template TEXT        NOT NULL DEFAULT '',
    -- status: active | paused | archived
    status          VARCHAR(32)  NOT NULL DEFAULT 'active',
    -- last fired time + last fire result
    last_fired_at   TIMESTAMPTZ,
    last_fire_status VARCHAR(32),
    next_fire_at    TIMESTAMPTZ,
    created_by      BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, agent_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agent_triggers_tenant ON agent_triggers (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_agent_triggers_next   ON agent_triggers (next_fire_at) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS agent_runs (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    trigger_id      BIGINT,
    -- triggered_by: manual | cron | event | webhook | api | system
    triggered_by    VARCHAR(32)  NOT NULL,
    triggered_user  BIGINT,
    -- status: queued | running | succeeded | failed | timeout | cancelled
    status          VARCHAR(32)  NOT NULL DEFAULT 'queued',
    input_payload   TEXT         NOT NULL DEFAULT '{}',
    output_payload  TEXT         NOT NULL DEFAULT '{}',
    error_message   TEXT         NOT NULL DEFAULT '',
    steps_count     INT          NOT NULL DEFAULT 0,
    tokens_used     BIGINT       NOT NULL DEFAULT 0,
    cost_micros     BIGINT       NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    duration_ms     INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_runs_tenant ON agent_runs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_agent  ON agent_runs (agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs (status, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_credentials (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    name            VARCHAR(128) NOT NULL,
    -- credential types: api_key | oauth2 | basic | bearer | custom
    credential_type VARCHAR(32)  NOT NULL,
    -- AES-256-GCM ciphertext + IV + auth tag concatenated
    ciphertext      BYTEA        NOT NULL,
    -- nonce (12 bytes) for GCM
    nonce           BYTEA        NOT NULL,
    -- auth tag (16 bytes)
    auth_tag        BYTEA        NOT NULL,
    -- encryption metadata: key_id, algorithm_version
    enc_meta        TEXT         NOT NULL DEFAULT '{}',
    created_by      BIGINT       NOT NULL,
    last_used_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS agent_credit_ledger (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    agent_id        VARCHAR(36)  NOT NULL,
    run_id          BIGINT,
    -- operations: charge | refund | grant | expire | adjust
    operation       VARCHAR(32)  NOT NULL,
    -- units: tokens | invocations | cost_micros
    unit            VARCHAR(32)  NOT NULL,
    -- quantity (positive charge / negative refund)
    quantity        BIGINT       NOT NULL,
    balance_after   BIGINT       NOT NULL,
    policy_version  BIGINT       NOT NULL DEFAULT 1,
    notes           TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_credit_ledger_tenant ON agent_credit_ledger (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_credit_ledger_agent  ON agent_credit_ledger (agent_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_quota_policies (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    name            VARCHAR(128) NOT NULL,
    version         BIGINT       NOT NULL DEFAULT 1,
    -- monthly token cap (0 = unlimited)
    monthly_tokens  BIGINT       NOT NULL DEFAULT 0,
    -- daily invocation cap (0 = unlimited)
    daily_invocations BIGINT     NOT NULL DEFAULT 0,
    -- per-run cost cap in micro-USD (0 = unlimited)
    per_run_cost_cap_micros BIGINT NOT NULL DEFAULT 0,
    -- single agent concurrency cap (0 = unlimited)
    per_agent_concurrency INT    NOT NULL DEFAULT 0,
    is_active       BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by      BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name, version)
);
