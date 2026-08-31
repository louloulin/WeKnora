-- v0.7.22 — DLP (Data Loss Prevention) policies + violations
--
-- Closes the data-security gap to Microsoft Purview / 飞书安全中心 /
-- Confluence DLP. Lets tenants define regex / dictionary rules that
-- scan chat messages, wiki pages, and KB documents for sensitive
-- content, then log violations to a queryable history table.
--
-- Three tables:
--   dlp_policies           — versioned policy definitions (immutable history)
--   dlp_rules              — individual regex / dictionary entries inside a policy
--   dlp_violations         — append-only scan results, queryable for SOC 2 audits
--
-- Rollback: 000115_dlp_policies.down.sql drops all three tables.
CREATE TABLE IF NOT EXISTS dlp_policies (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    name            VARCHAR(128) NOT NULL,
    version         BIGINT       NOT NULL DEFAULT 1,
    -- applicable resources: kb | wiki_page | chat_message | file | custom_agent | *
    resource_scope  VARCHAR(64)  NOT NULL DEFAULT '*',
    severity        VARCHAR(32)  NOT NULL DEFAULT 'medium',
    -- action: log | block | redact | notify_dpo
    action          VARCHAR(32)  NOT NULL DEFAULT 'log',
    is_active       BOOLEAN      NOT NULL DEFAULT FALSE,
    description     TEXT         NOT NULL DEFAULT '',
    created_by      BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX IF NOT EXISTS idx_dlp_policies_tenant ON dlp_policies (tenant_id, is_active);

CREATE TABLE IF NOT EXISTS dlp_rules (
    id              BIGSERIAL    PRIMARY KEY,
    policy_id       BIGINT       NOT NULL,
    tenant_id       BIGINT       NOT NULL,
    -- pattern_type: regex | dictionary | builtin
    pattern_type    VARCHAR(32)  NOT NULL,
    -- for regex: the pattern string
    -- for dictionary: comma-separated entries
    -- for builtin: one of "credit_card" | "id_card_cn" | "ssn_us" | "email" | "phone_cn" | "phone_intl" | "ip_addr"
    pattern_value   TEXT         NOT NULL,
    -- severity override at rule level
    severity        VARCHAR(32)  NOT NULL DEFAULT 'medium',
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    description     TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dlp_rules_policy ON dlp_rules (policy_id);

CREATE TABLE IF NOT EXISTS dlp_violations (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    policy_id       BIGINT       NOT NULL,
    rule_id         BIGINT,
    -- resource: kb | wiki_page | chat_message | file | custom_agent
    resource        VARCHAR(64)  NOT NULL,
    resource_id     VARCHAR(36)  NOT NULL,
    actor_id        BIGINT       NOT NULL,
    -- the offending substring (truncated to 256 chars)
    matched_value   TEXT         NOT NULL,
    -- context window (truncated to 512 chars)
    context         TEXT         NOT NULL DEFAULT '',
    -- rule that fired: builtin name / regex / dictionary key
    matched_pattern VARCHAR(128) NOT NULL,
    action_taken    VARCHAR(32)  NOT NULL,
    severity        VARCHAR(32)  NOT NULL,
    -- optional correlation back to audit_log
    audit_log_id    BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dlp_violations_tenant   ON dlp_violations (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlp_violations_resource ON dlp_violations (tenant_id, resource, resource_id);
