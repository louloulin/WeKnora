-- v0.7.19 — wiki_doc_snapshots
--
-- Background: Build #32 introduces Yjs-based realtime collaboration on
-- wiki pages. The Yjs document state (Y.encodeStateAsUpdate output) is the
-- authoritative binary representation that all connected clients converge to;
-- this table is the durable snapshot store so a server restart (or new
-- tenant join) restores the latest collaborative state.
--
-- Schema decisions:
--   • ydoc_state BYTEA — Y.encodeStateAsUpdate(doc) output, ~10-50 KB per page
--   • vector_clock BYTEA — last-seen client clocks for incremental merge
--   • size_bytes INT — for LRU / GC heuristics
--   • UNIQUE (tenant_id, kb_id, page_id) — one row per page, idempotent upserts
--   • updated_at triggers reuse wiki audit time helpers
--
-- Rollback: 000110_wiki_doc_snapshots.down.sql drops the table.
CREATE TABLE IF NOT EXISTS wiki_doc_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    page_id         VARCHAR(36)  NOT NULL,
    ydoc_state      BYTEA        NOT NULL,
    vector_clock    BYTEA        NOT NULL DEFAULT decode('', 'hex'),
    version         BIGINT       NOT NULL DEFAULT 1,
    size_bytes      INT          NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, kb_id, page_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_doc_snapshots_tenant
    ON wiki_doc_snapshots (tenant_id);

CREATE INDEX IF NOT EXISTS idx_wiki_doc_snapshots_updated
    ON wiki_doc_snapshots (updated_at DESC);

-- Auto-bump updated_at on UPDATE so snapshot writes always advance the timestamp.
CREATE OR REPLACE FUNCTION wiki_doc_snapshots_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := NOW();
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_wiki_doc_snapshots_updated_at ON wiki_doc_snapshots;
CREATE TRIGGER trg_wiki_doc_snapshots_updated_at
    BEFORE UPDATE ON wiki_doc_snapshots
    FOR EACH ROW EXECUTE FUNCTION wiki_doc_snapshots_set_updated_at();
