-- v0.7.20 — wiki_sync_block_refs
--
-- Background: companion table to wiki_sync_blocks. Each row records that
-- a specific wiki page embeds a specific synced block at a specific
-- anchor. On every page save the service rewrites this table from the
-- page content's `[[sync:UUID]]` markers, then calls MarkRendered to bump
-- content_version + rendered_at on each surviving ref.
--
-- Schema decisions:
--   • (block_id, page_id, anchor_slug) is the natural key
--   • content_version — last version of the canonical block this ref saw
--   • rendered_at — when this ref was last refreshed by the page renderer
--   • No FK to wiki_pages — pages can be deleted with refs auto-purged via service
--   • Indexes on (tenant, block) for fan-out, (tenant, page) for ref list
--
-- Rollback: 000113_wiki_sync_block_refs.down.sql drops the table.
CREATE TABLE IF NOT EXISTS wiki_sync_block_refs (
    id              BIGSERIAL    PRIMARY KEY,
    tenant_id       BIGINT       NOT NULL,
    kb_id           VARCHAR(36)  NOT NULL,
    block_id        VARCHAR(36)  NOT NULL,
    page_id         VARCHAR(36)  NOT NULL,
    anchor_slug     VARCHAR(256) NOT NULL DEFAULT '',
    content_version BIGINT       NOT NULL DEFAULT 0,
    rendered_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, block_id, page_id, anchor_slug)
);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_block_refs_block
    ON wiki_sync_block_refs (tenant_id, block_id);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_block_refs_page
    ON wiki_sync_block_refs (tenant_id, page_id);

CREATE INDEX IF NOT EXISTS idx_wiki_sync_block_refs_rendered
    ON wiki_sync_block_refs (rendered_at DESC);
