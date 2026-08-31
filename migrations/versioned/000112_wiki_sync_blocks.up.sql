-- v0.7.20 — wiki_sync_blocks
--
-- Background: Build #34 introduces "Synced Blocks" — the canonical
-- reusable content unit that lives independently of any specific page.
-- Pages embed references via `[[sync:UUID]]` markers stored in
-- wiki_sync_block_refs. Editing the canonical once makes every embedded
-- reference re-render to the latest version automatically. This brings
-- WeKnora Wiki to feature parity with Notion Synced Blocks, 飞书同步块,
-- and Microsoft Loop components.
--
-- Schema decisions:
--   • block_id UUID (VARCHAR(36)) — the canonical identity, stable across renames
--   • content_json TEXT — ProseMirror/Tiptap JSON, language-agnostic
--   • content_md TEXT — Markdown projection (for search indexing & export)
--   • version BIGINT — auto-incremented on every update; fan-out keys off this
--   • owner_id — first creator; subsequent edits don't transfer ownership
--   • UNIQUE (tenant_id, block_id) — one canonical row per logical block
--
-- Rollback: 000112_wiki_sync_blocks.down.sql drops the table.
CREATE TABLE IF NOT EXISTS wiki_sync_blocks (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    block_id        VARCHAR(36)  NOT NULL,
    title           VARCHAR(256) NOT NULL DEFAULT '',
    content_json    TEXT         NOT NULL DEFAULT '{}',
    content_md      TEXT         NOT NULL DEFAULT '',
    version         BIGINT       NOT NULL DEFAULT 1,
    owner_id        BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, block_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_blocks_tenant
    ON wiki_sync_blocks (tenant_id);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_blocks_kb
    ON wiki_sync_blocks (tenant_id, kb_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_blocks_owner
    ON wiki_sync_blocks (owner_id);
