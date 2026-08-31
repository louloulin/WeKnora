-- v0.7.25 — collaborative_docs
--
-- Background: WeKnora v0.7.25 introduces collaborative_docs to match the
-- "Feishu doc / Tencent doc" multi-format editing surface. Unlike the Wiki
-- (markdown-rich-text) which already has Yjs realtime (v0.7.19), this new
-- surface persists typed metadata (doc_kind) alongside the binary Yjs state
-- so the same Yjs WebSocket fan-out can carry TipTap (doc), Univer (sheet),
-- and pptxgenjs (slide) updates without schema leakage.
--
-- Schema decisions:
--   * collaborative_docs: per-tenant metadata row (title, kind, owner)
--   * collab_doc_snapshots: durable Yjs state keyed by (tenant, doc_id)
--     * doc_kind VARCHAR(16) — 'doc' | 'sheet' | 'slide' (drives client renderer)
--     * schema_version INT — bump when client schema breaks compatibility
--     * ydoc_state BYTEA — Y.encodeStateAsUpdate output
--   * collab_doc_sessions: live presence rows (user, client_id)
--   * UNIQUE (tenant_id, doc_id) so two clients cannot fork a doc
--   * Cascade FK from snapshots/sessions -> docs so delete cleans up
--
-- Rollback: 000132_collaborative_docs.down.sql drops all three tables.
CREATE TABLE IF NOT EXISTS collaborative_docs (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    title           VARCHAR(256) NOT NULL,
    doc_kind        VARCHAR(16)  NOT NULL DEFAULT 'doc',
    schema_version  INT          NOT NULL DEFAULT 1,
    owner_user_id   BIGINT       NOT NULL,
    visibility      VARCHAR(16)  NOT NULL DEFAULT 'private',
    share_token     VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    archived_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_collaborative_docs_tenant
    ON collaborative_docs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_collaborative_docs_kb
    ON collaborative_docs (kb_id);

CREATE TABLE IF NOT EXISTS collab_doc_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    doc_kind        VARCHAR(16)  NOT NULL DEFAULT 'doc',
    schema_version  INT          NOT NULL DEFAULT 1,
    ydoc_state      BYTEA        NOT NULL,
    vector_clock    BYTEA        NOT NULL DEFAULT decode('', 'hex'),
    version         BIGINT       NOT NULL DEFAULT 1,
    size_bytes      INT          NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, doc_id),
    CONSTRAINT fk_collab_doc_snapshots_doc FOREIGN KEY (doc_id)
        REFERENCES collaborative_docs (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_collab_doc_snapshots_tenant
    ON collab_doc_snapshots (tenant_id);
CREATE INDEX IF NOT EXISTS idx_collab_doc_snapshots_updated
    ON collab_doc_snapshots (updated_at DESC);

CREATE OR REPLACE FUNCTION collab_doc_snapshots_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := NOW();
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_collab_doc_snapshots_updated_at ON collab_doc_snapshots;
CREATE TRIGGER trg_collab_doc_snapshots_updated_at
    BEFORE UPDATE ON collab_doc_snapshots
    FOR EACH ROW EXECUTE FUNCTION collab_doc_snapshots_set_updated_at();

CREATE TABLE IF NOT EXISTS collab_doc_sessions (
    id              VARCHAR(36)  PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    doc_id          VARCHAR(36)  NOT NULL,
    user_id         BIGINT       NOT NULL,
    client_id       BIGINT       NOT NULL,
    color           VARCHAR(16)  NOT NULL DEFAULT '#58a6ff',
    display_name    VARCHAR(128) NOT NULL DEFAULT '',
    last_heartbeat  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    joined_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, doc_id, client_id),
    CONSTRAINT fk_collab_doc_sessions_doc FOREIGN KEY (doc_id)
        REFERENCES collaborative_docs (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_doc
    ON collab_doc_sessions (doc_id, last_heartbeat DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_user
    ON collab_doc_sessions (user_id, last_heartbeat DESC);
CREATE INDEX IF NOT EXISTS idx_collab_doc_sessions_heartbeat
    ON collab_doc_sessions (last_heartbeat);
